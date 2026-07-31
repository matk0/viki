package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"viki/internal/model"
	"viki/internal/store"
)

type scanner interface {
	Scan(...any) error
}

const pageSelect = `
	SELECT
		p.id::text,
		p.kind,
		p.primitive_kind,
		p.parent_id::text,
		p.slug,
		COALESCE(dr.title, ar.title, p.slug),
		p.accepted_revision_id::text,
		p.latest_draft_revision_id::text,
		(p.accepted_revision_id IS NOT NULL),
		(p.latest_draft_revision_id IS NOT NULL),
		(SELECT count(*) FROM comments c WHERE c.page_id = p.id AND c.blocking AND c.resolved_at IS NULL),
		p.created_at,
		p.updated_at
	FROM pages p
	LEFT JOIN revisions ar ON ar.id = p.accepted_revision_id
	LEFT JOIN revisions dr ON dr.id = p.latest_draft_revision_id
`

func scanPage(row scanner) (model.Page, error) {
	var page model.Page
	var kind string
	var primitiveKind *string
	if err := row.Scan(
		&page.ID,
		&kind,
		&primitiveKind,
		&page.ParentID,
		&page.Slug,
		&page.Title,
		&page.AcceptedRevisionID,
		&page.LatestDraftRevisionID,
		&page.Accepted,
		&page.HasDraft,
		&page.UnresolvedRejections,
		&page.CreatedAt,
		&page.UpdatedAt,
	); err != nil {
		return model.Page{}, err
	}
	page.Kind = model.PageKind(kind)
	if primitiveKind != nil {
		value := model.PrimitiveKind(*primitiveKind)
		page.PrimitiveKind = &value
	}
	return page, nil
}

func (r *Repository) ListPages(ctx context.Context, organizationID string, kind *model.PageKind) ([]model.Page, error) {
	query := pageSelect + ` WHERE p.organization_id = $1`
	args := []any{organizationID}
	if kind != nil {
		query += ` AND p.kind = $2`
		args = append(args, string(*kind))
	}
	query += ` ORDER BY CASE p.kind WHEN 'scenario' THEN 1 WHEN 'subscenario' THEN 2 ELSE 3 END, lower(COALESCE(dr.title, ar.title, p.slug))`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	defer rows.Close()
	pages := make([]model.Page, 0)
	for rows.Next() {
		page, err := scanPage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan page: %w", err)
		}
		pages = append(pages, page)
	}
	return pages, rows.Err()
}

func (r *Repository) SearchPages(ctx context.Context, organizationID string, options model.SearchOptions) ([]model.SearchResult, error) {
	if options.Limit <= 0 || options.Limit > 100 {
		options.Limit = 30
	}
	queryText := strings.TrimSpace(options.Query)
	args := []any{organizationID, queryText, options.IncludeDrafts, options.Limit}
	kindClause := ""
	if options.Kind != nil {
		args = append(args, string(*options.Kind))
		kindClause = fmt.Sprintf(" AND p.kind = $%d", len(args))
	}

	query := fmt.Sprintf(`
		WITH eligible AS (
			SELECT p.*, r.id AS revision_id, r.title, r.body_md, r.status,
				CASE WHEN r.id = p.latest_draft_revision_id THEN true ELSE false END AS is_draft,
				(
					ts_rank(r.search_document, plainto_tsquery('simple', $2)) * 2
					+ similarity(r.title, $2) * 2
					+ similarity(r.body_md, $2)
				) AS score
			FROM pages p
			JOIN revisions r ON r.id = p.accepted_revision_id OR ($3 AND r.id = p.latest_draft_revision_id)
			WHERE p.organization_id = $1 %s
		)
		SELECT id::text, kind, primitive_kind, parent_id::text, slug, title,
			accepted_revision_id::text, latest_draft_revision_id::text,
			(accepted_revision_id IS NOT NULL), (latest_draft_revision_id IS NOT NULL),
			(SELECT count(*) FROM comments c WHERE c.page_id = eligible.id AND c.blocking AND c.resolved_at IS NULL),
			created_at, updated_at, revision_id::text,
			left(regexp_replace(body_md, '\\s+', ' ', 'g'), 240), score, is_draft
		FROM eligible
		WHERE $2 = '' OR score > 0.05 OR title ILIKE '%%' || $2 || '%%' OR body_md ILIKE '%%' || $2 || '%%'
		ORDER BY score DESC, lower(title)
		LIMIT $4
	`, kindClause)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search pages: %w", err)
	}
	defer rows.Close()
	results := make([]model.SearchResult, 0)
	for rows.Next() {
		var result model.SearchResult
		var kind string
		var primitiveKind *string
		if err := rows.Scan(
			&result.Page.ID,
			&kind,
			&primitiveKind,
			&result.Page.ParentID,
			&result.Page.Slug,
			&result.Page.Title,
			&result.Page.AcceptedRevisionID,
			&result.Page.LatestDraftRevisionID,
			&result.Page.Accepted,
			&result.Page.HasDraft,
			&result.Page.UnresolvedRejections,
			&result.Page.CreatedAt,
			&result.Page.UpdatedAt,
			&result.RevisionID,
			&result.Excerpt,
			&result.Score,
			&result.Draft,
		); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		result.Page.Kind = model.PageKind(kind)
		if primitiveKind != nil {
			value := model.PrimitiveKind(*primitiveKind)
			result.Page.PrimitiveKind = &value
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (r *Repository) PageDetail(ctx context.Context, organizationID, pageID string) (model.PageDetail, error) {
	page, err := scanPage(r.pool.QueryRow(ctx, pageSelect+` WHERE p.organization_id = $1 AND p.id = $2`, organizationID, pageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.PageDetail{}, store.ErrNotFound
	}
	if err != nil {
		return model.PageDetail{}, fmt.Errorf("page detail: %w", err)
	}
	detail := model.PageDetail{Page: page, Revisions: []model.RevisionSummary{}, Comments: []model.Comment{}, Votes: []model.Vote{}, Children: []model.Page{}}
	if page.AcceptedRevisionID != nil {
		revision, err := r.loadRevision(ctx, *page.AcceptedRevisionID)
		if err != nil {
			return model.PageDetail{}, err
		}
		detail.AcceptedRevision = &revision
	}
	if page.LatestDraftRevisionID != nil {
		revision, err := r.loadRevision(ctx, *page.LatestDraftRevisionID)
		if err != nil {
			return model.PageDetail{}, err
		}
		detail.DraftRevision = &revision
	}
	if detail.Revisions, err = r.listRevisionSummaries(ctx, pageID); err != nil {
		return model.PageDetail{}, err
	}
	if detail.Comments, err = r.listComments(ctx, organizationID, pageID); err != nil {
		return model.PageDetail{}, err
	}
	if detail.Votes, err = r.listVotes(ctx, organizationID, pageID); err != nil {
		return model.PageDetail{}, err
	}
	if page.Kind == model.PageScenario {
		rows, err := r.pool.Query(ctx, pageSelect+` WHERE p.organization_id = $1 AND p.parent_id = $2 ORDER BY p.created_at`, organizationID, pageID)
		if err != nil {
			return model.PageDetail{}, err
		}
		defer rows.Close()
		for rows.Next() {
			child, err := scanPage(rows)
			if err != nil {
				return model.PageDetail{}, err
			}
			detail.Children = append(detail.Children, child)
		}
	}
	return detail, nil
}

func (r *Repository) Revision(ctx context.Context, organizationID, revisionID string) (model.Revision, error) {
	var valid bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM revisions r JOIN pages p ON p.id = r.page_id
			WHERE r.id = $1 AND p.organization_id = $2
		)
	`, revisionID, organizationID).Scan(&valid); err != nil {
		return model.Revision{}, err
	}
	if !valid {
		return model.Revision{}, store.ErrNotFound
	}
	return r.loadRevision(ctx, revisionID)
}

func (r *Repository) loadRevision(ctx context.Context, revisionID string) (model.Revision, error) {
	var revision model.Revision
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT r.id::text, r.page_id::text, r.number, r.status, r.title, r.body_md,
			r.aliases, r.base_revision_id::text,
			u.id::text, u.email, u.display_name, u.created_at,
			r.created_at, r.accepted_at
		FROM revisions r
		JOIN users u ON u.id = r.created_by
		WHERE r.id = $1
	`, revisionID).Scan(
		&revision.ID,
		&revision.PageID,
		&revision.Number,
		&status,
		&revision.Title,
		&revision.BodyMD,
		&revision.Aliases,
		&revision.BaseRevisionID,
		&revision.CreatedBy.ID,
		&revision.CreatedBy.Email,
		&revision.CreatedBy.DisplayName,
		&revision.CreatedBy.CreatedAt,
		&revision.CreatedAt,
		&revision.AcceptedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Revision{}, store.ErrNotFound
	}
	if err != nil {
		return model.Revision{}, fmt.Errorf("load revision: %w", err)
	}
	revision.Status = model.RevisionStatus(status)
	revision.Steps = []model.Step{}
	steps, err := r.pool.Query(ctx, `
		SELECT id::text, stable_id::text, position, keyword, text
		FROM bdd_steps WHERE revision_id = $1 ORDER BY position
	`, revisionID)
	if err != nil {
		return model.Revision{}, err
	}
	defer steps.Close()
	for steps.Next() {
		var step model.Step
		var keyword string
		if err := steps.Scan(&step.ID, &step.StableID, &step.Position, &keyword, &step.Text); err != nil {
			return model.Revision{}, err
		}
		step.Keyword = model.BDDKeyword(keyword)
		revision.Steps = append(revision.Steps, step)
	}
	revision.References = []model.PageReference{}
	references, err := r.pool.Query(ctx, `
		SELECT pr.target_page_id::text, COALESCE(ar.title, p.slug), pr.relation
		FROM page_references pr
		JOIN pages p ON p.id = pr.target_page_id
		LEFT JOIN revisions ar ON ar.id = p.accepted_revision_id
		WHERE pr.revision_id = $1
		ORDER BY lower(COALESCE(ar.title, p.slug))
	`, revisionID)
	if err != nil {
		return model.Revision{}, err
	}
	defer references.Close()
	for references.Next() {
		var reference model.PageReference
		if err := references.Scan(&reference.TargetPageID, &reference.TargetTitle, &reference.Relation); err != nil {
			return model.Revision{}, err
		}
		revision.References = append(revision.References, reference)
	}
	return revision, nil
}

func (r *Repository) listRevisionSummaries(ctx context.Context, pageID string) ([]model.RevisionSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id::text, r.number, r.status, r.title,
			u.id::text, u.email, u.display_name, u.created_at,
			r.created_at, r.accepted_at
		FROM revisions r JOIN users u ON u.id = r.created_by
		WHERE r.page_id = $1 ORDER BY r.number DESC
	`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.RevisionSummary{}
	for rows.Next() {
		var summary model.RevisionSummary
		var status string
		if err := rows.Scan(
			&summary.ID, &summary.Number, &status, &summary.Title,
			&summary.CreatedBy.ID, &summary.CreatedBy.Email, &summary.CreatedBy.DisplayName, &summary.CreatedBy.CreatedAt,
			&summary.CreatedAt, &summary.AcceptedAt,
		); err != nil {
			return nil, err
		}
		summary.Status = model.RevisionStatus(status)
		result = append(result, summary)
	}
	return result, rows.Err()
}
