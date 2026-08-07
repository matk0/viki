package postgres

import (
	"errors"
	"testing"

	"viki/internal/model"
	"viki/internal/store"
)

func validScenarioSteps() []model.Step {
	return []model.Step{
		{Keyword: model.KeywordGiven, Text: "precondition"},
		{Keyword: model.KeywordWhen, Text: "action"},
		{Keyword: model.KeywordThen, Text: "outcome"},
	}
}

func TestValidatePageInputCoversEveryKindAndHierarchyRule(t *testing.T) {
	t.Parallel()

	noun, verb, invalidConcept := model.ConceptNoun, model.ConceptVerb, model.ConceptKind("invalid")
	parent := "parent"
	validContent := model.RevisionContent{Title: "Title"}
	tests := []struct {
		name        string
		kind        model.PageKind
		conceptKind *model.ConceptKind
		parentID    *string
		content     model.RevisionContent
		wantErr     bool
	}{
		{name: "empty title", kind: model.PageConcept, conceptKind: &noun, content: model.RevisionContent{}, wantErr: true},
		{name: "concept missing subtype", kind: model.PageConcept, content: validContent, wantErr: true},
		{name: "concept invalid subtype", kind: model.PageConcept, conceptKind: &invalidConcept, content: validContent, wantErr: true},
		{name: "concept parent", kind: model.PageConcept, conceptKind: &noun, parentID: &parent, content: validContent, wantErr: true},
		{name: "concept steps", kind: model.PageConcept, conceptKind: &noun, content: model.RevisionContent{Title: "Title", Steps: validScenarioSteps()}, wantErr: true},
		{name: "noun", kind: model.PageConcept, conceptKind: &noun, content: validContent},
		{name: "verb", kind: model.PageConcept, conceptKind: &verb, content: validContent},
		{name: "feature subtype", kind: model.PageFeature, conceptKind: &noun, content: validContent, wantErr: true},
		{name: "feature parent", kind: model.PageFeature, parentID: &parent, content: validContent, wantErr: true},
		{name: "feature steps", kind: model.PageFeature, content: model.RevisionContent{Title: "Title", Steps: validScenarioSteps()}, wantErr: true},
		{name: "feature", kind: model.PageFeature, content: validContent},
		{name: "scenario subtype", kind: model.PageScenario, conceptKind: &noun, parentID: &parent, content: model.RevisionContent{Title: "Title", Steps: validScenarioSteps()}, wantErr: true},
		{name: "scenario missing parent", kind: model.PageScenario, content: model.RevisionContent{Title: "Title", Steps: validScenarioSteps()}, wantErr: true},
		{name: "scenario invalid steps", kind: model.PageScenario, parentID: &parent, content: validContent, wantErr: true},
		{name: "scenario", kind: model.PageScenario, parentID: &parent, content: model.RevisionContent{Title: "Title", Steps: validScenarioSteps()}},
		{name: "unknown kind", kind: model.PageKind("unknown"), content: validContent, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePageInput(test.kind, test.conceptKind, test.parentID, test.content)
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestValidateBDDStepsCoversOrderingContentAndKeywordRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		steps []model.Step
	}{
		{name: "too few", steps: validScenarioSteps()[:2]},
		{name: "empty text", steps: []model.Step{{Keyword: model.KeywordGiven, Text: " "}, {Keyword: model.KeywordWhen, Text: "w"}, {Keyword: model.KeywordThen, Text: "t"}}},
		{name: "given after when", steps: []model.Step{{Keyword: model.KeywordGiven, Text: "g"}, {Keyword: model.KeywordWhen, Text: "w"}, {Keyword: model.KeywordGiven, Text: "g2"}}},
		{name: "when before given", steps: []model.Step{{Keyword: model.KeywordWhen, Text: "w"}, {Keyword: model.KeywordThen, Text: "t"}, {Keyword: model.KeywordThen, Text: "t2"}}},
		{name: "when after then", steps: []model.Step{{Keyword: model.KeywordGiven, Text: "g"}, {Keyword: model.KeywordWhen, Text: "w"}, {Keyword: model.KeywordThen, Text: "t"}, {Keyword: model.KeywordWhen, Text: "w2"}}},
		{name: "then before when", steps: []model.Step{{Keyword: model.KeywordGiven, Text: "g"}, {Keyword: model.KeywordAnd, Text: "a"}, {Keyword: model.KeywordThen, Text: "t"}}},
		{name: "and first", steps: []model.Step{{Keyword: model.KeywordAnd, Text: "a"}, {Keyword: model.KeywordWhen, Text: "w"}, {Keyword: model.KeywordThen, Text: "t"}}},
		{name: "invalid keyword", steps: []model.Step{{Keyword: model.KeywordGiven, Text: "g"}, {Keyword: model.BDDKeyword("invalid"), Text: "x"}, {Keyword: model.KeywordThen, Text: "t"}}},
		{name: "missing phases", steps: []model.Step{{Keyword: model.KeywordGiven, Text: "g"}, {Keyword: model.KeywordAnd, Text: "a"}, {Keyword: model.KeywordBut, Text: "b"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateBDDSteps(test.steps); err == nil {
				t.Fatal("invalid steps were accepted")
			}
		})
	}
	valid := []model.Step{
		{Keyword: model.KeywordGiven, Text: "g"},
		{Keyword: model.KeywordAnd, Text: "a"},
		{Keyword: model.KeywordWhen, Text: "w"},
		{Keyword: model.KeywordBut, Text: "b"},
		{Keyword: model.KeywordThen, Text: "t"},
		{Keyword: model.KeywordAnd, Text: "a2"},
	}
	if err := validateBDDSteps(valid); err != nil {
		t.Fatalf("valid steps error=%v", err)
	}
	withDefinitions := []model.Step{
		{Keyword: model.KeywordGiven, DefinitionID: "definition-1"},
		{Keyword: model.KeywordWhen, Expression: "customer acts"},
		{Keyword: model.KeywordThen, DefinitionID: "definition-3"},
	}
	if err := validateBDDSteps(withDefinitions); err != nil {
		t.Fatalf("definition-backed steps error=%v", err)
	}
}

func TestStepRolesAndParameterRendering(t *testing.T) {
	t.Parallel()

	steps := []model.Step{
		{Keyword: model.KeywordGiven}, {Keyword: model.KeywordAnd},
		{Keyword: model.KeywordWhen}, {Keyword: model.KeywordBut},
		{Keyword: model.KeywordThen}, {Keyword: model.KeywordAnd},
	}
	wantRoles := []model.StepRole{model.StepContext, model.StepContext, model.StepAction, model.StepAction, model.StepOutcome, model.StepOutcome}
	for index, want := range wantRoles {
		if got := stepRole(steps, index); got != want {
			t.Fatalf("role %d = %s, want %s", index, got, want)
		}
	}
	if got := stepRole([]model.Step{{Keyword: model.KeywordAnd}}, 0); got != model.StepContext {
		t.Fatalf("fallback role = %s", got)
	}
	role := model.StepAction
	if stepRoleValue(nil) != "" || stepRoleValue(&role) != string(model.StepAction) {
		t.Fatal("step role query value mismatch")
	}

	tests := []struct {
		name       string
		expression string
		arguments  []string
		want       string
		wantErr    bool
	}{
		{name: "literal", expression: "  a contract exists  ", want: "a contract exists"},
		{name: "supported parameters", expression: "a {word} has {int} contracts named {string}", arguments: []string{"customer", "2", "internet"}, want: `a customer has 2 contracts named "internet"`},
		{name: "wrong count", expression: "a {word}", wantErr: true},
		{name: "invalid integer", expression: "there are {int}", arguments: []string{"two"}, wantErr: true},
		{name: "invalid word", expression: "a {word}", arguments: []string{"two words"}, wantErr: true},
		{name: "empty word", expression: "a {word}", arguments: []string{""}, wantErr: true},
		{name: "unsupported parameter", expression: "a {float}", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := renderStepExpression(test.expression, test.arguments)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("rendered=%q error=%v want=%q wantErr=%v", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestAssistantValidationAndOperationHelpersCoverEveryShape(t *testing.T) {
	t.Parallel()

	noun := model.ConceptNoun
	actualNoun, actualVerb := string(model.ConceptNoun), string(model.ConceptVerb)
	if !conceptKindMatches(nil, nil) || conceptKindMatches(nil, &actualNoun) || conceptKindMatches(&noun, nil) || !conceptKindMatches(&noun, &actualNoun) || conceptKindMatches(&noun, &actualVerb) {
		t.Fatal("concept kind matching failed")
	}
	value, same, other := "value", "value", "other"
	if !optionalStringMatches(nil, nil) || optionalStringMatches(nil, &value) || optionalStringMatches(&value, nil) || !optionalStringMatches(&value, &same) || optionalStringMatches(&value, &other) {
		t.Fatal("optional string matching failed")
	}

	pageID := "page-1"
	baseID := "revision-1"
	validConcept := model.AIChangeOperation{Operation: "create", ClientKey: "concept", Kind: model.PageConcept, ConceptKind: &noun, Content: model.RevisionContent{Title: "Concept"}}
	validFeature := model.AIChangeOperation{Operation: "create", ClientKey: "feature", Kind: model.PageFeature, Content: model.RevisionContent{Title: "Feature", References: []model.PageReference{{TargetClientKey: "concept", TargetTitle: "Concept", Relation: "uses"}}}}
	validScenario := model.AIChangeOperation{Operation: "create", ClientKey: "scenario", Kind: model.PageScenario, ParentClientKey: "feature", Content: model.RevisionContent{Title: "Scenario", Steps: validScenarioSteps(), References: []model.PageReference{{TargetClientKey: "concept", TargetTitle: "Concept", Relation: "uses"}}}}
	valid := model.AIChangeSet{Operations: []model.AIChangeOperation{validConcept, validFeature, validScenario}}
	if err := validateAssistantChangeSetShape(valid); err != nil {
		t.Fatalf("valid assistant shape error=%v", err)
	}

	tests := []model.AIChangeSet{
		{Operations: []model.AIChangeOperation{{Operation: "create", PageID: &pageID, Kind: model.PageConcept, ConceptKind: &noun, Content: model.RevisionContent{Title: "x"}}}},
		{Operations: []model.AIChangeOperation{{Operation: "revise", Kind: model.PageConcept, ConceptKind: &noun, Content: model.RevisionContent{Title: "x"}}}},
		{Operations: []model.AIChangeOperation{{Operation: "delete"}}},
		{Operations: []model.AIChangeOperation{{Operation: "create", Kind: model.PageScenario, ParentClientKey: "missing", Content: model.RevisionContent{Title: "x", Steps: validScenarioSteps()}}}},
		{Operations: []model.AIChangeOperation{{Operation: "create", Kind: model.PageConcept, ConceptKind: &noun, Content: model.RevisionContent{}}}},
		{Operations: []model.AIChangeOperation{{Operation: "create", Kind: model.PageFeature, Content: model.RevisionContent{Title: "x"}}}},
		{Operations: []model.AIChangeOperation{validConcept, {Operation: "create", Kind: model.PageFeature, Content: model.RevisionContent{Title: "x", References: []model.PageReference{{TargetClientKey: "concept", TargetTitle: "x", Relation: "uses"}}}}}},
		{Operations: []model.AIChangeOperation{validConcept, {Operation: "create", Kind: model.PageFeature, Content: model.RevisionContent{Title: "x", References: []model.PageReference{{TargetPageID: "page", TargetClientKey: "concept", TargetTitle: "x", Relation: "uses"}}}}}},
		{Operations: []model.AIChangeOperation{validConcept, {Operation: "create", Kind: model.PageFeature, Content: model.RevisionContent{Title: "x", References: []model.PageReference{{TargetPageID: "page", TargetTitle: "x"}}}}}},
		{Operations: []model.AIChangeOperation{validConcept, {Operation: "create", Kind: model.PageFeature, Content: model.RevisionContent{Title: "x", References: []model.PageReference{{TargetClientKey: "missing", TargetTitle: "x", Relation: "uses"}}}}}},
		{Operations: []model.AIChangeOperation{validConcept, validConcept}},
	}
	for index, invalid := range tests {
		if err := validateAssistantChangeSetShape(invalid); err == nil {
			t.Fatalf("invalid assistant shape %d was accepted", index)
		}
	}
	validRevision := model.AIChangeSet{Operations: []model.AIChangeOperation{{Operation: "revise", PageID: &pageID, BaseRevisionID: &baseID, Kind: model.PageConcept, ConceptKind: &noun, Content: model.RevisionContent{Title: "x"}}}}
	if err := validateAssistantChangeSetShape(validRevision); err != nil {
		t.Fatalf("valid revision shape error=%v", err)
	}
}

func TestRequireUpdatedCoversDatabaseErrorNotFoundAndSuccess(t *testing.T) {
	t.Parallel()
	want := errors.New("database error")
	if !errors.Is(requireUpdated(1, want), want) || !errors.Is(requireUpdated(0, nil), store.ErrNotFound) || requireUpdated(1, nil) != nil {
		t.Fatal("requireUpdated result mismatch")
	}
}
