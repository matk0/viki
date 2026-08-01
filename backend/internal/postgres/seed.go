package postgres

import (
	"context"

	"viki/internal/security"
)

const (
	defaultOrganizationID = "00000000-0000-4000-8000-000000000001"
	initialUserID         = "00000000-0000-4000-8000-000000000011"
	initialUserEmail      = "matej@matejlukasik.com"
)

var hashInitialPassword = security.HashPassword

func (r *Repository) EnsureInitialUser(ctx context.Context, password string) error {
	hash, err := hashInitialPassword(password)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO organizations(id, name) VALUES ($1, 'viki')
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name
	`, defaultOrganizationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO users(id, organization_id, email, display_name, password_hash)
		VALUES ($1, $2, $3, 'Matej', $4)
		ON CONFLICT (organization_id, email)
		DO UPDATE SET display_name = EXCLUDED.display_name, password_hash = EXCLUDED.password_hash, active = true
	`, initialUserID, defaultOrganizationID, initialUserEmail, hash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
