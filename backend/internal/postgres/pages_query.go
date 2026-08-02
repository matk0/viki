package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
		p.concept_kind,
		p.parent_id::text,
		p.slug,
		COALESCE(dr.title, ar.title, p.slug),
		p.approved_revision_id::text,
		p.latest_draft_revision_id::text,
		ar.title,
		dr.title,
		(p.approved_revision_id IS NOT NULL),
		(p.latest_draft_revision_id IS NOT NULL),
		(SELECT count(*) FROM objections o WHERE o.page_id = p.id AND o.resolved_at IS NULL),
		p.created_at,
		p.updated_at
	FROM pages p
	LEFT JOIN revisions ar ON ar.id = p.approved_revision_id
	LEFT JOIN revisions dr ON dr.id = p.latest_draft_revision_id
`

func scanPage(row scanner) (model.Page, error) {
	var page model.Page
	var kind string
	var conceptKind *string
	if err := row.Scan(
		&page.ID,
		&kind,
		&conceptKind,
		&page.ParentID,
		&page.Slug,
		&page.Title,
		&page.ApprovedRevisionID,
		&page.LatestDraftRevisionID,
		&page.ApprovedRevisionTitle,
		&page.DraftRevisionTitle,
		&page.Approved,
		&page.HasDraft,
		&page.UnresolvedObjections,
		&page.CreatedAt,
		&page.UpdatedAt,
	); err != nil {
		return model.Page{}, err
	}
	page.Kind = model.PageKind(kind)
	if conceptKind != nil {
		value := model.ConceptKind(*conceptKind)
		page.ConceptKind = &value
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
	query += ` ORDER BY CASE p.kind WHEN 'feature' THEN 1 WHEN 'scenario' THEN 2 ELSE 3 END, lower(COALESCE(dr.title, ar.title, p.slug))`
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
				ar.title AS approved_revision_title, dr.title AS draft_revision_title,
				CASE WHEN r.id = p.latest_draft_revision_id THEN true ELSE false END AS is_draft,
				(
					ts_rank(r.search_document, plainto_tsquery('simple', $2)) * 2
					+ similarity(r.title, $2) * 2
					+ similarity(r.body_md, $2)
				) AS score
			FROM pages p
			JOIN revisions r ON r.id = p.approved_revision_id OR ($3 AND r.id = p.latest_draft_revision_id)
			LEFT JOIN revisions ar ON ar.id = p.approved_revision_id
			LEFT JOIN revisions dr ON dr.id = p.latest_draft_revision_id
			WHERE p.organization_id = $1 %s
		)
		SELECT id::text, kind, concept_kind, parent_id::text, slug, title,
			approved_revision_id::text, latest_draft_revision_id::text,
			approved_revision_title, draft_revision_title,
			(approved_revision_id IS NOT NULL), (latest_draft_revision_id IS NOT NULL),
			(SELECT count(*) FROM objections o WHERE o.page_id = eligible.id AND o.resolved_at IS NULL),
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
		var conceptKind *string
		if err := rows.Scan(
			&result.Page.ID,
			&kind,
			&conceptKind,
			&result.Page.ParentID,
			&result.Page.Slug,
			&result.Page.Title,
			&result.Page.ApprovedRevisionID,
			&result.Page.LatestDraftRevisionID,
			&result.Page.ApprovedRevisionTitle,
			&result.Page.DraftRevisionTitle,
			&result.Page.Approved,
			&result.Page.HasDraft,
			&result.Page.UnresolvedObjections,
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
		if conceptKind != nil {
			value := model.ConceptKind(*conceptKind)
			result.Page.ConceptKind = &value
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
	detail := model.PageDetail{Page: page, Revisions: []model.RevisionSummary{}, Comments: []model.Comment{}, Objections: []model.Objection{}, Children: []model.Page{}, ReviewStates: []model.RevisionReviewState{}}
	if page.ApprovedRevisionID != nil {
		revision, err := r.loadRevision(ctx, *page.ApprovedRevisionID)
		if err != nil {
			return model.PageDetail{}, err
		}
		detail.ApprovedRevision = &revision
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
	if detail.ReviewStates, detail.Objections, err = r.reviewStates(ctx, organizationID, detail); err != nil {
		return model.PageDetail{}, err
	}
	if page.Kind == model.PageScenario && detail.ApprovedRevision != nil {
		development, err := r.scenarioDevelopment(ctx, detail.ApprovedRevision.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return model.PageDetail{}, err
		}
		if err == nil {
			detail.Development = &development
		}
	}
	if page.Kind == model.PageFeature {
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

func (r *Repository) reviewStates(ctx context.Context, organizationID string, detail model.PageDetail) ([]model.RevisionReviewState, []model.Objection, error) {
	states := make([]model.RevisionReviewState, 0, 2)
	objections := make([]model.Objection, 0)
	if detail.ApprovedRevision != nil {
		states = append(states, model.RevisionReviewState{RevisionID: detail.ApprovedRevision.ID, State: model.ReviewApproved, Blockers: []model.ReviewBlocker{}})
	}

	blockers := make([]model.ReviewBlocker, 0)
	if detail.DraftRevision != nil && detail.Page.Kind == model.PageScenario && detail.Page.ParentID != nil {
		var parentTitle string
		var parentApproved bool
		err := r.pool.QueryRow(ctx, `
			SELECT COALESCE(dr.title, ar.title, p.slug), p.approved_revision_id IS NOT NULL
			FROM pages p
			LEFT JOIN revisions ar ON ar.id = p.approved_revision_id
			LEFT JOIN revisions dr ON dr.id = p.latest_draft_revision_id
			WHERE p.organization_id = $1 AND p.id = $2
		`, organizationID, *detail.Page.ParentID).Scan(&parentTitle, &parentApproved)
		if err != nil {
			return nil, nil, fmt.Errorf("load parent feature review state: %w", err)
		}
		if !parentApproved {
			parentID := *detail.Page.ParentID
			blockers = append(blockers, model.ReviewBlocker{
				ID: parentID, Type: model.BlockerParentFeature,
				RelatedPageID: &parentID, RelatedPageTitle: &parentTitle,
			})
		}
	}

	rows, err := r.pool.Query(ctx, `
		SELECT o.id::text, o.page_id::text, o.revision_id::text, source.number, o.body, o.created_at, o.resolved_at,
			a.id::text, a.email, a.display_name, a.created_at,
			ru.id::text, ru.email, ru.display_name, ru.created_at
		FROM objections o
		JOIN pages p ON p.id = o.page_id
		JOIN revisions source ON source.id = o.revision_id
		JOIN users a ON a.id = o.author_id
		LEFT JOIN users ru ON ru.id = o.resolved_by
		WHERE p.organization_id = $1 AND p.id = $2
		ORDER BY o.created_at
	`, organizationID, detail.Page.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load review objections: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var objection model.Objection
		var resolverID, resolverEmail, resolverName *string
		var resolverCreated *time.Time
		if err := rows.Scan(
			&objection.ID, &objection.PageID, &objection.RevisionID, &objection.RevisionNumber, &objection.Body, &objection.CreatedAt, &objection.ResolvedAt,
			&objection.Author.ID, &objection.Author.Email, &objection.Author.DisplayName, &objection.Author.CreatedAt,
			&resolverID, &resolverEmail, &resolverName, &resolverCreated,
		); err != nil {
			return nil, nil, fmt.Errorf("scan review objection: %w", err)
		}
		if resolverID != nil {
			objection.ResolvedBy = &model.User{ID: *resolverID, Email: *resolverEmail, DisplayName: *resolverName, CreatedAt: *resolverCreated}
		}
		objections = append(objections, objection)
		if objection.ResolvedAt == nil && detail.DraftRevision != nil {
			sourceRevisionID := objection.RevisionID
			sourceRevisionNumber := objection.RevisionNumber
			body := objection.Body
			author := objection.Author
			blockers = append(blockers, model.ReviewBlocker{
				ID: objection.ID, Type: model.BlockerObjection,
				SourceRevisionID: &sourceRevisionID, SourceRevisionNumber: &sourceRevisionNumber,
				Body: &body, Author: &author,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate review objections: %w", err)
	}

	if detail.DraftRevision == nil {
		return states, objections, nil
	}

	readiness := model.ReviewReady
	if len(blockers) > 0 {
		readiness = model.ReviewBlocked
	}
	states = append(states, model.RevisionReviewState{RevisionID: detail.DraftRevision.ID, State: readiness, Blockers: blockers})
	return states, objections, nil
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
			r.base_revision_id::text,
			u.id::text, u.email, u.display_name, u.created_at,
			r.created_at, r.approved_at
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
		&revision.BaseRevisionID,
		&revision.CreatedBy.ID,
		&revision.CreatedBy.Email,
		&revision.CreatedBy.DisplayName,
		&revision.CreatedBy.CreatedAt,
		&revision.CreatedAt,
		&revision.ApprovedAt,
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
		SELECT s.id::text, s.stable_id::text, s.position, s.keyword,
			s.definition_id::text, d.expression, s.arguments, s.text
		FROM bdd_steps s
		JOIN step_definitions d ON d.id = s.definition_id
		WHERE s.revision_id = $1
		ORDER BY s.position
	`, revisionID)
	if err != nil {
		return model.Revision{}, err
	}
	defer steps.Close()
	for steps.Next() {
		var step model.Step
		var keyword string
		if err := steps.Scan(&step.ID, &step.StableID, &step.Position, &keyword, &step.DefinitionID, &step.Expression, &step.Arguments, &step.Text); err != nil {
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
		LEFT JOIN revisions ar ON ar.id = p.approved_revision_id
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
			r.created_at, r.approved_at
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
			&summary.CreatedAt, &summary.ApprovedAt,
		); err != nil {
			return nil, err
		}
		summary.Status = model.RevisionStatus(status)
		result = append(result, summary)
	}
	return result, rows.Err()
}
