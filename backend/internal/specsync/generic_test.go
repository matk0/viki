package specsync

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Exercise the generic contract using the existing consent fixture. This is a
// consumer unit test, not validation of a second application's integration.
func genericConsentFixture(t *testing.T) Snapshot {
	t.Helper()
	raw, _ := json.Marshal(mainFixture(t))
	text := strings.NewReplacer(
		"VOZ-CON-CONSENT", "concept.consent",
		"VOZ-FEAT-CONSENT", "feature.consent",
		"VOZ-SC-CONSENT-001", "scenario.consent.001",
		"specs/business/", "specifications/",
	).Replace(string(raw))
	var s Snapshot
	if err := json.Unmarshal([]byte(text), &s); err != nil {
		t.Fatal(err)
	}
	s.Format = "gherkin-specification"
	s.SchemaVersion = 2
	s.Verification.SchemaVersion = 2
	s.Verification.Backend = "application-owned"
	s.Verification.Runner.Name = "cucumber-compatible-runner"
	s.Verification.Command = "application-owned acceptance command"
	rehash(&s)
	return s
}

func rehash(s *Snapshot) {
	for i := range s.Documents {
		s.Documents[i].ContentHash = Hash([]byte(s.Documents[i].Content))
	}
	s.Source.ContentHash = CatalogueHash(s.Documents)
	s.Verification.SourceContentHash = s.Source.ContentHash
}

func TestGenericContractUsesSourceIdentitiesAndStandardExecution(t *testing.T) {
	s := genericConsentFixture(t)
	events := decodeEvents(t, testMessages(t, s))
	withoutAttachments := []map[string]any{}
	for _, e := range events {
		if e["attachment"] != nil {
			continue
		}
		if p, ok := e["pickle"].(map[string]any); ok {
			// Cucumber inherits the feature ID tag as well as the scenario ID.
			p["tags"] = append(p["tags"].([]any), map[string]any{"name": "@id:" + s.Scenarios[0].FeatureID})
		}
		withoutAttachments = append(withoutAttachments, e)
	}
	raw := encodeEvents(t, withoutAttachments)
	s.Verification.MessagesHash = Hash(raw)
	wire, _ := json.Marshal(s)
	decoded, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err = decoded.Eligible(); err != nil {
		t.Fatal(err)
	}
	entries, err := decoded.Entries()
	if err != nil || len(entries) != 3 {
		t.Fatal(entries, err)
	}
	if err = ValidateMessages(raw, decoded); err != nil {
		t.Fatal(err)
	}
	// Attachments belong to the producer; arbitrary media/body never claims proof.
	withoutAttachments = append(withoutAttachments, map[string]any{"attachment": map[string]any{"mediaType": "application/vnd.vozokany.fixture+json", "body": "application-owned", "testCaseStartedId": "started"}})
	if err = ValidateMessages(encodeEvents(t, withoutAttachments), decoded); err != nil {
		t.Fatal(err)
	}
}

func TestV2EnvelopePreservesLegacyBusinessIdentity(t *testing.T) {
	s := mainFixture(t)
	before, err := s.Entries()
	if err != nil {
		t.Fatal(err)
	}
	s.Format = "gherkin-specification"
	s.SchemaVersion = 2
	s.Verification.SchemaVersion = 2
	data, _ := json.Marshal(s)
	s, err = Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.Entries()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("contract upgrade changed business identities", err)
	}
}

func TestConceptsAreOptionalAndIdentitiesCannotCollide(t *testing.T) {
	s := genericConsentFixture(t)
	s.Documents = s.Documents[1:]
	s.Documents[0].Content = strings.ReplaceAll(s.Documents[0].Content, " @concept:concept.consent", "")
	s.Documents[0].ConceptIDs = []string{}
	rehash(&s)
	if _, err := s.Entries(); err != nil {
		t.Fatal(err)
	}
	s.Documents[0].Content = strings.ReplaceAll(s.Documents[0].Content, s.Scenarios[0].ID, s.Documents[0].ID)
	s.Scenarios[0].ID = s.Documents[0].ID
	s.Verification.Cases[0].ScenarioID = s.Documents[0].ID
	rehash(&s)
	if _, err := s.Entries(); err == nil {
		t.Fatal("accepted an ID shared by feature and scenario")
	}
}

func TestGenericPollerConfigurationAndSourceBinding(t *testing.T) {
	f := newPollFixture(t)
	f.poller.ArtifactPrefix = "" // generic default
	f.poller.Workflow = ".github/workflows/acceptance.yaml"
	f.workflowPath = f.poller.Workflow
	f.run.Path = f.poller.Workflow
	f.snapshot = genericConsentFixture(t)
	f.snapshot.Publication.Workflow = f.poller.Workflow
	f.archive = archiveFor(t, &f.snapshot, testMessages(t, f.snapshot), nil)
	f.artifact.Digest = Hash(f.archive)
	f.artifact.Size = int64(len(f.archive))
	f.artifact.Name = "gherkin-specs-v2-" + f.run.HeadSHA + "-10-1"
	interceptPollResponses(f, func(r *http.Request, _ int) *http.Response {
		switch r.URL.Path {
		case repositoryAPI + "/actions/workflows/acceptance.yaml":
			return pollJSONResponse(t, map[string]any{"id": f.workflowID, "path": f.workflowPath})
		case repositoryAPI + "/actions/workflows/acceptance.yaml/runs":
			return pollJSONResponse(t, map[string]any{"workflow_runs": []githubRun{f.run}})
		}
		return nil
	})
	if receipt, err := f.poller.Poll(context.Background()); err != nil || receipt.State != "imported" {
		t.Fatal(receipt, err)
	}
	for _, change := range []func(*Snapshot){
		func(s *Snapshot) { s.Source.Repository = "https://github.com/matk0/not-the-configured-repository" },
		func(s *Snapshot) { s.Publication.Workflow = Workflow },
	} {
		f.current.RunID = ""
		f.snapshot.Source.Repository = Repository
		f.snapshot.Publication.Workflow = f.poller.Workflow
		change(&f.snapshot)
		f.archive = archiveFor(t, &f.snapshot, testMessages(t, f.snapshot), nil)
		f.artifact.Digest = Hash(f.archive)
		f.artifact.Size = int64(len(f.archive))
		if _, err := f.poller.Poll(context.Background()); err == nil {
			t.Fatal("accepted evidence for a different authority")
		}
	}
}

func TestSourceAndWorkerConfigurationRejectURLInjection(t *testing.T) {
	for _, repository := range []string{"https://github.com/matk0/vozokany?x=1", "https://github.com/matk0/vozokany/../elsewhere", "https://github.com@elsewhere.test/matk0/vozokany", "http://github.com/matk0/vozokany", "https://github.com/matk0/vozokany#ref"} {
		if ValidRepository(repository) {
			t.Fatal("accepted non-canonical repository", repository)
		}
	}
	values := map[string]string{"VIKI_GITHUB_READ_TOKEN": "read-only", "VIKI_SNAPSHOT_IMPORT_TOKEN": strings.Repeat("s", 32)}
	env := func(k string) string { return values[k] }
	p, err := WorkerFromEnvironment(env)
	if err != nil || p.Workflow != Workflow || p.ArtifactPrefix != "gherkin-specs-v2" {
		t.Fatal(p, err)
	}
	values["VIKI_GIT_WORKFLOW"] = ".github/workflows/acceptance.yaml"
	values["VIKI_GIT_ARTIFACT_PREFIX"] = "vozokany-business-specs-v1"
	p, err = WorkerFromEnvironment(env)
	if err != nil || p.Workflow != values["VIKI_GIT_WORKFLOW"] || p.ArtifactPrefix != values["VIKI_GIT_ARTIFACT_PREFIX"] {
		t.Fatal(p, err)
	}
	values["VIKI_GIT_ARTIFACT_PREFIX"] = "../artifact"
	if _, err = WorkerFromEnvironment(env); err == nil {
		t.Fatal("accepted invalid artifact prefix")
	}
	f := newPollFixture(t)
	f.poller.Workflow = "../workflow"
	if _, err = f.poller.Poll(context.Background()); err == nil {
		t.Fatal("accepted invalid direct worker configuration")
	}
}

func TestArchivedVozokanyArtifactCompatibility(t *testing.T) {
	archivePath := os.Getenv("VIKI_TEST_COMPATIBILITY_ARCHIVE")
	if archivePath == "" {
		t.Skip("set VIKI_TEST_COMPATIBILITY_ARCHIVE to a retained Vozokany CI archive")
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	files, err := ReadArtifact(archive)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Decode(files["snapshot.json"])
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Eligible(); err != nil {
		t.Fatal(err)
	}
	entries, err := s.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 123 || len(s.Scenarios) != 103 {
		t.Fatal("unexpected compatibility catalogue", len(entries), len(s.Scenarios))
	}
	if Hash(files["cucumber-messages.ndjson"]) != s.Verification.MessagesHash {
		t.Fatal("message digest mismatch")
	}
	if err = ValidateMessages(files["cucumber-messages.ndjson"], s); err != nil {
		t.Fatal(err)
	}
	t.Logf("retained CI artifact: %d pages, %d scenarios, source %s", len(entries), len(s.Scenarios), s.Source.Commit)
}
