package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"viki/internal/model"
	"viki/internal/store"
)

func (r *Repository) CredentialByEmail(ctx context.Context, email string) (model.Credential, error) {
	var credential model.Credential
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, organization_id::text, email, display_name, password_hash, active, created_at
		FROM users
		WHERE lower(email) = lower($1)
	`, email).Scan(
		&credential.ID,
		&credential.OrganizationID,
		&credential.Email,
		&credential.DisplayName,
		&credential.PasswordHash,
		&credential.Active,
		&credential.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Credential{}, store.ErrUnauthorized
	}
	if err != nil {
		return model.Credential{}, fmt.Errorf("credential by email: %w", err)
	}
	return credential, nil
}

func (r *Repository) CreateSession(ctx context.Context, userID string, tokenHash, csrfHash []byte, expires time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sessions(token_hash, csrf_hash, user_id, expires_at)
		VALUES ($1, $2, $3, $4)
	`, tokenHash, csrfHash, userID, expires)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	_, _ = r.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return nil
}

func (r *Repository) SessionByHash(ctx context.Context, tokenHash []byte) (model.Session, error) {
	var session model.Session
	err := r.pool.QueryRow(ctx, `
		SELECT u.id::text, u.organization_id::text, u.email, u.display_name, u.created_at, s.csrf_hash, s.expires_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND u.active
	`, tokenHash).Scan(
		&session.User.ID,
		&session.OrganizationID,
		&session.User.Email,
		&session.User.DisplayName,
		&session.User.CreatedAt,
		&session.CSRFHash,
		&session.Expires,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Session{}, store.ErrUnauthorized
	}
	if err != nil {
		return model.Session{}, fmt.Errorf("session by hash: %w", err)
	}
	return session, nil
}

func (r *Repository) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *Repository) OrganizationIDForUser(ctx context.Context, userID string) (string, error) {
	var organizationID string
	if err := r.pool.QueryRow(ctx, `SELECT organization_id::text FROM users WHERE id = $1 AND active`, userID).Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", store.ErrUnauthorized
		}
		return "", err
	}
	return organizationID, nil
}
