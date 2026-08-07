package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"viki/internal/governance"
	"viki/internal/model"
	"viki/internal/store"
)

func (r *Repository) ApproveRevision(ctx context.Context, organizationID, userID, revisionID string) (model.PageDetail, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.PageDetail{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var pageID string
	var pageKind model.PageKind
	var latestDraftID *string
	var approvedID *string
	var parentApprovedID *string
	err = tx.QueryRow(ctx, `
		SELECT p.id::text, p.kind, p.latest_draft_revision_id::text, p.approved_revision_id::text,
		       parent.approved_revision_id::text
		FROM pages p
		JOIN revisions r ON r.page_id = p.id
		LEFT JOIN pages parent ON parent.id = p.parent_id
		WHERE p.organization_id = $1 AND r.id = $2
		FOR UPDATE OF p
	`, organizationID, revisionID).Scan(&pageID, &pageKind, &latestDraftID, &approvedID, &parentApprovedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.PageDetail{}, store.ErrNotFound
	}
	if err != nil {
		return model.PageDetail{}, err
	}
	if latestDraftID == nil || *latestDraftID != revisionID {
		return model.PageDetail{}, store.ErrConflict
	}
	if pageKind == model.PageScenario && parentApprovedID == nil {
		return model.PageDetail{}, governance.ErrParentFeatureNotApproved
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM objections WHERE page_id = $1 AND resolved_at IS NULL`, pageID)
	if err != nil {
		return model.PageDetail{}, err
	}
	objections := []governance.ObjectionState{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return model.PageDetail{}, err
		}
		objections = append(objections, governance.ObjectionState{ID: id})
	}
	rows.Close()
	if err := governance.CanApprove(objections); err != nil {
		return model.PageDetail{}, err
	}
	if approvedID != nil {
		if _, err := tx.Exec(ctx, `UPDATE revisions SET status = 'superseded' WHERE id = $1`, *approvedID); err != nil {
			return model.PageDetail{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE revisions SET status = 'approved', approved_at = now() WHERE id = $1`, revisionID); err != nil {
		return model.PageDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pages SET approved_revision_id = $2, latest_draft_revision_id = NULL, updated_at = now()
		WHERE id = $1
	`, pageID, revisionID); err != nil {
		return model.PageDetail{}, err
	}
	if pageKind == model.PageScenario {
		if _, err := tx.Exec(ctx, `
			UPDATE step_definitions
			SET approved_at = COALESCE(approved_at, now())
			WHERE id IN (SELECT definition_id FROM bdd_steps WHERE revision_id = $1)
		`, revisionID); err != nil {
			return model.PageDetail{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO scenario_developments(revision_id) VALUES ($1)`, revisionID); err != nil {
			return model.PageDetail{}, err
		}
	}
	if err := audit(ctx, tx, organizationID, userID, "revision.approved", "revision", revisionID, map[string]any{"pageId": pageID}); err != nil {
		return model.PageDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.PageDetail{}, err
	}
	return r.PageDetail(ctx, organizationID, pageID)
}

func (r *Repository) AddComment(ctx context.Context, organizationID, userID, pageID, revisionID string, parentID *string, body string) (model.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return model.Comment{}, fmt.Errorf("comment body is required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.Comment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var valid bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM revisions r JOIN pages p ON p.id = r.page_id
			WHERE r.id = $1 AND p.id = $2 AND p.organization_id = $3
		)
	`, revisionID, pageID, organizationID).Scan(&valid); err != nil || !valid {
		return model.Comment{}, store.ErrNotFound
	}
	if parentID != nil {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM comments WHERE id = $1 AND page_id = $2 AND revision_id = $3)`, *parentID, pageID, revisionID).Scan(&valid); err != nil || !valid {
			return model.Comment{}, fmt.Errorf("parent comment must belong to the same revision")
		}
	}
	var commentID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO comments(page_id, revision_id, parent_comment_id, body, author_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text
	`, pageID, revisionID, parentID, body, userID).Scan(&commentID); err != nil {
		return model.Comment{}, err
	}
	if err := audit(ctx, tx, organizationID, userID, "comment.created", "comment", commentID, map[string]any{"pageId": pageID, "revisionId": revisionID}); err != nil {
		return model.Comment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Comment{}, err
	}
	return r.commentByID(ctx, organizationID, commentID)
}

func (r *Repository) AddObjection(ctx context.Context, organizationID, userID, revisionID, body string) (model.Objection, error) {
	body = strings.TrimSpace(body)
	if err := governance.ValidateObjectionReason(body); err != nil {
		return model.Objection{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.Objection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var pageID string
	err = tx.QueryRow(ctx, `
		SELECT p.id::text
		FROM revisions r
		JOIN pages p ON p.id = r.page_id
		WHERE r.id = $1 AND p.organization_id = $2
	`, revisionID, organizationID).Scan(&pageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Objection{}, store.ErrNotFound
	}
	if err != nil {
		return model.Objection{}, err
	}
	var objectionID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO objections(page_id, revision_id, body, author_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text
	`, pageID, revisionID, body, userID).Scan(&objectionID); err != nil {
		return model.Objection{}, err
	}
	if err := audit(ctx, tx, organizationID, userID, "objection.created", "objection", objectionID, map[string]any{"pageId": pageID, "revisionId": revisionID}); err != nil {
		return model.Objection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Objection{}, err
	}
	return r.objectionByID(ctx, organizationID, objectionID)
}

func (r *Repository) ResolveObjection(ctx context.Context, organizationID, userID, objectionID string) (model.Objection, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.Objection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `
		UPDATE objections o
		SET resolved_at = now(), resolved_by = $2
		FROM pages p
		WHERE o.id = $1 AND p.id = o.page_id AND p.organization_id = $3 AND o.resolved_at IS NULL
	`, objectionID, userID, organizationID)
	if err != nil {
		return model.Objection{}, err
	}
	if command.RowsAffected() == 0 {
		return model.Objection{}, store.ErrNotFound
	}
	if err := audit(ctx, tx, organizationID, userID, "objection.resolved", "objection", objectionID, nil); err != nil {
		return model.Objection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Objection{}, err
	}
	return r.objectionByID(ctx, organizationID, objectionID)
}

func (r *Repository) objectionByID(ctx context.Context, organizationID, objectionID string) (model.Objection, error) {
	var objection model.Objection
	var resolverID, resolverEmail, resolverName *string
	var resolverCreated *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT o.id::text, o.page_id::text, o.revision_id::text, source.number, o.body, o.created_at, o.resolved_at,
			a.id::text, a.email, a.display_name, a.created_at,
			ru.id::text, ru.email, ru.display_name, ru.created_at
		FROM objections o
		JOIN pages p ON p.id = o.page_id
		JOIN revisions source ON source.id = o.revision_id
		JOIN users a ON a.id = o.author_id
		LEFT JOIN users ru ON ru.id = o.resolved_by
		WHERE p.organization_id = $1 AND o.id = $2
	`, organizationID, objectionID).Scan(
		&objection.ID, &objection.PageID, &objection.RevisionID, &objection.RevisionNumber, &objection.Body, &objection.CreatedAt, &objection.ResolvedAt,
		&objection.Author.ID, &objection.Author.Email, &objection.Author.DisplayName, &objection.Author.CreatedAt,
		&resolverID, &resolverEmail, &resolverName, &resolverCreated,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Objection{}, store.ErrNotFound
	}
	if err != nil {
		return model.Objection{}, err
	}
	if resolverID != nil {
		objection.ResolvedBy = &model.User{ID: *resolverID, Email: *resolverEmail, DisplayName: *resolverName, CreatedAt: *resolverCreated}
	}
	return objection, nil
}

func (r *Repository) commentByID(ctx context.Context, organizationID, commentID string) (model.Comment, error) {
	comments, err := r.queryComments(ctx, organizationID, "c.id = $2", commentID)
	if err != nil {
		return model.Comment{}, err
	}
	if len(comments) == 0 {
		return model.Comment{}, store.ErrNotFound
	}
	return comments[0], nil
}

func (r *Repository) listComments(ctx context.Context, organizationID, pageID string) ([]model.Comment, error) {
	flat, err := r.queryComments(ctx, organizationID, "c.page_id = $2", pageID)
	if err != nil {
		return nil, err
	}
	byParent := map[string][]model.Comment{}
	roots := []model.Comment{}
	for _, comment := range flat {
		if comment.ParentCommentID == nil {
			roots = append(roots, comment)
		} else {
			byParent[*comment.ParentCommentID] = append(byParent[*comment.ParentCommentID], comment)
		}
	}
	var attach func(model.Comment) model.Comment
	attach = func(comment model.Comment) model.Comment {
		comment.Replies = []model.Comment{}
		for _, child := range byParent[comment.ID] {
			comment.Replies = append(comment.Replies, attach(child))
		}
		return comment
	}
	for index := range roots {
		roots[index] = attach(roots[index])
	}
	return roots, nil
}

func (r *Repository) queryComments(ctx context.Context, organizationID, condition string, value string) ([]model.Comment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id::text, c.page_id::text, c.revision_id::text, c.parent_comment_id::text,
			c.body, c.created_at,
			a.id::text, a.email, a.display_name, a.created_at
		FROM comments c
		JOIN pages p ON p.id = c.page_id
		JOIN users a ON a.id = c.author_id
		WHERE p.organization_id = $1 AND `+condition+`
		ORDER BY c.created_at
	`, organizationID, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	comments := []model.Comment{}
	for rows.Next() {
		var comment model.Comment
		if err := rows.Scan(
			&comment.ID, &comment.PageID, &comment.RevisionID, &comment.ParentCommentID,
			&comment.Body, &comment.CreatedAt,
			&comment.Author.ID, &comment.Author.Email, &comment.Author.DisplayName, &comment.Author.CreatedAt,
		); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}
