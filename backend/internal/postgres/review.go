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

func (r *Repository) PublishRevision(ctx context.Context, organizationID, userID, revisionID string) (model.PageDetail, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.PageDetail{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var pageID string
	var latestDraftID *string
	var acceptedID *string
	err = tx.QueryRow(ctx, `
		SELECT p.id::text, p.latest_draft_revision_id::text, p.accepted_revision_id::text
		FROM pages p
		JOIN revisions r ON r.page_id = p.id
		WHERE p.organization_id = $1 AND r.id = $2
		FOR UPDATE OF p
	`, organizationID, revisionID).Scan(&pageID, &latestDraftID, &acceptedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.PageDetail{}, store.ErrNotFound
	}
	if err != nil {
		return model.PageDetail{}, err
	}
	if latestDraftID == nil || *latestDraftID != revisionID {
		return model.PageDetail{}, store.ErrConflict
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM comments WHERE page_id = $1 AND blocking AND resolved_at IS NULL`, pageID)
	if err != nil {
		return model.PageDetail{}, err
	}
	threads := []governance.BlockingThread{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return model.PageDetail{}, err
		}
		threads = append(threads, governance.BlockingThread{ID: id})
	}
	rows.Close()
	if err := governance.CanPublish(threads); err != nil {
		return model.PageDetail{}, err
	}
	if acceptedID != nil {
		if _, err := tx.Exec(ctx, `UPDATE revisions SET status = 'superseded' WHERE id = $1`, *acceptedID); err != nil {
			return model.PageDetail{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE revisions SET status = 'accepted', accepted_at = now() WHERE id = $1`, revisionID); err != nil {
		return model.PageDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE pages SET accepted_revision_id = $2, latest_draft_revision_id = NULL, updated_at = now()
		WHERE id = $1
	`, pageID, revisionID); err != nil {
		return model.PageDetail{}, err
	}
	if err := audit(ctx, tx, organizationID, userID, "revision.published", "revision", revisionID, map[string]any{"pageId": pageID}); err != nil {
		return model.PageDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.PageDetail{}, err
	}
	return r.PageDetail(ctx, organizationID, pageID)
}

func (r *Repository) AddComment(ctx context.Context, organizationID, userID, pageID, revisionID string, parentID, anchorKind, anchorID *string, body string, blocking bool) (model.Comment, error) {
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
		INSERT INTO comments(page_id, revision_id, parent_comment_id, anchor_kind, anchor_id, body, blocking, author_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text
	`, pageID, revisionID, parentID, anchorKind, anchorID, body, blocking, userID).Scan(&commentID); err != nil {
		return model.Comment{}, err
	}
	if err := audit(ctx, tx, organizationID, userID, "comment.created", "comment", commentID, map[string]any{"pageId": pageID, "revisionId": revisionID, "blocking": blocking}); err != nil {
		return model.Comment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Comment{}, err
	}
	return r.commentByID(ctx, organizationID, commentID)
}

func (r *Repository) ResolveComment(ctx context.Context, organizationID, userID, commentID string) (model.Comment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.Comment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `
		UPDATE comments c
		SET resolved_at = now(), resolved_by = $2
		FROM pages p
		WHERE c.id = $1 AND p.id = c.page_id AND p.organization_id = $3 AND c.resolved_at IS NULL
	`, commentID, userID, organizationID)
	if err != nil {
		return model.Comment{}, err
	}
	if command.RowsAffected() == 0 {
		return model.Comment{}, store.ErrNotFound
	}
	if err := audit(ctx, tx, organizationID, userID, "comment.resolved", "comment", commentID, nil); err != nil {
		return model.Comment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Comment{}, err
	}
	return r.commentByID(ctx, organizationID, commentID)
}

func (r *Repository) SetVote(ctx context.Context, organizationID, userID, revisionID string, value governance.VoteValue, reason string) (model.Vote, error) {
	if err := governance.ValidateVote(value, reason); err != nil {
		return model.Vote{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.Vote{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var pageID string
	err = tx.QueryRow(ctx, `
		SELECT p.id::text FROM revisions r JOIN pages p ON p.id = r.page_id
		WHERE r.id = $1 AND p.organization_id = $2
	`, revisionID, organizationID).Scan(&pageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Vote{}, store.ErrNotFound
	}
	if err != nil {
		return model.Vote{}, err
	}
	var commentID *string
	if value == governance.VoteReject {
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO comments(page_id, revision_id, body, blocking, author_id)
			VALUES ($1, $2, $3, true, $4)
			RETURNING id::text
		`, pageID, revisionID, strings.TrimSpace(reason), userID).Scan(&id); err != nil {
			return model.Vote{}, err
		}
		commentID = &id
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO votes(revision_id, user_id, value, comment_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (revision_id, user_id)
		DO UPDATE SET value = EXCLUDED.value, comment_id = EXCLUDED.comment_id, created_at = now()
	`, revisionID, userID, string(value), commentID); err != nil {
		return model.Vote{}, err
	}
	if err := audit(ctx, tx, organizationID, userID, "vote.recorded", "revision", revisionID, map[string]any{"value": value, "commentId": commentID}); err != nil {
		return model.Vote{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Vote{}, err
	}
	var vote model.Vote
	err = r.pool.QueryRow(ctx, `
		SELECT v.revision_id::text, v.value, v.comment_id::text, v.created_at,
			u.id::text, u.email, u.display_name, u.created_at
		FROM votes v JOIN users u ON u.id = v.user_id
		WHERE v.revision_id = $1 AND v.user_id = $2
	`, revisionID, userID).Scan(
		&vote.RevisionID, &vote.Value, &vote.CommentID, &vote.CreatedAt,
		&vote.User.ID, &vote.User.Email, &vote.User.DisplayName, &vote.User.CreatedAt,
	)
	return vote, err
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
			c.anchor_kind, c.anchor_id, c.body, c.blocking, c.created_at, c.resolved_at,
			a.id::text, a.email, a.display_name, a.created_at,
			ru.id::text, ru.email, ru.display_name, ru.created_at
		FROM comments c
		JOIN pages p ON p.id = c.page_id
		JOIN users a ON a.id = c.author_id
		LEFT JOIN users ru ON ru.id = c.resolved_by
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
		var resolverID, resolverEmail, resolverName *string
		var resolverCreated *time.Time
		if err := rows.Scan(
			&comment.ID, &comment.PageID, &comment.RevisionID, &comment.ParentCommentID,
			&comment.AnchorKind, &comment.AnchorID, &comment.Body, &comment.Blocking, &comment.CreatedAt, &comment.ResolvedAt,
			&comment.Author.ID, &comment.Author.Email, &comment.Author.DisplayName, &comment.Author.CreatedAt,
			&resolverID, &resolverEmail, &resolverName, &resolverCreated,
		); err != nil {
			return nil, err
		}
		if resolverID != nil {
			resolver := model.User{ID: *resolverID}
			if resolverEmail != nil {
				resolver.Email = *resolverEmail
			}
			if resolverName != nil {
				resolver.DisplayName = *resolverName
			}
			if resolverCreated != nil {
				resolver.CreatedAt = *resolverCreated
			}
			comment.ResolvedBy = &resolver
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}

func (r *Repository) listVotes(ctx context.Context, organizationID, pageID string) ([]model.Vote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT v.revision_id::text, v.value, v.comment_id::text, v.created_at,
			u.id::text, u.email, u.display_name, u.created_at
		FROM votes v
		JOIN revisions r ON r.id = v.revision_id
		JOIN pages p ON p.id = r.page_id
		JOIN users u ON u.id = v.user_id
		WHERE p.organization_id = $1 AND p.id = $2
		ORDER BY v.created_at
	`, organizationID, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	votes := []model.Vote{}
	for rows.Next() {
		var vote model.Vote
		if err := rows.Scan(
			&vote.RevisionID, &vote.Value, &vote.CommentID, &vote.CreatedAt,
			&vote.User.ID, &vote.User.Email, &vote.User.DisplayName, &vote.User.CreatedAt,
		); err != nil {
			return nil, err
		}
		votes = append(votes, vote)
	}
	return votes, rows.Err()
}
