package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"viki/internal/model"
	"viki/internal/specsync"
)

// Commit a real import after a request establishes its first database snapshot.
// The import uses the original pool, independently of the request's transaction.
type repositoryReadHookPool struct {
	databasePool
	afterFirstRead func() error
	begins         int
	fired          bool
	err            error
}

func (p *repositoryReadHookPool) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	p.begins++
	tx, err := p.databasePool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &repositoryReadHookTx{Tx: tx, pool: p}, nil
}

func (p *repositoryReadHookPool) afterRead() {
	if !p.fired {
		p.fired = true
		p.err = p.afterFirstRead()
	}
}

type repositoryReadHookTx struct {
	pgx.Tx
	pool *repositoryReadHookPool
}

func (tx *repositoryReadHookTx) QueryRow(ctx context.Context, query string, arguments ...any) pgx.Row {
	return &repositoryReadHookRow{Row: tx.Tx.QueryRow(ctx, query, arguments...), pool: tx.pool}
}

func (tx *repositoryReadHookTx) Query(ctx context.Context, query string, arguments ...any) (pgx.Rows, error) {
	rows, err := tx.Tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	return &repositoryReadHookRows{Rows: rows, pool: tx.pool}, nil
}

type repositoryReadHookRow struct {
	pgx.Row
	pool *repositoryReadHookPool
}

func (row *repositoryReadHookRow) Scan(destinations ...any) error {
	if err := row.Row.Scan(destinations...); err != nil {
		return err
	}
	row.pool.afterRead()
	return row.pool.err
}

type repositoryReadHookRows struct {
	pgx.Rows
	pool *repositoryReadHookPool
}

func (rows *repositoryReadHookRows) Close() {
	rows.Rows.Close()
	rows.pool.afterRead()
}

func replacementReadSnapshot(t *testing.T) specsync.Snapshot {
	t.Helper()
	s := eligibleFixture(t)
	s.Publication.RunID = "11"
	s.Source.Commit = strings.Repeat("2", 40)
	s.Verification.SourceCommit = s.Source.Commit
	s.Documents[1].Title = "Updated customer consent"
	s.Documents[1].Content = strings.Replace(s.Documents[1].Content, "Feature: Customer consent", "Feature: "+s.Documents[1].Title, 1)
	oldName := s.Scenarios[0].Name
	s.Scenarios[0].Name = "Updated paper consent outcome"
	s.Documents[1].Content = strings.Replace(s.Documents[1].Content, oldName, s.Scenarios[0].Name, 1)
	addSecondScenario(&s)
	return s
}

func TestRepositoryDetailRetainsOneBaselineWhenImportCommitsBetweenQueries(t *testing.T) {
	r, _, _, org := syncFixture(t)
	ctx := context.Background()
	base := eligibleFixture(t)
	importOK(t, r, org, base)
	kind := model.PageFeature
	pages, err := r.RepositoryPages(ctx, org, &kind)
	if err != nil || len(pages) != 1 {
		t.Fatalf("feature catalogue=%+v err=%v", pages, err)
	}
	feature := pages[0]
	next := replacementReadSnapshot(t)
	hook := &repositoryReadHookPool{databasePool: r.pool, afterFirstRead: func() error {
		_, err := r.ImportRepositorySnapshot(ctx, org, next)
		return err
	}}
	reader := &Repository{pool: hook}
	detail, err := reader.PageDetail(ctx, org, feature.ID)
	if err != nil || !hook.fired || hook.err != nil || hook.begins != 1 {
		t.Fatalf("interleaved request err=%v fired=%v importErr=%v transactions=%d", err, hook.fired, hook.err, hook.begins)
	}
	if detail.Page.Title != base.Documents[1].Title || detail.RepositoryRevision == nil || detail.RepositoryRevision.ID != *feature.RepositoryRevisionID || detail.RepositoryRevision.RepositorySource.RunID != "10" || len(detail.Revisions) != 1 || len(detail.Children) != 1 || detail.Children[0].Title != base.Scenarios[0].Name || detail.Children[0].RepositorySource.RunID != "10" {
		t.Fatalf("one detail mixed old header with newer history/children: %+v", detail)
	}
	detail, err = reader.PageDetail(ctx, org, feature.ID)
	if err != nil || hook.begins != 2 || detail.Page.Title != next.Documents[1].Title || detail.RepositoryRevision == nil || detail.RepositoryRevision.RepositorySource.RunID != "11" || len(detail.Revisions) != 2 || len(detail.Children) != 2 {
		t.Fatalf("next detail did not observe committed import: detail=%+v err=%v transactions=%d", detail, err, hook.begins)
	}
	for _, child := range detail.Children {
		if child.RepositorySource.RunID != "11" {
			t.Fatalf("new detail has old child provenance: %+v", child)
		}
	}
}

func TestRepositorySearchAndNestedRetrievalRetainOneBaselineDuringImport(t *testing.T) {
	for _, operation := range []string{"search", "retrieve"} {
		t.Run(operation, func(t *testing.T) {
			r, _, _, org := syncFixture(t)
			ctx := context.Background()
			base := eligibleFixture(t)
			importOK(t, r, org, base)
			next := replacementReadSnapshot(t)
			hook := &repositoryReadHookPool{databasePool: r.pool, afterFirstRead: func() error {
				_, err := r.ImportRepositorySnapshot(ctx, org, next)
				return err
			}}
			reader := &Repository{pool: hook}
			for index, wantRun := range []string{"10", "11"} {
				wantCount := 3 + index
				switch operation {
				case "search":
					results, err := reader.SearchPages(ctx, org, model.SearchOptions{})
					if err != nil || len(results) != wantCount {
						t.Fatalf("run%s search count=%d err=%v", wantRun, len(results), err)
					}
					for _, result := range results {
						if result.Page.RepositorySource.RunID != wantRun || result.Page.RepositoryRevisionID == nil || result.RevisionID != *result.Page.RepositoryRevisionID || (index == 0 && strings.Contains(result.Page.Title, "Updated")) {
							t.Fatalf("search mixed baselines in run%s: %+v", wantRun, result)
						}
					}
				case "retrieve":
					documents, err := reader.Retrieve(ctx, org, "", false, 20)
					if err != nil || len(documents) != wantCount {
						t.Fatalf("run%s retrieval count=%d err=%v", wantRun, len(documents), err)
					}
					commit := base.Source.Commit
					if index == 1 {
						commit = next.Source.Commit
					}
					for _, document := range documents {
						if !strings.Contains(document.Content, "source commit "+commit+"; verified at "+commit+";") || (index == 0 && strings.Contains(document.Content, "Updated")) {
							t.Fatalf("nested retrieval mixed baselines in run%s: %+v", wantRun, document)
						}
					}
				}
				if !hook.fired || hook.err != nil || hook.begins != index+1 {
					t.Fatalf("request did not reuse one read view: fired=%v importErr=%v transactions=%d", hook.fired, hook.err, hook.begins)
				}
			}
		})
	}
}

func TestRepositoryReadTransactionsDiscardResultsOnDatabaseFailures(t *testing.T) {
	r, _, fault, org := syncFixture(t)
	ctx := context.Background()
	importOK(t, r, org, eligibleFixture(t))
	pages, err := r.RepositoryPages(ctx, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []struct {
		name       string
		readMethod string
		run        func() (bool, error)
	}{
		{"detail", "tx_query_row", func() (bool, error) {
			detail, err := r.RepositoryPageDetail(ctx, org, pages[0].ID)
			return detail.Page.ID == "" && detail.RepositoryRevision == nil && detail.Revisions == nil, err
		}},
		{"search", "tx_query", func() (bool, error) {
			results, err := r.SearchRepositoryPages(ctx, org, model.SearchOptions{})
			return results == nil, err
		}},
		{"retrieve", "tx_query", func() (bool, error) {
			results, err := r.RetrieveRepositoryPages(ctx, org, "", 20)
			return results == nil, err
		}},
	} {
		for _, stage := range []string{"begin_tx", operation.readMethod, "commit"} {
			t.Run(operation.name+"/"+stage, func(t *testing.T) {
				fault.activate(stage, "", 1)
				zero, err := operation.run()
				requireInjectedFailure(t, err)
				if !zero {
					t.Fatal("failed read transaction returned partial/uncommitted result")
				}
				fault.activate("disabled", "", 1)
				zero, err = operation.run()
				if err != nil || zero {
					t.Fatalf("reader did not recover after rolled back transaction: zero=%v err=%v", zero, err)
				}
			})
		}
	}
}

func TestRepositoryReadViewRejectsAccidentalWrites(t *testing.T) {
	r, _, _, org := syncFixture(t)
	ctx := context.Background()
	importOK(t, r, org, eligibleFixture(t))
	_, err := repositoryRead(ctx, r, func(view *Repository) (bool, error) {
		var isolation, readOnly string
		if err := view.pool.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
			return false, err
		}
		if err := view.pool.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&readOnly); err != nil {
			return false, err
		}
		if isolation != "repeatable read" || readOnly != "on" {
			t.Fatalf("unexpected database read view: isolation=%q readOnly=%q", isolation, readOnly)
		}
		return false, view.RecordRepositoryPoll(ctx, org, "accidental_write")
	})
	var postgresErr *pgconn.PgError
	if !errors.As(err, &postgresErr) || postgresErr.Code != "25006" {
		t.Fatalf("read view permitted write or returned wrong error: %v", err)
	}
	status, err := r.WorkspaceSyncStatus(ctx, org)
	if err != nil || status.LastError != "" || status.RunID != "10" {
		t.Fatalf("read view write altered workspace: status=%+v err=%v", status, err)
	}
}
