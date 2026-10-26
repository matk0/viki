package specsync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// These fixtures only exercise isolated HTTP and parser boundaries. They are
// synthetic evidence and must never be used by a live repository-sync worker.
func interceptPollResponses(f *pollFixture, intercept func(*http.Request, int) *http.Response) {
	base := f.poller.Client.Transport
	counts := map[string]int{}
	f.poller.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		counts[r.URL.Path]++
		if response := intercept(r, counts[r.URL.Path]); response != nil {
			return response, nil
		}
		return base.RoundTrip(r)
	})
}

func pollJSONResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}
}

func TestPollRejectsMalformedMetadataAndChangedAttempts(t *testing.T) {
	for _, target := range []string{
		repositoryAPI,
		repositoryAPI + "/branches/main",
		repositoryAPI + "/actions/workflows/verify.yml",
		repositoryAPI + "/actions/workflows/verify.yml/runs",
		repositoryAPI + "/actions/runs/10/attempts/1",
		repositoryAPI + "/actions/runs/10/artifacts",
	} {
		t.Run("malformed "+target, func(t *testing.T) {
			f := newPollFixture(t)
			interceptPollResponses(f, func(r *http.Request, _ int) *http.Response {
				if r.URL.Path == target {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{")), Header: http.Header{}}
				}
				return nil
			})
			if _, err := f.poller.Poll(context.Background()); ErrorCode(err) != "candidate_rejected" || f.imports != 0 {
				t.Fatalf("malformed metadata changed baseline: err=%v imports=%d", err, f.imports)
			}
		})
	}
	for _, stage := range []string{"initial attempt", "branch recheck", "attempt recheck", "changed final attempt"} {
		t.Run(stage, func(t *testing.T) {
			f := newPollFixture(t)
			interceptPollResponses(f, func(r *http.Request, count int) *http.Response {
				isAttempt := r.URL.Path == repositoryAPI+"/actions/runs/10/attempts/1"
				isBranch := r.URL.Path == repositoryAPI+"/branches/main"
				if stage == "initial attempt" && isAttempt && count == 1 || stage == "changed final attempt" && isAttempt && count == 2 {
					changed := f.run
					changed.Attempt++
					return pollJSONResponse(t, changed)
				}
				if stage == "branch recheck" && isBranch && count == 2 || stage == "attempt recheck" && isAttempt && count == 2 {
					return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}
				}
				return nil
			})
			if _, err := f.poller.Poll(context.Background()); err == nil || f.imports != 0 {
				t.Fatalf("changed/missing evidence changed baseline: err=%v imports=%d", err, f.imports)
			}
		})
	}
}

func TestPollRequiresForwardAncestryAndChoosesNewestRun(t *testing.T) {
	for _, mutation := range []string{"behind", "wrong base", "different merge base"} {
		t.Run(mutation, func(t *testing.T) {
			f := newPollFixture(t)
			f.current.Commit = strings.Repeat("2", 40)
			interceptPollResponses(f, func(r *http.Request, _ int) *http.Response {
				if !strings.HasPrefix(r.URL.Path, repositoryAPI+"/compare/") {
					return nil
				}
				status, base, mergeBase := "ahead", f.current.Commit, f.current.Commit
				switch mutation {
				case "behind":
					status = "behind"
				case "wrong base":
					base = strings.Repeat("3", 40)
				case "different merge base":
					mergeBase = strings.Repeat("3", 40)
				}
				return pollJSONResponse(t, map[string]any{"status": status, "base_commit": map[string]string{"sha": base}, "merge_base_commit": map[string]string{"sha": mergeBase}})
			})
			if _, err := f.poller.Poll(context.Background()); ErrorCode(err) != "candidate_rejected" || f.imports != 0 {
				t.Fatalf("non-forward ancestry changed baseline: err=%v imports=%d", err, f.imports)
			}
		})
	}
	f := newPollFixture(t)
	interceptPollResponses(f, func(r *http.Request, _ int) *http.Response {
		if r.URL.Path == repositoryAPI+"/actions/workflows/verify.yml/runs" {
			older := f.run
			older.ID--
			older.Conclusion = "failure"
			return pollJSONResponse(t, map[string]any{"workflow_runs": []githubRun{older, f.run}})
		}
		return nil
	})
	receipt, err := f.poller.Poll(context.Background())
	if err != nil || receipt.State != "imported" || receipt.RunID != "10" || f.imports != 1 {
		t.Fatalf("newest eligible run was not selected: receipt=%+v err=%v imports=%d", receipt, err, f.imports)
	}
}

func TestHTTPDefaultBackoffStillHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := 0
	p := &Poller{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		cancel()
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	})}}
	if _, err := p.request(ctx, http.MethodGet, "https://local-fixture.invalid", "synthetic", nil, 100); err != context.Canceled || requests != 1 {
		t.Fatalf("default retry ignored cancellation: err=%v requests=%d", err, requests)
	}
}

func TestArtifactRejectsDamagedAndNonRegularZIPEntries(t *testing.T) {
	for _, mutation := range []string{"missing file", "symlink", "unknown compression", "expanded size", "CRC mismatch"} {
		t.Run(mutation, func(t *testing.T) {
			var out bytes.Buffer
			writer := zip.NewWriter(&out)
			for i, name := range []string{"snapshot.json", "execution.json", "cucumber-messages.ndjson"} {
				if mutation == "missing file" && i == 2 {
					continue
				}
				contents := []byte("synthetic artifact")
				header := &zip.FileHeader{Name: name, Method: zip.Store, CRC32: crc32.ChecksumIEEE(contents), CompressedSize64: uint64(len(contents)), UncompressedSize64: uint64(len(contents))}
				if i == 0 {
					switch mutation {
					case "symlink":
						header.SetMode(os.ModeSymlink | 0600)
					case "unknown compression":
						header.Method = 99
					case "expanded size":
						header.UncompressedSize64 = maxExpandedBytes + 1
					case "CRC mismatch":
						header.CRC32++
					}
				}
				entry, err := writer.CreateRaw(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := entry.Write(contents); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadArtifact(out.Bytes()); err == nil {
				t.Fatal("accepted damaged/non-regular artifact", mutation)
			}
		})
	}
}

func TestMessagesRequireExactSourceSetAndExecutableStepOrder(t *testing.T) {
	s := fixture(t)
	raw := testMessages(t, s)
	mutations := map[string]func([]map[string]any) []map[string]any{
		"unexpected executed source": func(events []map[string]any) []map[string]any {
			return append(events, map[string]any{"source": map[string]any{"uri": "specs/business/extra.feature", "data": "Feature: Unreviewed"}})
		},
		"source step replaced by hook": func(events []map[string]any) []map[string]any {
			for _, event := range events {
				if tc, ok := event["testCase"].(map[string]any); ok {
					step := tc["testSteps"].([]any)[1].(map[string]any)
					delete(step, "pickleStepId")
					step["hookId"] = "replacement-hook"
				}
			}
			return events
		},
		"reordered source steps": func(events []map[string]any) []map[string]any {
			for _, event := range events {
				if tc, ok := event["testCase"].(map[string]any); ok {
					steps := tc["testSteps"].([]any)
					steps[1], steps[2] = steps[2], steps[1]
				}
			}
			return events
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMessages(encodeEvents(t, mutate(decodeEvents(t, raw))), s); err == nil {
				t.Fatal("accepted incomplete or reordered execution proof")
			}
		})
	}
	if err := ValidateMessages(append([]byte("\n \t\n"), raw...), s); err != nil {
		t.Fatal("blank NDJSON lines changed valid execution", err)
	}
	if s.Eligible() == nil {
		t.Fatal("local parser fixture became live eligible")
	}
}

func TestSourceParserRejectsTagsEvenWithOtherwiseValidScenarios(t *testing.T) {
	for _, mutation := range []string{"duplicate feature ID", "duplicate scenario ID", "unknown feature tag", "unknown scenario tag"} {
		t.Run(mutation, func(t *testing.T) {
			s := fixture(t)
			document := &s.Documents[1]
			switch mutation {
			case "duplicate feature ID":
				document.Content = strings.Replace(document.Content, "@id:VOZ-FEAT-CONSENT", "@id:VOZ-FEAT-CONSENT @id:VOZ-FEAT-OTHER", 1)
			case "duplicate scenario ID":
				document.Content = strings.Replace(document.Content, "@id:VOZ-SC-CONSENT-001", "@id:VOZ-SC-CONSENT-001 @id:VOZ-SC-OTHER", 1)
			case "unknown feature tag":
				document.Content = strings.Replace(document.Content, "@id:VOZ-FEAT-CONSENT", "@id:VOZ-FEAT-CONSENT @extra", 1)
			case "unknown scenario tag":
				document.Content = strings.Replace(document.Content, "@id:VOZ-SC-CONSENT-001", "@id:VOZ-SC-CONSENT-001 @concept:VOZ-CON-CONSENT", 1)
			}
			document.ContentHash = Hash([]byte(document.Content))
			s.Source.ContentHash = CatalogueHash(s.Documents)
			s.Verification.SourceContentHash = s.Source.ContentHash
			if _, err := s.Entries(); err == nil {
				t.Fatal("accepted unsupported source metadata")
			}
		})
	}
}

func TestMessagesCompareCompleteScenarioSetsIndependentOfResultOrder(t *testing.T) {
	s := fixture(t)
	first := s.Scenarios[0]
	second := first
	second.ID = "VOZ-SC-CONSENT-002"
	second.Name = "Record consent for another customer"
	second.Steps = append([]Step{}, first.Steps...)
	content := s.Documents[1].Content
	block := content[strings.Index(content, "  @id:VOZ-SC-"):]
	block = strings.ReplaceAll(block, first.ID, second.ID)
	block = strings.ReplaceAll(block, first.Name, second.Name)
	second.Line = strings.Count(content, "\n") + 3
	for i := range second.Steps {
		second.Steps[i].Line += second.Line - first.Line
	}
	s.Documents[1].Content += "\n" + block
	s.Documents[1].ContentHash = Hash([]byte(s.Documents[1].Content))
	s.Source.ContentHash = CatalogueHash(s.Documents)
	s.Verification.SourceContentHash = s.Source.ContentHash
	s.Scenarios = append(s.Scenarios, second)
	secondResult := s.Verification.Cases[0]
	secondResult.ScenarioID = second.ID
	s.Verification.Cases = append([]Result{secondResult}, s.Verification.Cases...)
	if _, err := s.Entries(); err != nil {
		t.Fatal("multi-scenario fixture must match the real Gherkin parser", err)
	}
	events := decodeEvents(t, testMessages(t, s))
	runFinished := events[len(events)-1]
	events = events[:len(events)-1]
	secondSnapshot := s
	secondSnapshot.Scenarios = []Scenario{second}
	secondEvents := decodeEvents(t, testMessages(t, secondSnapshot))
	ids := []string{"pickle", "case", "started", "hook-step"}
	for i := range second.Steps {
		ids = append(ids, fmt.Sprintf("ps%d", i), fmt.Sprintf("ts%d", i), fmt.Sprintf("def%d", i))
	}
	replacements := map[string]string{}
	for _, id := range ids {
		replacements[id] = id + "-second"
	}
	var renameValues func(any) any
	renameValues = func(value any) any {
		switch node := value.(type) {
		case string:
			if replacement, ok := replacements[node]; ok {
				return replacement
			}
		case map[string]any:
			for key, child := range node {
				node[key] = renameValues(child)
			}
		case []any:
			for i, child := range node {
				node[i] = renameValues(child)
			}
		}
		return value
	}
	for _, event := range secondEvents {
		renameValues(event)
		if event["source"] == nil && event["testRunStarted"] == nil && event["testRunFinished"] == nil {
			events = append(events, event)
		}
	}
	events = append(events, runFinished)
	if err := ValidateMessages(encodeEvents(t, events), s); err != nil {
		t.Fatal("same complete scenario execution rejected after result reordering", err)
	}
	if s.Eligible() == nil {
		t.Fatal("synthetic multi-scenario fixture became live eligible")
	}
}
