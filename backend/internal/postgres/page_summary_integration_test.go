package postgres_test

import (
	"testing"

	"github.com/google/uuid"

	"viki/internal/model"
)

func TestPageSummariesExposeApprovedAndDraftTitles(t *testing.T) {
	fixture := newRepositoryFixture(t)
	noun := model.ConceptNoun
	approvedTitle := "Approved contract title"
	draftTitle := "Draft contract title"
	page := fixture.createApprovedPage(t, model.CreatePageInput{
		Kind:        model.PageConcept,
		ConceptKind: &noun,
		Slug:        "contract-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: approvedTitle, Steps: []model.Step{}, References: []model.PageReference{},
		},
	})
	if _, err := fixture.repository.SaveRevision(fixture.ctx, fixture.organizationID, fixture.userID, page.Page.ID, model.SaveRevisionInput{
		BaseRevisionID: page.ApprovedRevision.ID,
		Content: model.RevisionContent{
			Title: draftTitle, Steps: []model.Step{}, References: []model.PageReference{},
		},
	}); err != nil {
		t.Fatal(err)
	}

	pages, err := fixture.repository.ListPages(fixture.ctx, fixture.organizationID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("pages=%+v", pages)
	}
	summary := pages[0]
	if summary.Title != draftTitle {
		t.Fatalf("default title=%q, want active draft title %q", summary.Title, draftTitle)
	}
	if summary.ApprovedRevisionTitle == nil || *summary.ApprovedRevisionTitle != approvedTitle {
		t.Fatalf("approved title=%v, want %q", summary.ApprovedRevisionTitle, approvedTitle)
	}
	if summary.DraftRevisionTitle == nil || *summary.DraftRevisionTitle != draftTitle {
		t.Fatalf("draft title=%v, want %q", summary.DraftRevisionTitle, draftTitle)
	}

	results, err := fixture.repository.SearchPages(fixture.ctx, fixture.organizationID, model.SearchOptions{IncludeDrafts: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("search results=%+v", results)
	}
	for _, result := range results {
		wantTitle := approvedTitle
		if result.Draft {
			wantTitle = draftTitle
		}
		if result.Page.Title != wantTitle {
			t.Fatalf("search revision title=%q, want %q for draft=%t", result.Page.Title, wantTitle, result.Draft)
		}
		if result.Page.ApprovedRevisionTitle == nil || *result.Page.ApprovedRevisionTitle != approvedTitle || result.Page.DraftRevisionTitle == nil || *result.Page.DraftRevisionTitle != draftTitle {
			t.Fatalf("search page summary omitted revision titles: %+v", result.Page)
		}
	}
}
