package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"viki/internal/model"
	"viki/internal/postgres"
)

func TestFeatureAndScenarioRevisionLifecyclesAreIndependent(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	organizationID, userID := uuid.NewString(), uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Independent revisions')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Reviewer', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `UPDATE pages SET approved_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	feature, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{
		Kind: model.PageFeature,
		Slug: "feature-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Contract generation", Steps: []model.Step{}, References: []model.PageReference{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	feature, err = repository.ApproveRevision(ctx, organizationID, userID, feature.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	featureRevisionOne := feature.ApprovedRevision.ID

	scenario, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{
		Kind:     model.PageScenario,
		ParentID: &feature.Page.ID,
		Slug:     "scenario-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Customer signs a contract", References: []model.PageReference{},
			Steps: []model.Step{
				{Keyword: model.KeywordGiven, Text: "a contract is ready"},
				{Keyword: model.KeywordWhen, Text: "the customer signs it"},
				{Keyword: model.KeywordThen, Text: "the signature is recorded"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scenario, err = repository.ApproveRevision(ctx, organizationID, userID, scenario.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	scenarioRevisionOne := scenario.ApprovedRevision.ID

	featureDraft, err := repository.SaveRevision(ctx, organizationID, userID, feature.Page.ID, model.SaveRevisionInput{
		BaseRevisionID: featureRevisionOne,
		Content: model.RevisionContent{
			Title: "Contract generation v2", Steps: []model.Step{}, References: []model.PageReference{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	feature, err = repository.ApproveRevision(ctx, organizationID, userID, featureDraft.ID)
	if err != nil {
		t.Fatal(err)
	}
	scenarioAfterFeatureApproval, err := repository.PageDetail(ctx, organizationID, scenario.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scenarioAfterFeatureApproval.ApprovedRevision.ID != scenarioRevisionOne || scenarioAfterFeatureApproval.DraftRevision != nil {
		t.Fatalf("feature approval changed scenario lifecycle: %+v", scenarioAfterFeatureApproval)
	}

	scenarioDraft, err := repository.SaveRevision(ctx, organizationID, userID, scenario.Page.ID, model.SaveRevisionInput{
		BaseRevisionID: scenarioRevisionOne,
		Content: model.RevisionContent{
			Title: "Customer signs a contract v2", References: []model.PageReference{},
			Steps: []model.Step{
				{Keyword: model.KeywordGiven, Text: "a revised contract is ready"},
				{Keyword: model.KeywordWhen, Text: "the customer signs it"},
				{Keyword: model.KeywordThen, Text: "the revised signature is recorded"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApproveRevision(ctx, organizationID, userID, scenarioDraft.ID); err != nil {
		t.Fatal(err)
	}
	featureAfterScenarioApproval, err := repository.PageDetail(ctx, organizationID, feature.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if featureAfterScenarioApproval.ApprovedRevision.ID != featureDraft.ID || featureAfterScenarioApproval.DraftRevision != nil {
		t.Fatalf("scenario approval changed feature lifecycle: %+v", featureAfterScenarioApproval)
	}

	assertRevisionStatus(t, featureAfterScenarioApproval.Revisions, featureRevisionOne, model.RevisionSuperseded)
	assertRevisionStatus(t, featureAfterScenarioApproval.Revisions, featureDraft.ID, model.RevisionApproved)
	scenarioAfterApproval, err := repository.PageDetail(ctx, organizationID, scenario.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRevisionStatus(t, scenarioAfterApproval.Revisions, scenarioRevisionOne, model.RevisionSuperseded)
	assertRevisionStatus(t, scenarioAfterApproval.Revisions, scenarioDraft.ID, model.RevisionApproved)
}

func assertRevisionStatus(t *testing.T, revisions []model.RevisionSummary, revisionID string, status model.RevisionStatus) {
	t.Helper()
	for _, revision := range revisions {
		if revision.ID == revisionID {
			if revision.Status != status {
				t.Fatalf("revision %s status = %s, want %s", revisionID, revision.Status, status)
			}
			return
		}
	}
	t.Fatalf("revision %s not found in history", revisionID)
}
