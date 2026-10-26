package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"viki/internal/model"
	"viki/internal/specsync"
	"viki/internal/store"
)

func syncFixture(t *testing.T) (*Repository, *pgxpool.Pool, *databaseFault, string) {
	t.Helper()
	r, pool, fault := openFaultTestRepository(t)
	org := uuid.NewString()
	_, err := pool.Exec(context.Background(), `INSERT INTO organizations(id,name) VALUES($1,'Repository import test')`, org)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, query := range []string{`UPDATE workspace_repository_sync SET current_snapshot_id=NULL WHERE organization_id=$1`, `DELETE FROM repository_snapshot_entries WHERE page_id IN(SELECT id FROM repository_pages WHERE organization_id=$1)`, `UPDATE repository_pages SET current_revision_id=NULL WHERE organization_id=$1`, `DELETE FROM repository_revisions WHERE page_id IN(SELECT id FROM repository_pages WHERE organization_id=$1)`, `DELETE FROM repository_pages WHERE organization_id=$1`, `DELETE FROM repository_snapshots WHERE organization_id=$1`, `DELETE FROM workspace_repository_sync WHERE organization_id=$1`, `DELETE FROM organizations WHERE id=$1`} {
			_, _ = pool.Exec(ctx, query, org)
		}
	})
	if err = r.ConfigureRepositoryWorkspace(context.Background(), org, specsync.Repository); err != nil {
		t.Fatal(err)
	}
	return r, pool, fault, org
}
func eligibleFixture(t *testing.T) specsync.Snapshot {
	t.Helper()
	b, err := os.ReadFile("../specsync/testdata/snapshot-v1.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := specsync.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	s.Source.Commit = strings.Repeat("1", 40)
	s.Source.Dirty = false
	s.Verification.SourceCommit = s.Source.Commit
	s.Publication = specsync.Publication{Kind: "main", Event: "push", Ref: "refs/heads/main", Workflow: specsync.Workflow, RunID: "10", RunAttempt: 1, Eligible: true}
	s.Verification.MessagesHash = "sha256:" + strings.Repeat("a", 64)
	return s
}
func importOK(t *testing.T, r *Repository, org string, s specsync.Snapshot) model.WorkspaceSyncStatus {
	t.Helper()
	status, err := r.ImportRepositorySnapshot(context.Background(), org, s)
	if err != nil {
		t.Fatal(err)
	}
	return status
}
func revisionCount(t *testing.T, pool *pgxpool.Pool, org string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM repository_revisions r JOIN repository_pages p ON p.id=r.page_id WHERE p.organization_id=$1`, org).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
func updateCatalogueHash(s *specsync.Snapshot) {
	for i := range s.Documents {
		s.Documents[i].ContentHash = specsync.Hash([]byte(s.Documents[i].Content))
	}
	s.Source.ContentHash = specsync.CatalogueHash(s.Documents)
	s.Verification.SourceContentHash = s.Source.ContentHash
}
func addSecondScenario(s *specsync.Snapshot) {
	sc := s.Scenarios[0]
	sc.ID = "VOZ-SC-CONSENT-002"
	sc.Name = "Another consent outcome"
	sc.Line = 13
	sc.Steps = append([]specsync.Step{}, sc.Steps...)
	for i := range sc.Steps {
		sc.Steps[i].Line = 14 + i
	}
	s.Scenarios = append(s.Scenarios, sc)
	result := s.Verification.Cases[0]
	result.ScenarioID = sc.ID
	s.Verification.Cases = append(s.Verification.Cases, result)
	s.Documents[1].Content += "\n  @id:VOZ-SC-CONSENT-002\n  Scenario: Another consent outcome\n    Given a farmer is signed in\n    When the farmer adds a customer with existing paper consent\n    Then the customer is subscribed with recorded paper consent\n    And no confirmation is requested\n"
	updateCatalogueHash(s)
}
func TestRepositorySnapshotLifecycleAndReadOnly(t *testing.T) {
	r, pool, _, org := syncFixture(t)
	ctx := context.Background()
	before, err := r.WorkspaceSyncStatus(ctx, org)
	if err != nil || before.Mode != "git_managed" || !before.Stale {
		t.Fatalf("initial=%+v err=%v", before, err)
	}
	s := eligibleFixture(t)
	status := importOK(t, r, org, s)
	if status.PageCount != 3 || status.ScenarioCount != 1 || status.Stale {
		t.Fatal(status)
	}
	if revisionCount(t, pool, org) != 3 {
		t.Fatal("initial revisions")
	}
	importOK(t, r, org, s)
	if revisionCount(t, pool, org) != 3 {
		t.Fatal("retry duplicated revisions")
	}
	pages, err := r.ListPages(ctx, org, nil)
	if err != nil || len(pages) != 3 {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
	var scenarioPage model.Page
	for _, p := range pages {
		if p.Kind == model.PageScenario {
			scenarioPage = p
			if p.ParentID == nil || p.Approved || p.HasDraft || p.ApprovedRevisionID != nil {
				t.Fatal(p)
			}
		}
	}
	id := *scenarioPage.RepositoryRevisionID
	detail, err := r.PageDetail(ctx, org, scenarioPage.ID)
	if err != nil || detail.RepositoryRevision == nil || detail.ApprovedRevision != nil || detail.RepositoryRevision.ApprovedAt != nil || len(detail.RepositoryRevision.Steps) != 4 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	version, err := r.Revision(ctx, org, id)
	if err != nil || version.Status != model.RevisionRepositoryBaseline || version.RepositorySource.VerifiedCommit != s.Source.Commit {
		t.Fatalf("version=%+v err=%v", version, err)
	}
	results, err := r.SearchPages(ctx, org, model.SearchOptions{Query: "consent", Limit: 1})
	if err != nil || len(results) != 1 {
		t.Fatalf("search=%+v err=%v", results, err)
	}
	empty, err := r.SearchPages(ctx, org, model.SearchOptions{Query: "unmatchable"})
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	documents, err := r.Retrieve(ctx, org, "consent", false, 0)
	if err != nil || len(documents) == 0 || documents[0].Draft || !strings.Contains(documents[0].Content, "not human approval") {
		t.Fatal(documents, err)
	}
	if _, err = r.CreatePage(ctx, org, uuid.NewString(), model.CreatePageInput{}); !errors.Is(err, store.ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err = r.SaveRevision(ctx, org, uuid.NewString(), scenarioPage.ID, model.SaveRevisionInput{}); !errors.Is(err, store.ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err = r.ApproveRevision(ctx, org, uuid.NewString(), id); !errors.Is(err, store.ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err = r.ApplyAIChangeSet(ctx, org, uuid.NewString(), model.AssistantMutationContext{}, model.AIChangeSet{}); !errors.Is(err, store.ErrReadOnly) {
		t.Fatal(err)
	}
	// Only provenance changes: same immutable business revisions, updated exact CI/source evidence.
	s.Source.Commit = strings.Repeat("2", 40)
	s.Verification.SourceCommit = s.Source.Commit
	s.Publication.RunID = "11"
	s.Verification.Cases[0].Duration = "2000000000"
	importOK(t, r, org, s)
	if revisionCount(t, pool, org) != 3 {
		t.Fatal("metadata created business revisions")
	}
	version, err = r.Revision(ctx, org, id)
	if err != nil || version.RepositorySource.Commit != s.Source.Commit {
		t.Fatal(version, err)
	}
	// A move keeps stable IDs and the same business content.
	s.Documents[1].Path = "specs/business/moved-consent.feature"
	s.Publication.RunID = "12"
	updateCatalogueHash(&s)
	importOK(t, r, org, s)
	if revisionCount(t, pool, org) != 3 {
		t.Fatal("move duplicated business revisions")
	}
	// A real name/content change creates new versions while preserving the old revision.
	oldName := s.Scenarios[0].Name
	s.Scenarios[0].Name = "Record existing paper consent"
	s.Documents[1].Content = strings.Replace(s.Documents[1].Content, oldName, s.Scenarios[0].Name, 1)
	s.Publication.RunID = "13"
	updateCatalogueHash(&s)
	importOK(t, r, org, s)
	if revisionCount(t, pool, org) != 5 {
		t.Fatal("changed content missing revisions")
	}
	old, err := r.Revision(ctx, org, id)
	if err != nil || old.Title != oldName {
		t.Fatal(old, err)
	}
	if err = r.RecordRepositoryPoll(ctx, org, "artifact_unavailable"); err != nil {
		t.Fatal(err)
	}
	status, err = r.WorkspaceSyncStatus(ctx, org)
	if err != nil || !status.Stale || status.LastError == "" || status.RunID != "13" {
		t.Fatal(status, err)
	}
	// Stale, malformed and same-run conflicts preserve the last good baseline.
	bad := s
	bad.Publication.RunID = "12"
	if _, err = r.ImportRepositorySnapshot(ctx, org, bad); err == nil {
		t.Fatal("accepted stale")
	}
	bad = s
	bad.Verification.FinishedAt = "bad"
	if _, err = r.ImportRepositorySnapshot(ctx, org, bad); err == nil {
		t.Fatal("accepted malformed")
	}
	bad = s
	bad.Verification.Cases = append([]specsync.Result{}, s.Verification.Cases...)
	bad.Verification.Cases[0].Duration = "3"
	if _, err = r.ImportRepositorySnapshot(ctx, org, bad); err == nil {
		t.Fatal("accepted same-run conflict")
	}
	importOK(t, r, org, s)
	status, _ = r.WorkspaceSyncStatus(ctx, org)
	if status.LastError != "" {
		t.Fatal("successful retry did not clear error")
	}
	unknown := uuid.NewString()
	if _, err = r.PageDetail(ctx, org, unknown); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err = r.Revision(ctx, org, unknown); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	other := uuid.NewString()
	if _, err = r.Revision(ctx, other, id); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("cross-workspace history access", err)
	}
}
func TestRepositoryRemovalAdditionReappearanceAndConcurrentImports(t *testing.T) {
	r, pool, _, org := syncFixture(t)
	ctx := context.Background()
	base := eligibleFixture(t)
	withSecond := eligibleFixture(t)
	addSecondScenario(&withSecond)
	importOK(t, r, org, withSecond)
	pages, _ := r.ListPages(ctx, org, nil)
	var removed model.Page
	for _, p := range pages {
		if p.Slug == "VOZ-SC-CONSENT-002" {
			removed = p
		}
	}
	if removed.ID == "" {
		t.Fatal("missing added scenario")
	}
	base.Publication.RunID = "11"
	importOK(t, r, org, base)
	_, _ = r.SearchPages(ctx, org, model.SearchOptions{Query: "consent"})
	current, _ := r.ListPages(ctx, org, nil)
	if len(current) != 3 {
		t.Fatal(current)
	}
	history, err := r.PageDetail(ctx, org, removed.ID)
	if err != nil || !history.Page.Archived || len(history.Revisions) != 1 {
		t.Fatal(history, err)
	}
	documents, _ := r.Retrieve(ctx, org, "Another consent outcome", false, 10)
	if len(documents) != 0 {
		t.Fatal("removed scenario appears in Q&A", documents)
	}
	count := revisionCount(t, pool, org)
	withSecond.Publication.RunID = "12"
	importOK(t, r, org, withSecond)
	if revisionCount(t, pool, org) != count {
		t.Fatal("reappearance duplicated historical content")
	}
	older := withSecond
	newer := withSecond
	older.Publication.RunID = "19"
	newer.Publication.RunID = "20"
	var wg sync.WaitGroup
	for _, s := range []specsync.Snapshot{older, newer} {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = r.ImportRepositorySnapshot(ctx, org, s) }()
	}
	wg.Wait()
	status, _ := r.WorkspaceSyncStatus(ctx, org)
	if status.RunID != "20" {
		t.Fatal(status)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	newer.Publication.RunID = "21"
	if _, err = r.ImportRepositorySnapshot(canceled, org, newer); err == nil {
		t.Fatal("canceled import succeeded")
	}
	status, _ = r.WorkspaceSyncStatus(ctx, org)
	if status.RunID != "20" {
		t.Fatal("interrupted import changed baseline")
	}
}
func TestRepositoryImportDatabaseInterruptionKeepsLastGoodSnapshot(t *testing.T) {
	for _, faultCase := range []struct{ method, query string }{{"begin", ""}, {"tx_query_row", "SELECT repository"}, {"tx_query_row", "SELECT run_id"}, {"tx_query_row", "INSERT INTO repository_snapshots"}, {"tx_exec", "UPDATE repository_pages SET archived"}, {"tx_query_row", "INSERT INTO repository_pages"}, {"tx_query_row", "SELECT id::text FROM repository_revisions"}, {"tx_query_row", "INSERT INTO repository_revisions"}, {"tx_exec", "INSERT INTO repository_snapshot_entries"}, {"tx_exec", "UPDATE repository_pages SET current_revision"}, {"tx_exec", "UPDATE workspace_repository_sync SET current_snapshot"}, {"commit", ""}} {
		t.Run(faultCase.method+faultCase.query, func(t *testing.T) {
			r, pool, fault, org := syncFixture(t)
			s := eligibleFixture(t)
			importOK(t, r, org, s)
			s.Publication.RunID = "11"
			s.Documents[0].Content += "\nAdditional business prose.\n"
			updateCatalogueHash(&s)
			fault.activate(faultCase.method, faultCase.query, 1)
			if _, err := r.ImportRepositorySnapshot(context.Background(), org, s); err == nil {
				t.Fatal("interrupted import succeeded")
			}
			fault.activate("disabled", "", 1)
			status, err := r.WorkspaceSyncStatus(context.Background(), org)
			if err != nil || status.RunID != "10" || status.PageCount != 3 || revisionCount(t, pool, org) != 3 {
				t.Fatal("partial import visible", status, err)
			}
		})
	}
}
func TestRepositoryPollingFreshnessAndModeConfiguration(t *testing.T) {
	r, pool, _, org := syncFixture(t)
	ctx := context.Background()
	if err := r.ConfigureRepositoryWorkspace(ctx, org, "https://unrelated.example/repo"); err == nil {
		t.Fatal("unrelated repository accepted")
	}
	if err := r.ConfigureInitialRepositoryWorkspace(ctx, ""); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `UPDATE workspace_repository_sync SET last_checked_at=$2 WHERE organization_id=$1`, org, time.Now().Add(-16*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	status, err := r.WorkspaceSyncStatus(ctx, org)
	if err != nil || !status.Stale {
		t.Fatal(status, err)
	}
	if err = r.RecordRepositoryPoll(ctx, org, ""); err != nil {
		t.Fatal(err)
	}
	status, _ = r.WorkspaceSyncStatus(ctx, org)
	if status.Stale {
		t.Fatal(status)
	}
	human, _ := r.WorkspaceSyncStatus(ctx, uuid.NewString())
	if human.Mode != "human_governed" {
		t.Fatal(human)
	}
	raw, _ := json.Marshal(eligibleFixture(t))
	if len(raw) == 0 {
		t.Fatal("fixture")
	}
}

func TestRepositoryReadFailuresAndIdempotentCommitInterruption(t *testing.T) {
	r, _, fault, org := syncFixture(t)
	ctx := context.Background()
	s := eligibleFixture(t)
	importOK(t, r, org, s)
	pages, _ := r.RepositoryPages(ctx, org, nil)
	pageID := pages[0].ID
	revisionID := *pages[0].RepositoryRevisionID
	operations := []struct {
		name, method, query string
		run                 func() error
	}{
		{"configure exec", "exec", "INSERT INTO workspace_repository_sync", func() error { return r.ConfigureRepositoryWorkspace(ctx, org, specsync.Repository) }},
		{"configure status", "query_row", "SELECT w.repository", func() error { return r.ConfigureRepositoryWorkspace(ctx, org, specsync.Repository) }},
		{"workspace status", "query_row", "SELECT w.repository", func() error { _, err := r.WorkspaceSyncStatus(ctx, org); return err }},
		{"duplicate update", "tx_exec", "UPDATE workspace_repository_sync SET last_checked_at", func() error { _, err := r.ImportRepositorySnapshot(ctx, org, s); return err }},
		{"duplicate commit", "commit", "", func() error { _, err := r.ImportRepositorySnapshot(ctx, org, s); return err }},
		{"versions query", "query", "FROM repository_revisions r", func() error { _, err := r.repositoryVersions(ctx, org, pageID, ""); return err }},
		{"versions scan", "rows_scan", "FROM repository_revisions r", func() error { _, err := r.repositoryVersions(ctx, org, pageID, ""); return err }},
		{"catalogue query", "query", "FROM repository_pages p JOIN workspace", func() error { _, err := r.RepositoryPages(ctx, org, nil); return err }},
		{"catalogue scan", "rows_scan", "FROM repository_pages p JOIN workspace", func() error { _, err := r.RepositoryPages(ctx, org, nil); return err }},
		{"catalogue rows", "rows_error", "FROM repository_pages p JOIN workspace", func() error { _, err := r.RepositoryPages(ctx, org, nil); return err }},
		{"detail query", "tx_query_row", "SELECT p.id::text,p.kind", func() error { _, err := r.RepositoryPageDetail(ctx, org, pageID); return err }},
		{"detail versions", "tx_query", "FROM repository_revisions r", func() error { _, err := r.RepositoryPageDetail(ctx, org, pageID); return err }},
		{"detail children", "tx_query", "FROM repository_pages p JOIN workspace", func() error { _, err := r.RepositoryPageDetail(ctx, org, pageID); return err }},
		{"revision query", "query", "FROM repository_revisions r", func() error { _, err := r.RepositoryRevision(ctx, org, revisionID); return err }},
		{"search catalogue", "tx_query", "FROM repository_pages p JOIN workspace", func() error { _, err := r.SearchRepositoryPages(ctx, org, model.SearchOptions{}); return err }},
		{"search versions", "tx_query", "FROM repository_revisions r", func() error { _, err := r.SearchRepositoryPages(ctx, org, model.SearchOptions{}); return err }},
		{"retrieve search", "tx_query", "FROM repository_pages p JOIN workspace", func() error { _, err := r.RetrieveRepositoryPages(ctx, org, "consent", 1); return err }},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			fault.activate(op.method, op.query, 1)
			err := op.run()
			fault.activate("disabled", "", 1)
			if err == nil {
				t.Fatal("database interruption hidden")
			}
		})
	}
	fault.activate("tx_query", "FROM repository_revisions r", len(pages)+1)
	_, err := r.RetrieveRepositoryPages(ctx, org, "", 20)
	fault.activate("disabled", "", 1)
	if err == nil {
		t.Fatal("retrieval revision failure hidden")
	}
	for _, method := range []string{"ListPages", "SearchPages", "Retrieve", "Revision"} {
		fault.activate("query_row", "SELECT EXISTS", 1)
		var err error
		switch method {
		case "ListPages":
			_, err = r.ListPages(ctx, org, nil)
		case "SearchPages":
			_, err = r.SearchPages(ctx, org, model.SearchOptions{})
		case "Retrieve":
			_, err = r.Retrieve(ctx, org, "", false, 10)
		case "Revision":
			_, err = r.Revision(ctx, org, revisionID)
		}
		fault.activate("disabled", "", 1)
		if err == nil {
			t.Fatal(method, "unknown authority did not fail closed")
		}
	}
}
func TestRepositoryCorruptStorageAndHumanModeActivationSafety(t *testing.T) {
	r, pool, _, org := syncFixture(t)
	ctx := context.Background()
	s := eligibleFixture(t)
	importOK(t, r, org, s)
	pages, _ := r.RepositoryPages(ctx, org, nil)
	id := pages[0].ID
	if _, err := r.ImportRepositorySnapshot(ctx, org, specsync.Snapshot{}); err == nil {
		t.Fatal("ineligible import accepted")
	}
	_, _ = pool.Exec(ctx, `UPDATE workspace_repository_sync SET repository='https://other.example/repo' WHERE organization_id=$1`, org)
	if _, err := r.ImportRepositorySnapshot(ctx, org, s); err == nil {
		t.Fatal("workspace repository mismatch")
	}
	_, _ = pool.Exec(ctx, `UPDATE workspace_repository_sync SET repository=$2 WHERE organization_id=$1`, org, specsync.Repository)
	_, _ = pool.Exec(ctx, `UPDATE repository_snapshots SET snapshot='{"format":[]}'::jsonb WHERE organization_id=$1`, org)
	if _, err := r.ImportRepositorySnapshot(ctx, org, s); err == nil {
		t.Fatal("corrupt run receipt accepted")
	}
	_, _ = pool.Exec(ctx, `UPDATE repository_snapshot_entries SET entry='"invalid"'::jsonb WHERE page_id=$1`, id)
	if _, err := r.RepositoryPages(ctx, org, nil); err == nil {
		t.Fatal("corrupt catalogue accepted")
	}
	if _, err := r.repositoryVersions(ctx, org, id, ""); err == nil {
		t.Fatal("corrupt version accepted")
	}
	_, _ = pool.Exec(ctx, `UPDATE repository_revisions SET entry='"invalid"'::jsonb WHERE page_id=$1`, id)
	if _, err := r.RepositoryPageDetail(ctx, org, id); err == nil {
		t.Fatal("corrupt page accepted")
	}
	human := newFaultFixture(t)
	if err := human.repository.ConfigureRepositoryWorkspace(ctx, human.organizationID, specsync.Repository); err == nil {
		t.Fatal("existing human catalogue hidden by Git mode")
	}
	for _, op := range []func() error{func() error { _, err := human.repository.ListPages(ctx, human.organizationID, nil); return err }, func() error {
		_, err := human.repository.SearchPages(ctx, human.organizationID, model.SearchOptions{})
		return err
	}, func() error {
		_, err := human.repository.Retrieve(ctx, human.organizationID, "", false, 10)
		return err
	}} {
		human.fault.activate("query", "FROM pages p", 1)
		if err := op(); err == nil {
			t.Fatal("human database query failure hidden")
		}
		human.fault.activate("disabled", "", 1)
	}
	human.fault.activate("query_row", "SELECT EXISTS(\n", 1)
	if _, err := human.repository.Revision(ctx, human.organizationID, human.revisionID); err == nil {
		t.Fatal("human revision lookup error hidden")
	}
	human.fault.activate("disabled", "", 1)
	if _, err := pool.Exec(ctx, `INSERT INTO pages(organization_id,kind,concept_kind,slug,created_by) VALUES($1,'concept','noun','forbidden',$2)`, org, uuid.NewString()); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatal("database trigger did not reject canonical fork", err)
	}
}
