package postgres

import (
	"context"
	"fmt"
	"strings"

	"viki/internal/model"
)

func (r *Repository) ListStepDefinitions(ctx context.Context, organizationID, query string, role *model.StepRole) ([]model.StepDefinition, error) {
	query = strings.TrimSpace(query)
	rows, err := r.pool.Query(ctx, `
		SELECT d.id::text, d.expression, d.role, d.approved_at IS NOT NULL, count(DISTINCT r.page_id)::int
		FROM step_definitions d
		LEFT JOIN bdd_steps s ON s.definition_id = d.id
		LEFT JOIN revisions r ON r.id = s.revision_id AND r.status = 'approved'
		WHERE d.organization_id = $1
		  AND d.approved_at IS NOT NULL
		  AND ($2 = '' OR unaccent(lower(d.expression)) LIKE '%' || unaccent(lower($2)) || '%')
		  AND ($3 = '' OR d.role = $3)
		GROUP BY d.id
		ORDER BY count(DISTINCT r.page_id) DESC, lower(d.expression)
		LIMIT 50
	`, organizationID, query, stepRoleValue(role))
	if err != nil {
		return nil, fmt.Errorf("list step definitions: %w", err)
	}
	defer rows.Close()
	definitions := make([]model.StepDefinition, 0)
	for rows.Next() {
		var definition model.StepDefinition
		if err := rows.Scan(&definition.ID, &definition.Expression, &definition.Role, &definition.Approved, &definition.UsageCount); err != nil {
			return nil, fmt.Errorf("scan step definition: %w", err)
		}
		definitions = append(definitions, definition)
	}
	return definitions, rows.Err()
}

func stepRoleValue(role *model.StepRole) string {
	if role == nil {
		return ""
	}
	return string(*role)
}
