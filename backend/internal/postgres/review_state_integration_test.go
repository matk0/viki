package postgres_test

import (
	"testing"

	"github.com/google/uuid"

	"viki/internal/model"
)

func TestPageDetailDerivesReadinessAndTypedReviewBlockers(t *testing.T) {
	fixture := newRepositoryFixture(t)
	feature, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind:    model.PageFeature,
		Slug:    "review-feature-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Rezervácia služby", Steps: []model.Step{}, References: []model.PageReference{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind:     model.PageScenario,
		ParentID: &feature.Page.ID,
		Slug:     "review-scenario-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Zákazník rezervuje službu", References: []model.PageReference{},
			Steps: []model.Step{{Keyword: model.KeywordGiven, Text: "zákazník existuje"}, {Keyword: model.KeywordWhen, Text: "rezervuje službu"}, {Keyword: model.KeywordThen, Text: "rezervácia vznikne"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	blocked, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, scenario.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	review := reviewStateForRevision(t, blocked, scenario.DraftRevision.ID)
	if review.State != model.ReviewBlocked || len(review.Blockers) != 1 || review.Blockers[0].Type != model.BlockerParentFeature || review.Blockers[0].RelatedPageID == nil || *review.Blockers[0].RelatedPageID != feature.Page.ID || review.Blockers[0].RelatedPageTitle == nil || *review.Blockers[0].RelatedPageTitle != "Rezervácia služby" {
		t.Fatalf("parent review state = %+v", review)
	}

	if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, feature.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	ready, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, scenario.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if review := reviewStateForRevision(t, ready, scenario.DraftRevision.ID); review.State != model.ReviewReady || len(review.Blockers) != 0 {
		t.Fatalf("ready review state = %+v", review)
	}

	objection, err := fixture.repository.AddObjection(fixture.ctx, fixture.organizationID, fixture.userID, scenario.DraftRevision.ID, "Chýba potvrdenie dostupnosti")
	if err != nil {
		t.Fatal(err)
	}
	if objection.ID == "" || objection.RevisionID != scenario.DraftRevision.ID {
		t.Fatalf("created objection = %+v", objection)
	}
	withObjection, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, scenario.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(withObjection.Comments) != 0 {
		t.Fatalf("objection leaked into discussion comments: %+v", withObjection.Comments)
	}
	review = reviewStateForRevision(t, withObjection, scenario.DraftRevision.ID)
	if review.State != model.ReviewBlocked || len(review.Blockers) != 1 {
		t.Fatalf("objection review state = %+v", review)
	}
	blocker := review.Blockers[0]
	if blocker.Type != model.BlockerObjection || blocker.ID != objection.ID || blocker.SourceRevisionID == nil || *blocker.SourceRevisionID != scenario.DraftRevision.ID || blocker.SourceRevisionNumber == nil || *blocker.SourceRevisionNumber != scenario.DraftRevision.Number || blocker.Body == nil || *blocker.Body != "Chýba potvrdenie dostupnosti" || blocker.Author == nil || blocker.Author.ID != fixture.userID {
		t.Fatalf("objection blocker = %+v", blocker)
	}

	if _, err := fixture.repository.ResolveObjection(fixture.ctx, fixture.organizationID, fixture.userID, objection.ID); err != nil {
		t.Fatal(err)
	}
	resolved, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, scenario.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if review := reviewStateForRevision(t, resolved, scenario.DraftRevision.ID); review.State != model.ReviewReady || len(review.Blockers) != 0 {
		t.Fatalf("resolved review state = %+v", review)
	}
	if len(resolved.Objections) != 1 {
		t.Fatalf("resolved objections = %+v", resolved.Objections)
	}
	retained := resolved.Objections[0]
	if retained.ID != objection.ID || retained.RevisionID != scenario.DraftRevision.ID || retained.RevisionNumber != scenario.DraftRevision.Number || retained.ResolvedAt == nil || retained.ResolvedBy == nil || retained.ResolvedBy.ID != fixture.userID {
		t.Fatalf("retained resolved objection = %+v", retained)
	}
}

func reviewStateForRevision(t *testing.T, detail model.PageDetail, revisionID string) model.RevisionReviewState {
	t.Helper()
	for _, review := range detail.ReviewStates {
		if review.RevisionID == revisionID {
			return review
		}
	}
	t.Fatalf("review state for revision %s is missing: %+v", revisionID, detail.ReviewStates)
	return model.RevisionReviewState{}
}
