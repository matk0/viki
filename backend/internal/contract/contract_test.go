package contract_test

import (
	"os"
	"strings"
	"testing"
)

func TestOpenAPIContractContainsCoreSurface(t *testing.T) {
	content, err := os.ReadFile("../../../openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract := string(content)
	required := []string{
		"openapi: 3.1.0",
		"/api/v1/auth/login:",
		"/api/v1/pages/{pageId}/revisions:",
		"/api/v1/assistant/status:",
		"/api/v1/assistant/conversations/{conversationId}/messages:",
		"/api/v1/assistant/conversations/{conversationId}/events:",
		"/api/v1/assistant/conversations/{conversationId}/stop:",
		"/api/v1/assistant/conversations/{conversationId}/clarifications/{requestId}:",
		"/api/v1/draft-proposals:",
		"PageKind:",
		"enum: [concept, feature, scenario]",
		"ConceptKind:",
		"enum: [given, when, then, and, but]",
	}
	for _, value := range required {
		if !strings.Contains(contract, value) {
			t.Errorf("OpenAPI contract is missing %q", value)
		}
	}
	for _, removed := range []string{"/api/v1/chats", "/api/v1/transcriptions"} {
		if strings.Contains(contract, removed) {
			t.Errorf("OpenAPI contract still contains removed surface %q", removed)
		}
	}
}

func TestGeneratedTypesContainContractEnums(t *testing.T) {
	t.Parallel()
	goTypes, err := os.ReadFile("../api/openapi.gen.go")
	if err != nil {
		t.Fatal(err)
	}
	typescriptTypes, err := os.ReadFile("../../../frontend/src/api/generated.ts")
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name     string
		content  string
		required []string
	}{
		{"Go", string(goTypes), []string{"type PageKind string", `PageKind = "concept"`, `PageKind = "feature"`, `PageKind = "scenario"`, "type ConceptKind string", "type BDDKeyword string", `BDDKeyword = "given"`, `BDDKeyword = "but"`, "type AssistantConversation struct", "type AssistantStatus struct"}},
		{"TypeScript", string(typescriptTypes), []string{`PageKind: "concept" | "feature" | "scenario"`, `ConceptKind: "noun" | "verb"`, `BDDKeyword: "given" | "when" | "then" | "and" | "but"`, `AssistantMode: "qa" | "edit"`, `AssistantConversation:`, `AssistantStatus:`}},
	}
	for _, check := range checks {
		for _, value := range check.required {
			if !strings.Contains(check.content, value) {
				t.Errorf("generated %s types are missing %q", check.name, value)
			}
		}
	}
}

func TestCoverageCommandsArePartOfTheRepositoryContract(t *testing.T) {
	t.Parallel()

	checks := []struct {
		path     string
		required []string
	}{
		{
			path: "../../../Makefile",
				required: []string{
					"test-coverage:",
					"test-coverage-backend:",
					"test-coverage-frontend:",
					"test-coverage-hermes:",
					"reset-test-database:",
					"-coverpkg=./...",
					"viki_test?sslmode=disable",
				},
		},
		{
			path:     "../../../frontend/package.json",
			required: []string{`"test:coverage"`, `"@vitest/coverage-v8"`},
		},
		{
			path: "../../../frontend/vite.config.ts",
			required: []string{
				"provider: 'v8'",
				"reportsDirectory:",
				"thresholds:",
			},
		},
	}

	for _, check := range checks {
		content, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range check.required {
			if !strings.Contains(string(content), value) {
				t.Errorf("%s is missing coverage contract %q", check.path, value)
			}
		}
	}
}
