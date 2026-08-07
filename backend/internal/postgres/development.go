package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"viki/internal/model"
	"viki/internal/store"
)

func (r *Repository) HasQueuedScenarioDevelopment(ctx context.Context) (bool, error) {
	var queued bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM scenario_developments WHERE status = 'queued')`).Scan(&queued)
	return queued, err
}

func (r *Repository) ClaimScenarioDevelopment(ctx context.Context) (model.DevelopmentTask, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.DevelopmentTask{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var development model.ScenarioDevelopment
	var status string
	err = tx.QueryRow(ctx, `
		SELECT sd.revision_id::text, sd.status, sd.detail, sd.updated_at
		FROM scenario_developments sd
		JOIN revisions r ON r.id = sd.revision_id
		WHERE sd.status = 'queued'
		ORDER BY r.approved_at, sd.revision_id
		FOR UPDATE OF sd SKIP LOCKED
		LIMIT 1
	`).Scan(&development.RevisionID, &status, &development.Detail, &development.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.DevelopmentTask{}, store.ErrNotFound
	}
	if err != nil {
		return model.DevelopmentTask{}, err
	}
	if err := tx.QueryRow(ctx, `
		UPDATE scenario_developments
		SET status = 'running', updated_at = now()
		WHERE revision_id = $1
		RETURNING status, updated_at
	`, development.RevisionID).Scan(&status, &development.UpdatedAt); err != nil {
		return model.DevelopmentTask{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.DevelopmentTask{}, err
	}
	development.Status = model.DevelopmentStatus(status)
	scenario, err := r.loadRevision(ctx, development.RevisionID)
	if err != nil {
		return model.DevelopmentTask{}, err
	}
	return model.DevelopmentTask{ScenarioDevelopment: development, Scenario: scenario}, nil
}

func (r *Repository) CompleteScenarioDevelopment(ctx context.Context, revisionID, detail string) (model.ScenarioDevelopment, error) {
	return r.finishScenarioDevelopment(ctx, revisionID, model.DevelopmentDeveloped, detail)
}

func (r *Repository) BlockScenarioDevelopment(ctx context.Context, revisionID, detail string) (model.ScenarioDevelopment, error) {
	return r.finishScenarioDevelopment(ctx, revisionID, model.DevelopmentBlocked, detail)
}

func (r *Repository) finishScenarioDevelopment(ctx context.Context, revisionID string, status model.DevelopmentStatus, detail string) (model.ScenarioDevelopment, error) {
	var development model.ScenarioDevelopment
	var storedStatus string
	err := r.pool.QueryRow(ctx, `
		UPDATE scenario_developments
		SET status = $1, detail = $2, updated_at = now()
		WHERE revision_id = $3 AND status = 'running'
		RETURNING revision_id::text, status, detail, updated_at
	`, status, detail, revisionID).Scan(&development.RevisionID, &storedStatus, &development.Detail, &development.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ScenarioDevelopment{}, store.ErrNotFound
	}
	if err != nil {
		return model.ScenarioDevelopment{}, err
	}
	development.Status = model.DevelopmentStatus(storedStatus)
	return development, nil
}

func (r *Repository) scenarioDevelopment(ctx context.Context, revisionID string) (model.ScenarioDevelopment, error) {
	var development model.ScenarioDevelopment
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT revision_id::text, status, detail, updated_at
		FROM scenario_developments
		WHERE revision_id = $1
	`, revisionID).Scan(&development.RevisionID, &status, &development.Detail, &development.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ScenarioDevelopment{}, store.ErrNotFound
	}
	if err != nil {
		return model.ScenarioDevelopment{}, err
	}
	development.Status = model.DevelopmentStatus(status)
	return development, nil
}
