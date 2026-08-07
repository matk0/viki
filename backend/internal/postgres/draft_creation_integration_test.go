package postgres

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"viki/internal/model"
)

func TestAssistantFeatureCreationRequiresScenario(t *testing.T) {
	fixture := newFaultFixture(t)
	noun := model.ConceptNoun
	conceptKey := "customer"
	featureKey := "verify-consent"
	_, err := fixture.repository.ApplyAIChangeSet(fixture.ctx, fixture.organizationID, fixture.userID, model.AssistantMutationContext{
		ConversationID:  uuid.NewString(),
		TurnID:          uuid.NewString(),
		HermesProfile:   "viki-edit",
		HermesSessionID: "session-feature-without-scenario",
	}, model.AIChangeSet{Operations: []model.AIChangeOperation{
		{
			Operation: "create", ClientKey: conceptKey, Kind: model.PageConcept, ConceptKind: &noun,
			Slug: "customer-" + uuid.NewString(), Content: model.RevisionContent{Title: "Customer"},
		},
		{
			Operation: "create", ClientKey: featureKey, Kind: model.PageFeature,
			Slug: "verify-consent-" + uuid.NewString(), Content: model.RevisionContent{
				Title:      "Verify consent",
				References: []model.PageReference{{TargetClientKey: conceptKey, TargetTitle: "Customer", Relation: "uses"}},
			},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "feature requires at least one scenario") {
		t.Fatalf("feature-only assistant changeset error = %v, want required scenario error", err)
	}
}

func TestCreateFeatureIncludesInitialScenarioInSameTransaction(t *testing.T) {
	fixture := newFaultFixture(t)
	featureSlug := "feature-with-scenario-" + uuid.NewString()
	scenarioSlug := "initial-scenario-" + uuid.NewString()
	detail, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageFeature, Slug: featureSlug,
		Content: model.RevisionContent{Title: "Verify consent"},
		InitialScenario: &model.InitialScenarioInput{
			Slug: scenarioSlug,
			Content: model.RevisionContent{
				Title: "Customer grants consent",
				Steps: validScenarioSteps(),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Children) != 1 || detail.Children[0].Kind != model.PageScenario || detail.Children[0].ParentID == nil || *detail.Children[0].ParentID != detail.Page.ID {
		t.Fatalf("created feature children = %+v, want one independently versioned scenario", detail.Children)
	}
	var scenarioDrafts int
	if err := fixture.repository.pool.QueryRow(fixture.ctx, `
		SELECT count(*)
		FROM revisions r
		JOIN pages p ON p.id = r.page_id
		WHERE p.parent_id = $1 AND p.slug = $2 AND r.status = 'draft'
	`, detail.Page.ID, scenarioSlug).Scan(&scenarioDrafts); err != nil {
		t.Fatal(err)
	}
	if scenarioDrafts != 1 {
		t.Fatalf("initial scenario draft count = %d, want 1", scenarioDrafts)
	}
}

func TestCreatePageAlwaysStartsWithDraftRevision(t *testing.T) {
	fixture := newFaultFixture(t)
	noun := model.ConceptNoun
	concept, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageConcept, ConceptKind: &noun, Slug: "draft-concept-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Draft concept", Steps: []model.Step{}, References: []model.PageReference{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPageCreatedAsDraft(t, concept)

	feature, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageFeature, Slug: "draft-feature-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Draft feature", Steps: []model.Step{}, References: []model.PageReference{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPageCreatedAsDraft(t, feature)
	parentID := feature.Page.ID
	scenario, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: &parentID, Slug: "draft-scenario-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Draft scenario", Steps: scenarioSteps(), References: []model.PageReference{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPageCreatedAsDraft(t, scenario)
}

func TestAssistantChangeSetAlwaysCreatesDraftRevisions(t *testing.T) {
	fixture := newFaultFixture(t)
	tx, err := fixture.repository.pool.BeginTx(fixture.ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(fixture.ctx) }()
	createdIDs, err := fixture.repository.applyAIChangeSetTx(
		fixture.ctx,
		tx,
		fixture.organizationID,
		fixture.userID,
		simpleConceptChangeSet("assistant-draft-"+uuid.NewString()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(createdIDs) != 1 {
		t.Fatalf("created revision IDs = %v, want one", createdIDs)
	}
	var status model.RevisionStatus
	if err := tx.QueryRow(fixture.ctx, `SELECT status FROM revisions WHERE id = $1`, createdIDs[0]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != model.RevisionDraft {
		t.Fatalf("assistant revision status = %q, want %q", status, model.RevisionDraft)
	}
}

func assertPageCreatedAsDraft(t *testing.T, detail model.PageDetail) {
	t.Helper()
	if detail.ApprovedRevision != nil || detail.Page.ApprovedRevisionID != nil || detail.DraftRevision == nil || detail.DraftRevision.Status != model.RevisionDraft {
		t.Fatalf("created page bypassed draft lifecycle: %+v", detail)
	}
}
