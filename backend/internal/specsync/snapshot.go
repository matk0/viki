// Package specsync validates complete repository snapshots without executing their contents.
package specsync

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	gherkin "github.com/cucumber/gherkin/go/v36"
	messages "github.com/cucumber/messages/go/v30"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const Repository = "https://github.com/matk0/vozokany"
const Workflow = ".github/workflows/verify.yml"

//go:embed contract/snapshot-v1.schema.json
var schemaBytes []byte
var newSchemaCompiler = jsonschema.NewCompiler

type Source struct {
	Repository  string `json:"repository"`
	Commit      string `json:"commit"`
	Dirty       bool   `json:"dirty"`
	ContentHash string `json:"contentHash"`
}
type Publication struct {
	Kind       string `json:"kind"`
	Event      string `json:"event"`
	Ref        string `json:"ref"`
	Workflow   string `json:"workflow"`
	RunID      string `json:"runId"`
	RunAttempt int    `json:"runAttempt"`
	Eligible   bool   `json:"eligibleForImport"`
}
type Document struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Path        string   `json:"path"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	ContentHash string   `json:"contentHash"`
	ConceptIDs  []string `json:"conceptIds"`
}
type Step struct {
	Keyword string `json:"keyword"`
	Text    string `json:"text"`
	Line    int    `json:"line"`
}
type Scenario struct {
	ID          string `json:"id"`
	FeatureID   string `json:"featureId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Line        int    `json:"line"`
	Steps       []Step `json:"steps"`
}
type ResultStep struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}
type Result struct {
	ScenarioID string       `json:"scenarioId"`
	Status     string       `json:"status"`
	Duration   string       `json:"durationNanoseconds"`
	Steps      []ResultStep `json:"steps"`
}
type Verification struct {
	SchemaVersion     int    `json:"schemaVersion"`
	SourceCommit      string `json:"sourceCommit"`
	SourceContentHash string `json:"sourceContentHash"`
	Backend           string `json:"backend"`
	Runner            struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"runner"`
	Command      string   `json:"command"`
	StartedAt    string   `json:"startedAt"`
	FinishedAt   string   `json:"finishedAt"`
	MessagesHash string   `json:"messagesHash"`
	Cases        []Result `json:"cases"`
}
type Snapshot struct {
	Format        string       `json:"format"`
	SchemaVersion int          `json:"schemaVersion"`
	Source        Source       `json:"source"`
	Publication   Publication  `json:"publication"`
	Documents     []Document   `json:"documents"`
	Scenarios     []Scenario   `json:"scenarios"`
	Verification  Verification `json:"verification"`
}

// Entry is derived from validated authoritative sources; wire fields never bypass the parser.
type Entry struct {
	ExternalID    string   `json:"externalId"`
	Kind          string   `json:"kind"`
	ParentID      string   `json:"parentId,omitempty"`
	Title         string   `json:"title"`
	Body          string   `json:"body"`
	Steps         []Step   `json:"steps"`
	ConceptIDs    []string `json:"conceptIds"`
	Path          string   `json:"path"`
	SourceContent string   `json:"sourceContent"`
	DocumentHash  string   `json:"documentHash"`
	BusinessHash  string   `json:"businessHash"`
}

func Hash(data []byte) string { h := sha256.Sum256(data); return "sha256:" + hex.EncodeToString(h[:]) }
func CatalogueHash(documents []Document) string {
	sorted := append([]Document{}, documents...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	var b strings.Builder
	for _, d := range sorted {
		b.WriteString(d.Path + "\x00" + d.ContentHash + "\n")
	}
	return Hash([]byte(b.String()))
}
func Decode(data []byte) (Snapshot, error) {
	var s Snapshot
	if !utf8.Valid(data) {
		return s, fmt.Errorf("invalid UTF-8 JSON")
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return s, fmt.Errorf("invalid snapshot JSON: %w", err)
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaBytes))
	if err != nil {
		return s, err
	}
	compiler := newSchemaCompiler()
	compiler.AssertFormat()
	if err = compiler.AddResource("snapshot.json", schemaDoc); err != nil {
		return s, err
	}
	schema, err := compiler.Compile("snapshot.json")
	if err != nil {
		return s, err
	}
	if err = schema.Validate(value); err != nil {
		return s, fmt.Errorf("snapshot schema: %w", err)
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	return s, nil
}

var conceptID = regexp.MustCompile(`^VOZ-CON-[A-Z0-9-]+$`)
var featureID = regexp.MustCompile(`^VOZ-FEAT-[A-Z0-9-]+$`)
var scenarioID = regexp.MustCompile(`^VOZ-SC-[A-Z0-9-]+$`)
var conceptHeader = regexp.MustCompile(`^<!-- business-id: (VOZ-CON-[A-Z0-9-]+) -->$`)
var conceptRefs = regexp.MustCompile(`(?m)^<!-- relates-to: ([^\n]+) -->$`)
var heading = regexp.MustCompile(`(?m)^# (.+)$`)

func (s Snapshot) Entries() ([]Entry, error) {
	if len(s.Documents) == 0 || len(s.Scenarios) == 0 {
		return nil, fmt.Errorf("empty catalogue")
	}
	if s.Source.Commit != s.Verification.SourceCommit || s.Source.ContentHash != s.Verification.SourceContentHash || CatalogueHash(s.Documents) != s.Source.ContentHash {
		return nil, fmt.Errorf("source/results content or commit mismatch")
	}
	start, e1 := time.Parse(time.RFC3339Nano, s.Verification.StartedAt)
	end, e2 := time.Parse(time.RFC3339Nano, s.Verification.FinishedAt)
	if e1 != nil || e2 != nil || end.Before(start) {
		return nil, fmt.Errorf("invalid execution interval")
	}
	docs := map[string]Document{}
	paths := map[string]bool{}
	parsed := map[string]Scenario{}
	entries := []Entry{}
	for _, d := range s.Documents {
		if _, ok := docs[d.ID]; ok || paths[d.Path] {
			return nil, fmt.Errorf("duplicate document identity or path")
		}
		docs[d.ID] = d
		paths[d.Path] = true
		if !utf8.ValidString(d.Content) || (strings.ContainsAny(d.Content, "\x00\r") || strings.HasPrefix(d.Content, "\ufeff")) || Hash([]byte(d.Content)) != d.ContentHash {
			return nil, fmt.Errorf("invalid source encoding or document hash")
		}
		for _, segment := range strings.Split(d.Path, "/") {
			if segment == ".." || segment == "." || segment == "" {
				return nil, fmt.Errorf("invalid source path")
			}
		}
		expected := d
		entry := Entry{ExternalID: d.ID, Kind: d.Kind, Title: d.Title, Body: d.Content, Steps: []Step{}, ConceptIDs: d.ConceptIDs, Path: d.Path, SourceContent: d.Content, DocumentHash: d.ContentHash, BusinessHash: d.ContentHash}
		switch d.Kind {
		case "concept":
			id := conceptHeader.FindStringSubmatch(strings.Split(d.Content, "\n")[0])
			titles := heading.FindStringSubmatch(d.Content)
			if len(regexp.MustCompile(`(?m)^<!-- business-id: VOZ-CON-[A-Z0-9-]+ -->$`).FindAllString(d.Content, -1)) != 1 || len(id) != 2 || !conceptID.MatchString(d.ID) || id[1] != d.ID || len(titles) != 2 || !strings.HasSuffix(d.Path, ".md") {
				return nil, fmt.Errorf("invalid concept source")
			}
			expected.Title = titles[1]
			expected.Description = ""
			expected.ConceptIDs = []string{}
			refs := conceptRefs.FindAllStringSubmatch(d.Content, -1)
			if len(refs) > 1 {
				return nil, fmt.Errorf("duplicate concept reference annotation")
			}
			if len(refs) == 1 {
				for _, id := range strings.Split(refs[0][1], ", ") {
					expected.ConceptIDs = append(expected.ConceptIDs, id)
				}
			}
		case "feature":
			document, err := gherkin.ParseGherkinDocument(strings.NewReader(d.Content), uuid.NewString)
			if err != nil {
				return nil, fmt.Errorf("Gherkin parse: %w", err)
			}
			f := document.Feature
			if f == nil || f.Language != "en" || f.Keyword != "Feature" || len(f.Children) == 0 || !strings.HasSuffix(d.Path, ".feature") {
				return nil, fmt.Errorf("unsupported feature syntax")
			}
			id, refs, err := tags(f.Tags, true)
			if err != nil || id != d.ID || !featureID.MatchString(id) {
				return nil, fmt.Errorf("invalid feature tags")
			}
			expected.Title = f.Name
			expected.Description = f.Description
			expected.ConceptIDs = refs
			entry.Body = f.Description
			for _, child := range f.Children {
				sc := child.Scenario
				if sc == nil || child.Rule != nil || child.Background != nil || sc.Keyword != "Scenario" || len(sc.Examples) > 0 || len(sc.Steps) == 0 {
					return nil, fmt.Errorf("unsupported scenario syntax")
				}
				id, _, err := tags(sc.Tags, false)
				if err != nil || !scenarioID.MatchString(id) {
					return nil, fmt.Errorf("invalid scenario tags")
				}
				if _, ok := parsed[id]; ok {
					return nil, fmt.Errorf("duplicate source scenario")
				}
				scenario := Scenario{ID: id, FeatureID: d.ID, Name: sc.Name, Description: sc.Description, Line: int(sc.Location.Line), Steps: []Step{}}
				for _, st := range sc.Steps {
					keyword := strings.TrimSpace(st.Keyword)
					if st.DataTable != nil || st.DocString != nil || !validKeyword(keyword) {
						return nil, fmt.Errorf("unsupported step syntax")
					}
					scenario.Steps = append(scenario.Steps, Step{keyword, st.Text, int(st.Location.Line)})
				}
				if scenario.Steps[0].Keyword != "Given" {
					return nil, fmt.Errorf("Given, When and Then required")
				}
				hasWhen, hasThen := false, false
				for _, step := range scenario.Steps {
					hasWhen = hasWhen || step.Keyword == "When"
					hasThen = hasThen || step.Keyword == "Then"
				}
				if !hasWhen || !hasThen {
					return nil, fmt.Errorf("Given, When and Then required")
				}
				parsed[id] = scenario
				se := Entry{ExternalID: id, Kind: "scenario", ParentID: d.ID, Title: sc.Name, Body: sc.Description, Steps: scenario.Steps, ConceptIDs: refs, Path: d.Path, SourceContent: d.Content, DocumentHash: d.ContentHash}
				// Location/path/run metadata cannot create another business revision.
				fields := []string{id, d.ID, sc.Name, sc.Description}
				fields = append(fields, refs...)
				for _, st := range se.Steps {
					fields = append(fields, st.Keyword, st.Text)
				}
				se.BusinessHash = Hash([]byte(strings.Join(fields, "\x00")))
				entries = append(entries, se)
			}
		default:
			return nil, fmt.Errorf("unsupported document kind")
		}
		if d.Title != expected.Title || d.Description != expected.Description || !reflect.DeepEqual(d.ConceptIDs, expected.ConceptIDs) {
			return nil, fmt.Errorf("parsed document differs from hashed source")
		}
		entries = append(entries, entry)
	}
	for _, d := range s.Documents {
		seen := map[string]bool{}
		for _, id := range d.ConceptIDs {
			ref, ok := docs[id]
			if !ok || ref.Kind != "concept" || seen[id] || id == d.ID {
				return nil, fmt.Errorf("unresolved or duplicate concept reference")
			}
			seen[id] = true
		}
	}
	if len(s.Scenarios) != len(parsed) || len(s.Verification.Cases) != len(parsed) {
		return nil, fmt.Errorf("source/executed scenario sets differ")
	}
	seen := map[string]bool{}
	for _, sc := range s.Scenarios {
		actual, ok := parsed[sc.ID]
		if !ok || seen[sc.ID] || !reflect.DeepEqual(sc, actual) {
			return nil, fmt.Errorf("parsed scenario differs from hashed source")
		}
		seen[sc.ID] = true
	}
	seen = map[string]bool{}
	for _, result := range s.Verification.Cases {
		sc, ok := parsed[result.ScenarioID]
		if !ok || seen[result.ScenarioID] || result.Status != "passed" || len(result.Steps) != len(sc.Steps) {
			return nil, fmt.Errorf("missing, duplicate or unsuccessful execution")
		}
		seen[result.ScenarioID] = true
		for i, st := range result.Steps {
			if st.Status != "passed" || st.Text != sc.Steps[i].Text {
				return nil, fmt.Errorf("step result differs or did not pass")
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ExternalID < entries[j].ExternalID })
	return entries, nil
}
func validKeyword(s string) bool {
	return s == "Given" || s == "When" || s == "Then" || s == "And" || s == "But"
}
func tags(list []*messages.Tag, feature bool) (string, []string, error) {
	id := ""
	refs := []string{}
	for _, t := range list {
		switch {
		case strings.HasPrefix(t.Name, "@id:"):
			if id != "" {
				return "", nil, fmt.Errorf("duplicate id tag")
			}
			id = strings.TrimPrefix(t.Name, "@id:")
		case feature && strings.HasPrefix(t.Name, "@concept:"):
			refs = append(refs, strings.TrimPrefix(t.Name, "@concept:"))
		default:
			return "", nil, fmt.Errorf("unsupported tag")
		}
	}
	if id == "" || (feature && len(refs) == 0) {
		return "", nil, fmt.Errorf("missing metadata tags")
	}
	return id, refs, nil
}
func (s Snapshot) Eligible() error {
	id, err := strconv.ParseInt(s.Publication.RunID, 10, 64)
	if err != nil || id <= 0 || s.Source.Repository != Repository || s.Source.Dirty || s.Source.Commit == strings.Repeat("0", 40) || s.Publication.Kind != "main" || s.Publication.Event != "push" || s.Publication.Ref != "refs/heads/main" || s.Publication.Workflow != Workflow || s.Publication.RunAttempt < 1 || !s.Publication.Eligible || s.Verification.MessagesHash == "sha256:"+strings.Repeat("0", 64) {
		return fmt.Errorf("snapshot is not eligible successful-main evidence")
	}
	return nil
}
