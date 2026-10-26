package specsync

import (
	"bytes"
	"encoding/json"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T) Snapshot {
	t.Helper()
	b, e := os.ReadFile("testdata/snapshot-v1.valid.json")
	if e != nil {
		t.Fatal(e)
	}
	s, e := Decode(b)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestSharedContractSourceIsAuthoritative(t *testing.T) {
	s := fixture(t)
	entries, e := s.Entries()
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) != 3 {
		t.Fatal(entries)
	}
	if s.Eligible() == nil {
		t.Fatal("synthetic local fixture must never be live eligible")
	}
}
func TestRejectsMutatedParsedCopiesAndIncompleteResults(t *testing.T) {
	cases := map[string]func(*Snapshot){
		"title": func(s *Snapshot) { s.Documents[1].Title = "fabricated" }, "description": func(s *Snapshot) { s.Documents[1].Description = "fabricated" }, "reference": func(s *Snapshot) { s.Documents[1].ConceptIDs = []string{} }, "step": func(s *Snapshot) { s.Scenarios[0].Steps[0].Text = "fabricated" }, "name": func(s *Snapshot) { s.Scenarios[0].Name = "fabricated" }, "location": func(s *Snapshot) { s.Scenarios[0].Line++ }, "parent": func(s *Snapshot) { s.Scenarios[0].FeatureID = "VOZ-FEAT-FAKE" }, "scenario missing": func(s *Snapshot) { s.Scenarios = nil }, "result missing": func(s *Snapshot) { s.Verification.Cases = nil }, "result duplicate": func(s *Snapshot) { s.Verification.Cases = append(s.Verification.Cases, s.Verification.Cases[0]) }, "step mismatch": func(s *Snapshot) { s.Verification.Cases[0].Steps[0].Text = "fabricated" }, "hash": func(s *Snapshot) { s.Documents[0].Content += "changed" }, "source mismatch": func(s *Snapshot) { s.Verification.SourceCommit = "changed" }, "duplicate document": func(s *Snapshot) {
			s.Documents = append(s.Documents, s.Documents[0])
			s.Source.ContentHash = CatalogueHash(s.Documents)
			s.Verification.SourceContentHash = s.Source.ContentHash
		},
	}
	for _, status := range []string{"failed", "undefined", "ambiguous", "pending", "skipped", "unknown"} {
		cases["case "+status] = func(s *Snapshot) { s.Verification.Cases[0].Status = status }
		cases["step "+status] = func(s *Snapshot) { s.Verification.Cases[0].Steps[0].Status = status }
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := fixture(t)
			mutate(&s)
			if _, e := s.Entries(); e == nil {
				t.Fatal("accepted mutated source/result")
			}
		})
	}
}
func TestSchemaRejectsMissingUnknownAndUnsupportedVersion(t *testing.T) {
	s := fixture(t)
	b, _ := json.Marshal(s)
	for _, data := range []string{"{", strings.Replace(string(b), `"schemaVersion":1`, `"schemaVersion":2`, 1), strings.Replace(string(b), `"format":`, `"unknown":true,"format":`, 1), strings.Replace(string(b), `"dirty":true,`, "", 1)} {
		if _, e := Decode([]byte(data)); e == nil {
			t.Fatal("accepted invalid schema")
		}
	}
}

func TestSnapshotUnsupportedSyntaxAndReferencesFailClosed(t *testing.T) {
	mutations := map[string]func(*Snapshot){
		"empty": func(s *Snapshot) { s.Documents = nil }, "time": func(s *Snapshot) { s.Verification.FinishedAt = "2000-01-01T00:00:00Z" }, "path": func(s *Snapshot) { s.Documents[0].Path = "specs/business/../consent.md" }, "concept source": func(s *Snapshot) {
			s.Documents[0].Content = strings.Replace(s.Documents[0].Content, "<!-- business-id:", "<!-- wrong-id:", 1)
		}, "duplicate concept metadata": func(s *Snapshot) { s.Documents[0].Content += "\n<!-- business-id: VOZ-CON-OTHER -->\n" }, "duplicate refs": func(s *Snapshot) {
			s.Documents[0].Content += "<!-- relates-to: VOZ-CON-X -->\n<!-- relates-to: VOZ-CON-Y -->\n"
		}, "dangling ref": func(s *Snapshot) {
			s.Documents[0].Content += "<!-- relates-to: VOZ-CON-X -->\n"
			s.Documents[0].ConceptIDs = []string{"VOZ-CON-X"}
		}, "self ref": func(s *Snapshot) {
			s.Documents[0].Content += "<!-- relates-to: VOZ-CON-CONSENT -->\n"
			s.Documents[0].ConceptIDs = []string{"VOZ-CON-CONSENT"}
		}, "unknown kind": func(s *Snapshot) { s.Documents[0].Kind = "unknown" }, "duplicate scenario": func(s *Snapshot) {
			s.Documents[1].Content += "\n  @id:VOZ-SC-CONSENT-001\n  Scenario: Duplicate\n    Given first\n    When next\n    Then last\n"
		}, "result unknown": func(s *Snapshot) { s.Verification.Cases[0].ScenarioID = "VOZ-SC-UNKNOWN" }, "duplicate scenario metadata": func(s *Snapshot) {
			s.Scenarios = append(s.Scenarios, s.Scenarios[0])
			s.Scenarios = s.Scenarios[:1]
			s.Scenarios[0].ID = "VOZ-SC-MISSING"
		},
	}
	for name, content := range map[string]string{
		"parse": "not valid Gherkin", "empty feature": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n", "French": "# language: fr\nFonctionnalité: exemple\n", "background": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n  Background: State\n    Given state\n", "outline": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n  @id:VOZ-SC-CONSENT-001\n  Scenario Outline: Example\n    Given <value>\n    When next\n    Then last\n    Examples:\n      | value |\n      | one |\n", "rule": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n Rule: Example\n  Scenario: Example\n   Given state\n", "wildcard": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n  @id:VOZ-SC-CONSENT-001\n  Scenario: Example\n    * wildcard\n", "table": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n  @id:VOZ-SC-CONSENT-001\n  Scenario: Example\n    Given state\n      | x |\n", "missing Given": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n  @id:VOZ-SC-CONSENT-001\n  Scenario: Example\n    When action\n    Then result\n", "missing Then": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n  @id:VOZ-SC-CONSENT-001\n  Scenario: Example\n    Given state\n    When action\n", "unknown feature tag": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT @extra\nFeature: Customer consent\n", "duplicate feature id": "@id:VOZ-FEAT-CONSENT @id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n", "no scenario id": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n Scenario: Example\n  Given state\n  When action\n  Then result\n", "no feature id": "@concept:VOZ-CON-CONSENT\nFeature: Customer consent\n Scenario: Example\n  Given state\n", "invalid scenario id": "@id:VOZ-FEAT-CONSENT @concept:VOZ-CON-CONSENT\nFeature: Customer consent\n @id:VOZ-CON-INVALID\n Scenario: Example\n  Given state\n  When action\n  Then result\n",
	} {
		mutations[name] = func(s *Snapshot) { s.Documents[1].Content = content }
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s := fixture(t)
			mutate(&s)
			for i := range s.Documents {
				s.Documents[i].ContentHash = Hash([]byte(s.Documents[i].Content))
			}
			s.Source.ContentHash = CatalogueHash(s.Documents)
			s.Verification.SourceContentHash = s.Source.ContentHash
			if _, err := s.Entries(); err == nil {
				t.Fatal("accepted invalid source")
			}
		})
	}
}
func TestSchemaLoadAndNumericEncodingFailures(t *testing.T) {
	s := fixture(t)
	raw, _ := json.Marshal(s)
	if _, err := Decode([]byte{0xff}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	if _, err := Decode(bytes.Replace(raw, []byte(`"runAttempt":1`), []byte(`"runAttempt":100000000000000000000000000000`), 1)); err == nil {
		t.Fatal("overflow accepted")
	}
	old := schemaBytes
	defer func() { schemaBytes = old }()
	for _, schema := range []string{"{", `{"type":"invalid"}`} {
		schemaBytes = []byte(schema)
		if _, err := Decode(raw); err == nil {
			t.Fatal("invalid contract accepted")
		}
	}
	schemaBytes = old
	compiler := jsonschema.NewCompiler()
	doc, _ := jsonschema.UnmarshalJSON(bytes.NewReader(old))
	if err := compiler.AddResource("snapshot.json", doc); err != nil {
		t.Fatal(err)
	}
	factory := newSchemaCompiler
	defer func() { newSchemaCompiler = factory }()
	newSchemaCompiler = func() *jsonschema.Compiler { return compiler }
	if _, err := Decode(raw); err == nil {
		t.Fatal("duplicate schema registration ignored")
	}
}
