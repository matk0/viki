package postgres

import (
	"context"
	"time"

	"viki/internal/model"
)

func (r *Repository) ListAudit(ctx context.Context, organizationID string, limit int) ([]model.AuditEvent, error) {
	if limit <= 0 {
		limit = 80
	}
	rows, err := r.pool.Query(ctx, `
		SELECT a.id::text, a.action, a.entity_type, a.entity_id::text,
			u.id::text, u.email, u.display_name, u.created_at,
			a.metadata, a.created_at
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.organization_id = $1
		  AND a.action NOT IN ('auth.login', 'auth.logout')
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $2
	`, organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]model.AuditEvent, 0)
	for rows.Next() {
		var event model.AuditEvent
		var actorID, actorEmail, actorDisplayName *string
		var actorCreatedAt *time.Time
		if err := rows.Scan(
			&event.ID,
			&event.Action,
			&event.EntityType,
			&event.EntityID,
			&actorID,
			&actorEmail,
			&actorDisplayName,
			&actorCreatedAt,
			&event.Metadata,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		if actorID != nil && actorEmail != nil && actorDisplayName != nil && actorCreatedAt != nil {
			event.Actor = &model.User{
				ID:          *actorID,
				Email:       *actorEmail,
				DisplayName: *actorDisplayName,
				CreatedAt:   *actorCreatedAt,
			}
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
