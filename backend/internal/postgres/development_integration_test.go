package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"viki/internal/model"
	"viki/internal/postgres"
	"viki/internal/store"
)

func TestApprovingScenarioQueuesItsExactRevisionForDevelopment(t *testing.T) {
	fixture := newDevelopmentFixture(t)

	var queued int
	if err := fixture.connection.QueryRow(fixture.ctx, `SELECT count(*) FROM scenario_developments`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("feature approval queued %d development items, want 0", queued)
	}

	if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.scenario.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	var revisionID, status string
	if err := fixture.connection.QueryRow(fixture.ctx, `SELECT revision_id::text, status FROM scenario_developments`).Scan(&revisionID, &status); err != nil {
		t.Fatal(err)
	}
	if revisionID != fixture.scenario.DraftRevision.ID || status != "queued" {
		t.Fatalf("development = revision %q status %q, want revision %q status queued", revisionID, status, fixture.scenario.DraftRevision.ID)
	}
}

func TestDeveloperClaimsAndCompletesOneScenario(t *testing.T) {
	fixture := newDevelopmentFixture(t)
	approved, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.scenario.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}

	task, err := fixture.repository.ClaimScenarioDevelopment(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != model.DevelopmentRunning || task.Scenario.ID != approved.ApprovedRevision.ID || len(task.Scenario.Steps) != 3 {
		t.Fatalf("claimed task = %+v", task)
	}
	if _, err := fixture.repository.ClaimScenarioDevelopment(fixture.ctx); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second claim error = %v, want not found", err)
	}

	development, err := fixture.repository.CompleteScenarioDevelopment(fixture.ctx, task.RevisionID, "mock-receipt-1")
	if err != nil {
		t.Fatal(err)
	}
	if development.Status != model.DevelopmentDeveloped || development.Detail != "mock-receipt-1" {
		t.Fatalf("completed development = %+v", development)
	}
	detail, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, fixture.scenario.Page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Development == nil || detail.Development.Status != model.DevelopmentDeveloped {
		t.Fatalf("page development = %+v", detail.Development)
	}
}

func TestDeveloperCompletionTargetsItsClaimedRevision(t *testing.T) {
	fixture := newDevelopmentFixture(t)
	if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.scenario.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	firstTask, err := fixture.repository.ClaimScenarioDevelopment(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondScenario, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: &fixture.featureID, Slug: "second-development-scenario-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Second development scenario", References: []model.PageReference{}, Steps: developmentSteps()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, secondScenario.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	secondTask, err := fixture.repository.ClaimScenarioDevelopment(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if secondTask.RevisionID == firstTask.RevisionID {
		t.Fatalf("second claim reused first revision %q", secondTask.RevisionID)
	}
	if _, err := fixture.repository.CompleteScenarioDevelopment(fixture.ctx, secondTask.RevisionID, "mock-receipt-2"); err != nil {
		t.Fatal(err)
	}
	var firstStatus, secondStatus string
	if err := fixture.connection.QueryRow(fixture.ctx, `SELECT status FROM scenario_developments WHERE revision_id = $1`, firstTask.RevisionID).Scan(&firstStatus); err != nil {
		t.Fatal(err)
	}
	if err := fixture.connection.QueryRow(fixture.ctx, `SELECT status FROM scenario_developments WHERE revision_id = $1`, secondTask.RevisionID).Scan(&secondStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus != string(model.DevelopmentRunning) || secondStatus != string(model.DevelopmentDeveloped) {
		t.Fatalf("completion changed statuses first=%q second=%q", firstStatus, secondStatus)
	}
}

func TestFeatureDevelopmentProgressCountsOnlyApprovedScenarios(t *testing.T) {
	fixture := newDevelopmentFixture(t)
	if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.scenario.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	task, err := fixture.repository.ClaimScenarioDevelopment(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.CompleteScenarioDevelopment(fixture.ctx, task.RevisionID, "mock-receipt-1"); err != nil {
		t.Fatal(err)
	}

	approvedScenario, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: &fixture.featureID, Slug: "approved-scenario-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Approved scenario", References: []model.PageReference{}, Steps: developmentSteps()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, approvedScenario.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: &fixture.featureID, Slug: "draft-scenario-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Draft scenario", References: []model.PageReference{}, Steps: developmentSteps()},
	}); err != nil {
		t.Fatal(err)
	}

	detail, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, fixture.featureID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.DevelopmentProgress == nil || detail.DevelopmentProgress.Developed != 1 || detail.DevelopmentProgress.Total != 2 {
		t.Fatalf("feature development progress = %+v, want 1/2", detail.DevelopmentProgress)
	}
}

type developmentFixture struct {
	ctx            context.Context
	repository     *postgres.Repository
	connection     *pgx.Conn
	organizationID string
	userID         string
	featureID      string
	scenario       model.PageDetail
}

func newDevelopmentFixture(t *testing.T) developmentFixture {
	t.Helper()
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Migrate(ctx); err != nil {
		repository.Close()
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		repository.Close()
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `DELETE FROM scenario_developments`); err != nil {
		connection.Close(ctx)
		repository.Close()
		t.Fatal(err)
	}

	organizationID, userID := uuid.NewString(), uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Development queue')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Developer test', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = connection.Exec(ctx, `DELETE FROM scenario_developments WHERE revision_id IN (SELECT r.id FROM revisions r JOIN pages p ON p.id = r.page_id WHERE p.organization_id = $1)`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `UPDATE pages SET approved_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
		connection.Close(ctx)
		repository.Close()
	})

	feature, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{
		Kind: model.PageFeature,
		Slug: "development-feature-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Contract generation", Steps: []model.Step{}, References: []model.PageReference{},
		},
		InitialScenario: &model.InitialScenarioInput{
			Slug: "development-scenario-" + uuid.NewString(),
			Content: model.RevisionContent{
				Title: "Customer signs a contract", References: []model.PageReference{},
				Steps: []model.Step{
					{Keyword: model.KeywordGiven, Text: "a contract is ready"},
					{Keyword: model.KeywordWhen, Text: "the customer signs it"},
					{Keyword: model.KeywordThen, Text: "the signature is recorded"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApproveRevision(ctx, organizationID, userID, feature.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	var scenarioPageID string
	if err := connection.QueryRow(ctx, `SELECT id::text FROM pages WHERE parent_id = $1`, feature.Page.ID).Scan(&scenarioPageID); err != nil {
		t.Fatal(err)
	}
	scenario, err := repository.PageDetail(ctx, organizationID, scenarioPageID)
	if err != nil {
		t.Fatal(err)
	}
	return developmentFixture{
		ctx: ctx, repository: repository, connection: connection,
		organizationID: organizationID, userID: userID, featureID: feature.Page.ID, scenario: scenario,
	}
}

func developmentSteps() []model.Step {
	return []model.Step{
		{Keyword: model.KeywordGiven, Text: "a contract is ready"},
		{Keyword: model.KeywordWhen, Text: "the customer signs it"},
		{Keyword: model.KeywordThen, Text: "the signature is recorded"},
	}
}
