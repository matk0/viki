package postgres

import (
	"context"
	"strings"

	"viki/internal/model"
)

func (r *Repository) Retrieve(ctx context.Context, organizationID, query string, includeDrafts bool, limit int) ([]model.RetrievedDocument, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	args := []any{organizationID, strings.TrimSpace(query), includeDrafts, limit}
	rows, err := r.pool.Query(ctx, `
		WITH candidates AS (
			SELECT r.id, p.id AS page_id, r.title, r.body_md,
				COALESCE(r.id = p.latest_draft_revision_id, false) AS is_draft,
				(
					ts_rank(r.search_document, plainto_tsquery('simple', $2)) * 2
					+ similarity(r.title, $2) * 2
					+ similarity(r.body_md, $2)
				) AS score,
				COALESCE((SELECT string_agg(upper(s.keyword) || ' ' || s.text, E'\n' ORDER BY s.position) FROM bdd_steps s WHERE s.revision_id = r.id), '') AS steps
			FROM pages p
			JOIN revisions r ON r.id = p.approved_revision_id OR ($3 AND r.id = p.latest_draft_revision_id)
			WHERE p.organization_id = $1
		)
		SELECT id::text, page_id::text, title,
			title || E'\n' || body_md || E'\n' || steps,
			is_draft, score
		FROM candidates
		WHERE $2 = '' OR score > 0.03 OR title ILIKE '%%' || $2 || '%%' OR body_md ILIKE '%%' || $2 || '%%'
		ORDER BY score DESC, title
		LIMIT $4
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	documents := []model.RetrievedDocument{}
	for rows.Next() {
		var document model.RetrievedDocument
		if err := rows.Scan(&document.RevisionID, &document.PageID, &document.PageTitle, &document.Content, &document.Draft, &document.Score); err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, rows.Err()
}
