package postgres_test

import (
	"testing"

	"github.com/google/uuid"

	"viki/internal/model"
)

func TestApprovedScenarioPublishesReusableStepDefinitions(t *testing.T) {
	fixture := newDevelopmentFixture(t)

	definitions, err := fixture.repository.ListStepDefinitions(fixture.ctx, fixture.organizationID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 0 {
		t.Fatalf("draft definitions in reusable catalog = %d, want 0", len(definitions))
	}

	approved, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.scenario.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err = fixture.repository.ListStepDefinitions(fixture.ctx, fixture.organizationID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 3 {
		t.Fatalf("approved definitions = %d, want 3", len(definitions))
	}

	byRole := map[model.StepRole]model.StepDefinition{}
	for _, definition := range definitions {
		byRole[definition.Role] = definition
	}
	for _, role := range []model.StepRole{model.StepContext, model.StepAction, model.StepOutcome} {
		if byRole[role].ID == "" || !byRole[role].Approved || byRole[role].UsageCount != 1 {
			t.Fatalf("definition for %s = %+v", role, byRole[role])
		}
	}

	parentID := approved.Page.ParentID
	second, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: parentID, Slug: "reused-steps-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Another contract scenario", References: []model.PageReference{},
			Steps: []model.Step{
				{Keyword: model.KeywordGiven, DefinitionID: byRole[model.StepContext].ID},
				{Keyword: model.KeywordWhen, DefinitionID: byRole[model.StepAction].ID},
				{Keyword: model.KeywordThen, DefinitionID: byRole[model.StepOutcome].ID},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, step := range second.DraftRevision.Steps {
		if step.DefinitionID == "" || step.Text == "" {
			t.Fatalf("reused step %d = %+v", index, step)
		}
	}
	definitions, err = fixture.repository.ListStepDefinitions(fixture.ctx, fixture.organizationID, "CONTRACT", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) == 0 || definitions[0].UsageCount != 1 {
		t.Fatalf("draft reuse changed approved usage: %+v", definitions)
	}
	if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, second.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	definitions, err = fixture.repository.ListStepDefinitions(fixture.ctx, fixture.organizationID, "contract", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		if definition.UsageCount != 2 {
			t.Fatalf("approved usage for %q = %d, want 2", definition.Expression, definition.UsageCount)
		}
	}
}
