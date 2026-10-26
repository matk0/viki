package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"viki/internal/model"
)

// A catalogue/detail/retrieval operation may need several queries. Keep those
// queries on one MVCC snapshot so an import cannot mix two valid baselines.
type repositoryReadPool struct {
	databasePool
	tx pgx.Tx
}

func (p *repositoryReadPool) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	return p.tx.Query(ctx, query, args...)
}
func (p *repositoryReadPool) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return p.tx.QueryRow(ctx, query, args...)
}
func (p *repositoryReadPool) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	return p.tx.Exec(ctx, query, args...)
}
func repositoryRead[T any](ctx context.Context, r *Repository, read func(*Repository) (T, error)) (T, error) {
	if _, bound := r.pool.(*repositoryReadPool); bound {
		return read(r)
	}
	var zero T
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return zero, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	view := &Repository{pool: &repositoryReadPool{databasePool: r.pool, tx: tx}}
	result, err := read(view)
	if err != nil {
		return zero, err
	}
	if err = tx.Commit(ctx); err != nil {
		return zero, err
	}
	return result, nil
}
func (r *Repository) RepositoryPageDetail(ctx context.Context, org, pageID string) (model.PageDetail, error) {
	return repositoryRead(ctx, r, func(view *Repository) (model.PageDetail, error) { return view.repositoryPageDetail(ctx, org, pageID) })
}
func (r *Repository) SearchRepositoryPages(ctx context.Context, org string, options model.SearchOptions) ([]model.SearchResult, error) {
	return repositoryRead(ctx, r, func(view *Repository) ([]model.SearchResult, error) {
		return view.searchRepositoryPages(ctx, org, options)
	})
}
func (r *Repository) RetrieveRepositoryPages(ctx context.Context, org, query string, limit int) ([]model.RetrievedDocument, error) {
	return repositoryRead(ctx, r, func(view *Repository) ([]model.RetrievedDocument, error) {
		return view.retrieveRepositoryPages(ctx, org, query, limit)
	})
}
