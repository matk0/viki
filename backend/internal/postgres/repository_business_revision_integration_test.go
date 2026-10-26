package postgres

import (
	"context"
	"strings"
	"testing"

	"viki/internal/model"
	"viki/internal/specsync"
)

func TestRepositoryScenarioEditPreservesSiblingBusinessRevisionAndRefreshesItsSource(t *testing.T) {
	r, pool, _, org := syncFixture(t)
	ctx := context.Background()
	base := eligibleFixture(t)
	addSecondScenario(&base)
	importOK(t, r, org, base)
	pages, err := r.RepositoryPages(ctx, org, nil)
	if err != nil || len(pages) != 4 || revisionCount(t, pool, org) != 4 {
		t.Fatalf("initial two-scenario baseline: pages=%+v err=%v", pages, err)
	}
	before := map[string]model.Page{}
	for _, page := range pages {
		before[page.Slug] = page
	}

	next := eligibleFixture(t)
	addSecondScenario(&next)
	next.Publication.RunID = "11"
	next.Source.Commit = strings.Repeat("2", 40)
	next.Verification.SourceCommit = next.Source.Commit
	next.Verification.MessagesHash = "sha256:" + strings.Repeat("b", 64)
	next.Verification.FinishedAt = "2026-08-21T12:00:02.000Z"
	oldStep := next.Scenarios[0].Steps[1].Text
	newStep := "the farmer imports a customer with existing paper consent"
	next.Scenarios[0].Steps[1].Text = newStep
	next.Documents[1].Content = strings.Replace(next.Documents[1].Content, oldStep, newStep, 1)
	// The fixture helper shares result-step storage; this execution changed only
	// the edited scenario, so preserve the sibling's independent passed receipt.
	next.Verification.Cases[0].Steps = append([]specsync.ResultStep{}, next.Verification.Cases[0].Steps...)
	next.Verification.Cases[0].Steps[1].Text = newStep
	next.Verification.Cases[0].Duration = "2000000000"
	updateCatalogueHash(&next)
	if base.Documents[1].ContentHash == next.Documents[1].ContentHash {
		t.Fatal("scenario edit did not change the containing feature file hash")
	}
	importOK(t, r, org, next)

	pages, err = r.RepositoryPages(ctx, org, nil)
	if err != nil || len(pages) != 4 || revisionCount(t, pool, org) != 6 {
		t.Fatalf("expected exactly feature and edited-scenario revisions: pages=%+v err=%v", pages, err)
	}
	for _, page := range pages {
		original := before[page.Slug]
		if page.ID != original.ID || page.RepositoryRevisionID == nil || original.RepositoryRevisionID == nil {
			t.Fatalf("import changed stable page identity: before=%+v after=%+v", original, page)
		}
		changed := page.Slug == next.Documents[1].ID || page.Slug == next.Scenarios[0].ID
		wantRevisions := 1
		if changed {
			wantRevisions = 2
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM repository_revisions WHERE page_id=$1`, page.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != wantRevisions || (*page.RepositoryRevisionID != *original.RepositoryRevisionID) != changed {
			t.Fatalf("business revision changed for wrong page: slug=%s count=%d before=%s after=%s", page.Slug, count, *original.RepositoryRevisionID, *page.RepositoryRevisionID)
		}
	}

	sibling := before[next.Scenarios[1].ID]
	version, err := r.Revision(ctx, org, *sibling.RepositoryRevisionID)
	if err != nil || version.ID != *sibling.RepositoryRevisionID || version.Number != 1 || version.Title != base.Scenarios[1].Name || len(version.Steps) != len(base.Scenarios[1].Steps) || version.Steps[1].Text != oldStep {
		t.Fatalf("untouched sibling business content changed: revision=%+v err=%v", version, err)
	}
	source := version.RepositorySource
	if source == nil || source.Content != next.Documents[1].Content || source.ContentHash != next.Documents[1].ContentHash || source.Path != next.Documents[1].Path || source.Commit != next.Source.Commit || source.VerifiedCommit != next.Source.Commit || source.RunID != "11" || source.RunAttempt != 1 {
		t.Fatalf("stable sibling revision has stale containing-file provenance: %+v", source)
	}
	if !strings.Contains(source.Content, newStep) || !strings.Contains(source.Content, "Scenario: "+base.Scenarios[1].Name) {
		t.Fatal("updated source does not include both the edited scenario and untouched sibling")
	}
}
