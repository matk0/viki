package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"viki/internal/model"
	"viki/internal/specsync"
)

func TestRepositoryRejectedSnapshotPreservesBaselineWithoutOpeningTransaction(t *testing.T) {
	r, pool, fault, org := syncFixture(t)
	ctx := context.Background()
	importOK(t, r, org, eligibleFixture(t))
	for _, test := range []struct {
		name   string
		mutate func(*specsync.Snapshot)
	}{
		{"wire schema", func(s *specsync.Snapshot) { s.SchemaVersion = 2 }},
		{"untrusted publication", func(s *specsync.Snapshot) { s.Publication.Eligible = false }},
		{"source content mismatch", func(s *specsync.Snapshot) { s.Documents[0].Content += "Unverified alteration\n" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := eligibleFixture(t)
			s.Publication.RunID = "11"
			test.mutate(&s)
			fault.activate("begin", "", 1)
			_, err := r.ImportRepositorySnapshot(ctx, org, s)
			if err == nil || errors.Is(err, errInjectedDatabase) || fault.hits != 0 {
				t.Fatalf("invalid snapshot reached database transaction: err=%v begins=%d", err, fault.hits)
			}
			fault.activate("disabled", "", 1)
			status, err := r.WorkspaceSyncStatus(ctx, org)
			if err != nil || status.RunID != "10" || status.PageCount != 3 || revisionCount(t, pool, org) != 3 {
				t.Fatalf("rejected snapshot changed baseline: status=%+v err=%v", status, err)
			}
		})
	}
}

func TestRepositoryAuthorityLookupFailuresDoNotReachReadsOrMutations(t *testing.T) {
	r, pool, fault, org := syncFixture(t)
	ctx := context.Background()
	importOK(t, r, org, eligibleFixture(t))
	pages, err := r.RepositoryPages(ctx, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	pageID, revisionID := pages[0].ID, *pages[0].RepositoryRevisionID
	userID := uuid.NewString()
	for _, test := range []struct {
		name string
		run  func() error
	}{
		{"list", func() error { _, err := r.ListPages(ctx, org, nil); return err }},
		{"search", func() error { _, err := r.SearchPages(ctx, org, model.SearchOptions{}); return err }},
		{"retrieve", func() error { _, err := r.Retrieve(ctx, org, "", true, 10); return err }},
		{"detail", func() error { _, err := r.PageDetail(ctx, org, pageID); return err }},
		{"revision", func() error { _, err := r.Revision(ctx, org, revisionID); return err }},
		{"create", func() error { _, err := r.CreatePage(ctx, org, userID, model.CreatePageInput{}); return err }},
		{"save", func() error {
			_, err := r.SaveRevision(ctx, org, userID, pageID, model.SaveRevisionInput{})
			return err
		}},
		{"approve", func() error { _, err := r.ApproveRevision(ctx, org, userID, revisionID); return err }},
		{"assistant changes", func() error {
			_, err := r.ApplyAIChangeSet(ctx, org, userID, model.AssistantMutationContext{}, model.AIChangeSet{Clarification: "No mutation needed"})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fault.activate("query_row", "SELECT EXISTS(SELECT 1 FROM workspace_repository_sync", 1)
			requireInjectedFailure(t, test.run())
			if fault.hits != 1 {
				t.Fatalf("authority lookup count=%d", fault.hits)
			}
			fault.activate("disabled", "", 1)
		})
	}
	var humanPages int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pages WHERE organization_id=$1`, org).Scan(&humanPages); err != nil {
		t.Fatal(err)
	}
	if humanPages != 0 || revisionCount(t, pool, org) != 3 {
		t.Fatalf("unknown authority allowed mutation: human pages=%d", humanPages)
	}
}

func TestRepositoryRestoredBusinessRevisionControlsChildrenSearchAndRetrieval(t *testing.T) {
	r, _, _, org := syncFixture(t)
	ctx := context.Background()
	base := eligibleFixture(t)
	importOK(t, r, org, base)
	pages, err := r.RepositoryPages(ctx, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	var feature, scenario model.Page
	for _, page := range pages {
		switch page.Kind {
		case model.PageFeature:
			feature = page
		case model.PageScenario:
			scenario = page
		}
	}
	if feature.ID == "" || scenario.ID == "" {
		t.Fatalf("missing feature or scenario in fixture: %+v", pages)
	}
	detail, err := r.PageDetail(ctx, org, feature.ID)
	if err != nil || len(detail.Children) != 1 || detail.Children[0].ID != scenario.ID {
		t.Fatalf("feature children=%+v err=%v", detail.Children, err)
	}
	changed := eligibleFixture(t)
	changed.Publication.RunID = "11"
	changed.Scenarios[0].Name = "Temporary consent outcome"
	changed.Documents[1].Content = strings.Replace(changed.Documents[1].Content, base.Scenarios[0].Name, changed.Scenarios[0].Name, 1)
	updateCatalogueHash(&changed)
	importOK(t, r, org, changed)
	base.Publication.RunID = "12"
	importOK(t, r, org, base)
	detail, err = r.PageDetail(ctx, org, scenario.ID)
	if err != nil || len(detail.Revisions) != 2 || detail.RepositoryRevision == nil || detail.RepositoryRevision.ID != *scenario.RepositoryRevisionID {
		t.Fatalf("restored baseline lost immutable history: detail=%+v err=%v", detail, err)
	}
	results, err := r.SearchPages(ctx, org, model.SearchOptions{Query: "Temporary"})
	if err != nil || len(results) != 0 {
		t.Fatalf("superseded title remains searchable: results=%+v err=%v", results, err)
	}
	documents, err := r.Retrieve(ctx, org, base.Scenarios[0].Name, true, 1)
	if err != nil || len(documents) != 1 || documents[0].RevisionID != *scenario.RepositoryRevisionID || strings.Contains(documents[0].Content, changed.Scenarios[0].Name) {
		t.Fatalf("retrieval did not use restored baseline: documents=%+v err=%v", documents, err)
	}
}

func TestRepositoryDuplicateImportRollbackPreservesPollFailure(t *testing.T) {
	for _, test := range []struct{ name, method, query string }{
		{"receipt update", "tx_exec", "UPDATE workspace_repository_sync SET last_checked_at"},
		{"commit", "commit", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, pool, fault, org := syncFixture(t)
			ctx := context.Background()
			s := eligibleFixture(t)
			importOK(t, r, org, s)
			if err := r.RecordRepositoryPoll(ctx, org, "artifact_unavailable"); err != nil {
				t.Fatal(err)
			}
			before, err := r.WorkspaceSyncStatus(ctx, org)
			if err != nil {
				t.Fatal(err)
			}
			fault.activate(test.method, test.query, 1)
			_, err = r.ImportRepositorySnapshot(ctx, org, s)
			requireInjectedFailure(t, err)
			fault.activate("disabled", "", 1)
			after, err := r.WorkspaceSyncStatus(ctx, org)
			if err != nil || !after.Stale || after.LastError != before.LastError || after.LastErrorAt == nil || !after.LastErrorAt.Equal(*before.LastErrorAt) || after.LastCheckedAt == nil || !after.LastCheckedAt.Equal(*before.LastCheckedAt) || revisionCount(t, pool, org) != 3 {
				t.Fatalf("failed retry erased poll evidence: before=%+v after=%+v err=%v", before, after, err)
			}
			status := importOK(t, r, org, s)
			if status.Stale || status.LastError != "" || status.LastErrorAt != nil {
				t.Fatalf("successful retry did not recover: %+v", status)
			}
		})
	}
}

func TestRepositoryImportCancellationWhileWaitingForWorkspaceLock(t *testing.T) {
	r, pool, _, org := syncFixture(t)
	ctx := context.Background()
	s := eligibleFixture(t)
	importOK(t, r, org, s)
	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Rollback(ctx) }()
	if _, err := lock.Exec(ctx, `SELECT organization_id FROM workspace_repository_sync WHERE organization_id=$1 FOR UPDATE`, org); err != nil {
		t.Fatal(err)
	}
	importCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.Publication.RunID = "11"
	result := make(chan error, 1)
	go func() { _, err := r.ImportRepositorySnapshot(importCtx, org, s); result <- err }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT repository,current_snapshot_id%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("import never waited for the workspace row lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled lock waiter error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled import remained blocked on workspace lock")
	}
	if err := lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := r.WorkspaceSyncStatus(ctx, org)
	if err != nil || status.RunID != "10" || revisionCount(t, pool, org) != 3 {
		t.Fatalf("canceled waiter changed visible baseline: status=%+v err=%v", status, err)
	}
	status = importOK(t, r, org, s)
	if status.RunID != "11" || revisionCount(t, pool, org) != 3 {
		t.Fatalf("canceled waiter prevented recovery: %+v", status)
	}
}
