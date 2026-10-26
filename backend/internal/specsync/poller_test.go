package specsync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"viki/internal/model"
)

func testMessages(t *testing.T, s Snapshot) []byte {
	t.Helper()
	events := []map[string]any{{"testRunStarted": map[string]any{"timestamp": map[string]int{"seconds": 1, "nanos": 0}}}, {"source": map[string]any{"uri": s.Documents[1].Path, "data": s.Documents[1].Content}}}
	steps := []map[string]any{}
	testSteps := []map[string]any{{"id": "hook-step", "hookId": "hook"}}
	for i, step := range s.Scenarios[0].Steps {
		id := fmt.Sprint(i)
		steps = append(steps, map[string]any{"id": "ps" + id, "text": step.Text})
		testSteps = append(testSteps, map[string]any{"id": "ts" + id, "pickleStepId": "ps" + id, "stepDefinitionIds": []string{"def" + id}})
		events = append(events, map[string]any{"stepDefinition": map[string]any{"id": "def" + id}})
	}
	events = append(events, map[string]any{"pickle": map[string]any{"id": "pickle", "uri": s.Documents[1].Path, "name": s.Scenarios[0].Name, "language": "en", "tags": []map[string]string{{"name": "@id:" + s.Scenarios[0].ID}}, "steps": steps}}, map[string]any{"testCase": map[string]any{"id": "case", "pickleId": "pickle", "testSteps": testSteps}}, map[string]any{"testCaseStarted": map[string]any{"id": "started", "testCaseId": "case", "attempt": 0}}, map[string]any{"attachment": map[string]any{"testCaseStartedId": "started", "mediaType": "application/vnd.vozokany.fixture+json", "body": `{"backend":"rust"}`}})
	for _, step := range testSteps {
		nanos := 250000000
		if step["hookId"] != nil {
			nanos = 0
		}
		events = append(events, map[string]any{"testStepFinished": map[string]any{"testCaseStartedId": "started", "testStepId": step["id"], "testStepResult": map[string]any{"status": "PASSED", "duration": map[string]int{"seconds": 0, "nanos": nanos}}}})
	}
	events = append(events, map[string]any{"testCaseFinished": map[string]any{"testCaseStartedId": "started", "willBeRetried": false}}, map[string]any{"testRunFinished": map[string]any{"success": true}})
	return encodeEvents(t, events)
}
func encodeEvents(t *testing.T, events []map[string]any) []byte {
	t.Helper()
	var b bytes.Buffer
	for _, e := range events {
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.Bytes()
}
func decodeEvents(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	events := []map[string]any{}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var e map[string]any
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	return events
}
func mainFixture(t *testing.T) Snapshot {
	s := fixture(t)
	s.Source.Commit = strings.Repeat("1", 40)
	s.Source.Dirty = false
	s.Verification.SourceCommit = s.Source.Commit
	s.Publication = Publication{Kind: "main", Event: "push", Ref: "refs/heads/main", Workflow: Workflow, RunID: "10", RunAttempt: 1, Eligible: true}
	return s
}
func archiveFor(t *testing.T, s *Snapshot, raw []byte, edit func(map[string][]byte)) []byte {
	t.Helper()
	s.Verification.MessagesHash = Hash(raw)
	snapshot, _ := json.Marshal(s)
	execution, _ := json.Marshal(map[string]any{"source": map[string]any{"commit": s.Source.Commit, "dirty": false, "fingerprint": Hash([]byte{0})}, "publication": s.Publication, "verification": s.Verification})
	files := map[string][]byte{"snapshot.json": snapshot, "execution.json": execution, "cucumber-messages.ndjson": raw}
	if edit != nil {
		edit(files)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, name := range []string{"snapshot.json", "execution.json", "cucumber-messages.ndjson"} {
		w, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

type pollFixture struct {
	poller        *Poller
	snapshot      Snapshot
	run           githubRun
	artifact      githubArtifact
	archive       []byte
	current       model.WorkspaceSyncStatus
	imports       int
	records       []string
	branch        string
	branchReads   int
	moveBranch    bool
	code          map[string]int
	repository    githubRepository
	workflowID    int64
	workflowPath  string
	extraArtifact bool
	noRuns        bool
	total         int
}

func newPollFixture(t *testing.T) *pollFixture {
	t.Helper()
	f := &pollFixture{snapshot: mainFixture(t), repository: githubRepository{ID: 7, FullName: "matk0/vozokany"}, workflowID: 5, workflowPath: Workflow, code: map[string]int{}, total: 1}
	f.current = model.WorkspaceSyncStatus{Mode: "git_managed", Repository: Repository}
	f.run = githubRun{ID: 10, Attempt: 1, HeadSHA: f.snapshot.Source.Commit, HeadBranch: "main", Event: "push", Path: Workflow, Status: "completed", Conclusion: "success", WorkflowID: 5, Repository: f.repository, HeadRepository: f.repository}
	f.branch = f.run.HeadSHA
	f.archive = archiveFor(t, &f.snapshot, testMessages(t, f.snapshot), nil)
	f.artifact = githubArtifact{ID: 17, Name: "vozokany-business-specs-v1-" + f.run.HeadSHA + "-10-1", ExpiresAt: time.Now().Add(time.Hour), Size: int64(len(f.archive)), Digest: Hash(f.archive)}
	f.artifact.Run.ID = 10
	f.artifact.Run.RepositoryID = 7
	f.artifact.Run.HeadRepositoryID = 7
	f.artifact.Run.HeadSHA = f.run.HeadSHA
	f.artifact.Run.HeadBranch = "main"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "github-scoped-test"
		if strings.HasPrefix(r.URL.Path, "/internal/") {
			expected = "import-scoped-test"
		}
		if r.Header.Get("Authorization") != "Bearer "+expected {
			t.Error("wrong credential audience")
		}
		if code := f.code[r.URL.Path]; code != 0 {
			w.WriteHeader(code)
			return
		}
		respond := func(x any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(x) }
		switch r.URL.Path {
		case "/internal/v1/repository-sync":
			respond(f.current)
		case "/internal/v1/repository-sync/status":
			var record map[string]string
			_ = json.NewDecoder(r.Body).Decode(&record)
			f.records = append(f.records, record["error"])
			respond(map[string]bool{"recorded": true})
		case "/internal/v1/repository-snapshots":
			f.imports++
			var s Snapshot
			if json.NewDecoder(r.Body).Decode(&s) != nil {
				t.Error("invalid imported JSON")
			}
			f.current.Commit = s.Source.Commit
			f.current.RunID = s.Publication.RunID
			f.current.RunAttempt = s.Publication.RunAttempt
			f.current.ContentHash = s.Source.ContentHash
			respond(f.current)
		case repositoryAPI:
			respond(f.repository)
		case repositoryAPI + "/branches/main":
			f.branchReads++
			sha := f.branch
			if f.moveBranch && f.branchReads > 1 {
				sha = strings.Repeat("9", 40)
			}
			respond(map[string]any{"commit": map[string]string{"sha": sha}})
		case repositoryAPI + "/actions/workflows/verify.yml":
			respond(map[string]any{"id": f.workflowID, "path": f.workflowPath})
		case repositoryAPI + "/actions/workflows/verify.yml/runs":
			runs := []githubRun{f.run}
			if f.noRuns {
				runs = nil
			}
			respond(map[string]any{"workflow_runs": runs})
		case repositoryAPI + "/actions/runs/10/attempts/1":
			respond(f.run)
		case repositoryAPI + "/actions/runs/10/artifacts":
			artifacts := []githubArtifact{f.artifact}
			if f.extraArtifact {
				artifacts = append(artifacts, f.artifact)
			}
			respond(map[string]any{"total_count": f.total, "artifacts": artifacts})
		case repositoryAPI + "/actions/artifacts/17/zip":
			_, _ = w.Write(f.archive)
		default:
			if strings.HasPrefix(r.URL.Path, repositoryAPI+"/compare/") {
				respond(map[string]any{"status": "ahead", "base_commit": map[string]string{"sha": f.current.Commit}, "merge_base_commit": map[string]string{"sha": f.current.Commit}})
			} else {
				t.Errorf("unexpected route %s", r.URL.Path)
				w.WriteHeader(404)
			}
		}
	}))
	t.Cleanup(server.Close)
	f.poller = &Poller{Client: server.Client(), GitHubURL: server.URL, ImportURL: server.URL, GitHubToken: "github-scoped-test", ImportToken: "import-scoped-test", Now: time.Now, RetryDelay: time.Millisecond}
	return f
}
func TestWorkerImportsOnlyEligibleMainArtifactAndRecordsUnchangedPoll(t *testing.T) {
	f := newPollFixture(t)
	receipt, err := f.poller.Poll(context.Background())
	if err != nil || receipt.State != "imported" || f.imports != 1 {
		t.Fatal(receipt, err, f.imports)
	}
	receipt, err = f.poller.Poll(context.Background())
	if err != nil || receipt.State != "unchanged" || f.imports != 1 || len(f.records) != 1 {
		t.Fatal(receipt, err, f.imports, f.records)
	}
}
func TestWorkerRejectsIneligibleOrCorruptedCandidatesWithoutImporting(t *testing.T) {
	mutations := map[string]func(*pollFixture){"github auth": func(f *pollFixture) { f.code[repositoryAPI] = 401 }, "private auth": func(f *pollFixture) { f.code["/internal/v1/repository-sync"] = 403 }, "wrong repository": func(f *pollFixture) { f.repository.FullName = "other/repo" }, "wrong workflow": func(f *pollFixture) { f.workflowPath = "other.yml" }, "no run": func(f *pollFixture) { f.noRuns = true }, "failed run": func(f *pollFixture) { f.run.Conclusion = "failure" }, "pending run": func(f *pollFixture) { f.run.Status = "in_progress" }, "wrong event": func(f *pollFixture) { f.run.Event = "pull_request" }, "wrong branch": func(f *pollFixture) { f.run.HeadBranch = "other" }, "wrong sha": func(f *pollFixture) { f.branch = strings.Repeat("2", 40) }, "fork": func(f *pollFixture) { f.run.HeadRepository.ID = 99 }, "wrong run workflow": func(f *pollFixture) { f.run.WorkflowID = 9 }, "stale": func(f *pollFixture) { f.current.RunID = "11" }, "invalid previous run": func(f *pollFixture) { f.current.RunID = "invalid" }, "same run different commit": func(f *pollFixture) {
		f.current.RunID = "10"
		f.current.RunAttempt = 1
		f.current.Commit = strings.Repeat("2", 40)
	}, "expired artifact": func(f *pollFixture) { f.artifact.Expired = true }, "expired time": func(f *pollFixture) { f.artifact.ExpiresAt = time.Now().Add(-time.Hour) }, "wrong artifact run": func(f *pollFixture) { f.artifact.Run.ID = 9 }, "wrong artifact sha": func(f *pollFixture) { f.artifact.Run.HeadSHA = strings.Repeat("2", 40) }, "missing artifact": func(f *pollFixture) { f.artifact.Name = "other" }, "duplicate artifact": func(f *pollFixture) { f.extraArtifact = true; f.total = 2 }, "incomplete pagination": func(f *pollFixture) { f.total = 101 }, "archive digest": func(f *pollFixture) { f.artifact.Digest = "wrong" }, "moving main": func(f *pollFixture) { f.moveBranch = true }, "import failed": func(f *pollFixture) { f.code["/internal/v1/repository-snapshots"] = 503 }}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f := newPollFixture(t)
			mutate(f)
			if _, err := f.poller.Poll(context.Background()); err == nil {
				t.Fatal("accepted candidate")
			}
			if f.imports != 0 {
				t.Fatal("imported rejected candidate")
			}
		})
	}
	for _, name := range []string{"parsed source", "execution copy", "raw stream", "failed execution"} {
		t.Run(name, func(t *testing.T) {
			f := newPollFixture(t)
			raw := testMessages(t, f.snapshot)
			if name == "failed execution" {
				raw = bytes.Replace(raw, []byte(`"status":"PASSED"`), []byte(`"status":"SKIPPED"`), 1)
			}
			f.archive = archiveFor(t, &f.snapshot, raw, func(files map[string][]byte) {
				switch name {
				case "parsed source":
					files["snapshot.json"] = bytes.Replace(files["snapshot.json"], []byte(`"name":"Add a customer`), []byte(`"name":"Corrupted customer`), 1)
				case "execution copy":
					files["execution.json"] = []byte(`{}`)
				case "raw stream":
					files["cucumber-messages.ndjson"] = append(files["cucumber-messages.ndjson"], byte('x'))
				}
			})
			f.artifact.Digest = Hash(f.archive)
			f.artifact.Size = int64(len(f.archive))
			if _, err := f.poller.Poll(context.Background()); err == nil || f.imports != 0 {
				t.Fatal("corrupted evidence accepted", err)
			}
		})
	}
}
func TestCucumberStreamExactResultsAndSource(t *testing.T) {
	s := mainFixture(t)
	raw := testMessages(t, s)
	if err := ValidateMessages(raw, s); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func([]map[string]any) []map[string]any{
		"missing completion": func(e []map[string]any) []map[string]any { return e[:len(e)-1] }, "duplicate": func(e []map[string]any) []map[string]any { return append(e, e[0]) }, "missing source": func(e []map[string]any) []map[string]any {
			e[1]["source"].(map[string]any)["data"] = "changed"
			return e
		}, "step mismatch": func(e []map[string]any) []map[string]any {
			for _, v := range e {
				if p, ok := v["pickle"].(map[string]any); ok {
					p["name"] = "changed"
				}
			}
			return e
		}, "retry": func(e []map[string]any) []map[string]any {
			for _, v := range e {
				if p, ok := v["testCaseStarted"].(map[string]any); ok {
					p["attempt"] = 1
				}
			}
			return e
		}, "fixture": func(e []map[string]any) []map[string]any {
			for _, v := range e {
				if p, ok := v["attachment"].(map[string]any); ok {
					p["body"] = `{"backend":"node"}`
				}
			}
			return e
		}, "undefined step": func(e []map[string]any) []map[string]any {
			for _, v := range e {
				if p, ok := v["testCase"].(map[string]any); ok {
					p["testSteps"].([]any)[1].(map[string]any)["stepDefinitionIds"] = []string{}
				}
			}
			return e
		}}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMessages(encodeEvents(t, mutate(decodeEvents(t, raw))), s); err == nil {
				t.Fatal("invalid stream accepted")
			}
		})
	}
	if ValidateMessages([]byte("not json"), s) == nil {
		t.Fatal("invalid JSON accepted")
	}
}
func TestArtifactRejectsTraversalAndUnexpectedFiles(t *testing.T) {
	if _, err := ReadArtifact([]byte("not zip")); err == nil {
		t.Fatal("invalid ZIP")
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	_, _ = w.Create("../snapshot.json")
	_, _ = w.Create("execution.json")
	_, _ = w.Create("cucumber-messages.ndjson")
	_ = w.Close()
	if _, err := ReadArtifact(b.Bytes()); err == nil {
		t.Fatal("traversal accepted")
	}
}
func TestHTTPBoundedRetryLimitsAndRedirectCredentialIsolation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	p := &Poller{Client: server.Client(), RetryDelay: time.Millisecond}
	data, err := p.request(context.Background(), "GET", server.URL, "secret", nil, 2)
	if err != nil || string(data) != "ok" || calls != 3 {
		t.Fatal(calls, string(data), err)
	}
	if _, err = p.request(context.Background(), "GET", server.URL, "secret", nil, 1); err == nil {
		t.Fatal("body limit ignored")
	}
	client := NewHTTPClient()
	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "other.example"}, Header: http.Header{"Authorization": []string{"Bearer secret"}}}
	if err = client.CheckRedirect(req, []*http.Request{{URL: &url.URL{Scheme: "https", Host: "api.github.com"}}}); err != nil || req.Header.Get("Authorization") != "" {
		t.Fatal("redirect leaked credential", err)
	}
	req.URL.Scheme = "http"
	if client.CheckRedirect(req, nil) == nil {
		t.Fatal("HTTP downgrade allowed")
	}
}

type loopWorker struct {
	cancel   context.CancelFunc
	polls    int
	fail     bool
	recorded bool
}

func (w *loopWorker) Poll(context.Context) (Receipt, error) {
	w.polls++
	if w.polls == 2 {
		w.cancel()
	}
	if w.fail {
		return Receipt{}, pollError{"candidate_rejected"}
	}
	return Receipt{State: "unchanged"}, nil
}
func (w *loopWorker) Record(context.Context, string) error { w.recorded = true; return nil }
func TestRestartableWorkerLoopAndConfiguration(t *testing.T) {
	for _, fail := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		worker := &loopWorker{cancel: cancel, fail: fail}
		Run(ctx, worker, time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if worker.polls != 2 || worker.recorded != fail {
			t.Fatal(worker)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	Run(ctx, &loopWorker{}, time.Millisecond, slog.Default())
	for _, target := range []string{"http://example.com:8090", "https://viki:8090", "http://user:pass@viki:8090", "http://viki:8090/path", "http://viki:8090?token=x", ":invalid", "http://viki"} {
		_, err := WorkerFromEnvironment(func(key string) string {
			switch key {
			case "VIKI_GITHUB_READ_TOKEN":
				return "scoped"
			case "VIKI_SNAPSHOT_IMPORT_TOKEN":
				return strings.Repeat("s", 32)
			default:
				return target
			}
		})
		if err == nil {
			t.Fatal("accepted target", target)
		}
	}
	if ErrorCode(io.EOF) != "github_unavailable" {
		t.Fatal("error mapping")
	}
}
func TestProducerArtifactInteroperability(t *testing.T) {
	directory := os.Getenv("VIKI_TEST_PRODUCER_ARTIFACT_DIR")
	if directory == "" {
		t.Skip("set VIKI_TEST_PRODUCER_ARTIFACT_DIR for actual producer interchange test")
	}
	data, err := os.ReadFile(directory + "/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Entries(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(directory + "/cucumber-messages.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	if Hash(raw) != s.Verification.MessagesHash {
		t.Fatal("stream hash mismatch")
	}
	if err = ValidateMessages(raw, s); err != nil {
		t.Fatal(err)
	}
	if s.Publication.Eligible || s.Publication.Kind == "main" || s.Eligible() == nil {
		t.Fatal("local interoperability fixture must be rejected by live eligibility")
	}
	stepCount := 0
	for _, scenario := range s.Scenarios {
		stepCount += len(scenario.Steps)
	}
	t.Logf("Validated actual producer interchange: %d documents, %d scenarios, %d business steps; source %s; catalogue %s; publication %s; live eligibility rejected", len(s.Documents), len(s.Scenarios), stepCount, s.Source.Commit, s.Source.ContentHash, s.Publication.Kind)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failedBody struct{}

func (failedBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failedBody) Close() error             { return nil }
func TestHTTPFailuresAreBoundedAndDoNotExposeCredentials(t *testing.T) {
	p := &Poller{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })}, RetryDelay: time.Millisecond}
	if _, err := p.request(context.Background(), "GET", ":bad", "secret", nil, 100); err == nil {
		t.Fatal("invalid URL accepted")
	}
	if _, err := p.request(context.Background(), "GET", "https://api.example", "secret", nil, 100); err == nil {
		t.Fatal("transport failure hidden")
	}
	p.Client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: failedBody{}, Header: http.Header{}}, nil
	})
	if _, err := p.request(context.Background(), "GET", "https://api.example", "secret", nil, 100); err == nil {
		t.Fatal("truncated response accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.Client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	})
	if _, err := p.request(ctx, "GET", "https://api.example", "secret", nil, 100); err == nil {
		t.Fatal("canceled retry ignored")
	}
	if fmt.Sprint(pollError{"candidate_rejected"}) != "candidate_rejected" {
		t.Fatal("unsafe error representation")
	}
	p = newPollFixture(t).poller
	p.GitHubURL = ":bad"
	if err := p.get(context.Background(), "/", new(any)); err == nil {
		t.Fatal("upstream transport failure hidden")
	}
}
func TestWorkerRejectsEveryUntrustedBoundaryAndMovingRun(t *testing.T) {
	for _, path := range []string{repositoryAPI, repositoryAPI + "/branches/main", repositoryAPI + "/actions/workflows/verify.yml", repositoryAPI + "/actions/workflows/verify.yml/runs", repositoryAPI + "/actions/runs/10/attempts/1", repositoryAPI + "/actions/runs/10/artifacts", repositoryAPI + "/actions/artifacts/17/zip"} {
		t.Run(path, func(t *testing.T) {
			f := newPollFixture(t)
			f.code[path] = 404
			if _, err := f.poller.Poll(context.Background()); err == nil || f.imports != 0 {
				t.Fatal("upstream failure replaced baseline", err)
			}
		})
	}
	f := newPollFixture(t)
	f.current.Commit = strings.Repeat("0", 40)
	receipt, err := f.poller.Poll(context.Background())
	if err != nil || receipt.State != "imported" {
		t.Fatal("valid forward ancestry rejected", receipt, err)
	}
	for name, mutate := range map[string]func(*pollFixture){"invalid saved SHA": func(f *pollFixture) { f.current.Commit = "bad" }, "compare failure": func(f *pollFixture) {
		f.current.Commit = strings.Repeat("2", 40)
		f.code[repositoryAPI+"/compare/"+f.current.Commit+"..."+f.run.HeadSHA] = 404
	}, "stale attempt": func(f *pollFixture) { f.current.RunID = "10"; f.current.RunAttempt = 2 }, "invalid run": func(f *pollFixture) { f.run.ID = 0 }, "wrong private mode": func(f *pollFixture) { f.current.Mode = "human_governed" }, "local snapshot": func(f *pollFixture) {
		f.snapshot.Publication.Kind = "local"
		f.archive = archiveFor(t, &f.snapshot, testMessages(t, f.snapshot), nil)
		f.artifact.Digest = Hash(f.archive)
		f.artifact.Size = int64(len(f.archive))
	}, "archive invalid": func(f *pollFixture) { f.archive = []byte("invalid zip"); f.artifact.Digest = Hash(f.archive) }, "schema invalid": func(f *pollFixture) {
		f.archive = archiveFor(t, &f.snapshot, testMessages(t, f.snapshot), func(files map[string][]byte) { files["snapshot.json"] = []byte("{}") })
		f.artifact.Digest = Hash(f.archive)
	}, "mismatched snapshot run": func(f *pollFixture) {
		f.snapshot.Publication.RunID = "11"
		f.archive = archiveFor(t, &f.snapshot, testMessages(t, f.snapshot), nil)
		f.artifact.Digest = Hash(f.archive)
	}} {
		t.Run(name, func(t *testing.T) {
			f := newPollFixture(t)
			mutate(f)
			if _, err := f.poller.Poll(context.Background()); err == nil || f.imports != 0 {
				t.Fatal("boundary accepted", err)
			}
		})
	}
}
func TestRawMessagesRejectAllIncompleteExecutionForms(t *testing.T) {
	s := mainFixture(t)
	raw := testMessages(t, s)
	cases := map[string]func([]map[string]any) []map[string]any{}
	for _, key := range []string{"source", "pickle", "testCase", "testCaseStarted", "testCaseFinished", "stepDefinition", "testStepFinished"} {
		cases["duplicate "+key] = func(events []map[string]any) []map[string]any {
			for _, e := range events {
				if e[key] != nil {
					return append(events, e)
				}
			}
			return events
		}
	}
	for _, key := range []string{"parseError", "undefinedParameterType"} {
		cases[key] = func(events []map[string]any) []map[string]any {
			return append(events, map[string]any{key: map[string]any{}})
		}
	}
	mutations := map[string]struct {
		key    string
		change func(map[string]any)
	}{
		"failed run": {"testRunFinished", func(e map[string]any) { e["success"] = false }}, "retry completion": {"testCaseFinished", func(e map[string]any) { e["willBeRetried"] = true }}, "failed hook": {"testRunHookFinished", func(e map[string]any) { e["result"] = map[string]any{"status": "FAILED"} }}, "missing case": {"testCaseStarted", func(e map[string]any) { e["testCaseId"] = "missing" }}, "missing pickle": {"testCase", func(e map[string]any) { e["pickleId"] = "missing" }}, "unknown scenario": {"pickle", func(e map[string]any) { e["tags"] = []map[string]string{{"name": "@id:VOZ-SC-UNKNOWN"}} }}, "missing scenario tag": {"pickle", func(e map[string]any) { e["tags"] = []any{} }}, "changed pickle text": {"pickle", func(e map[string]any) { e["steps"].([]any)[0].(map[string]any)["text"] = "changed" }}, "missing step result": {"testCase", func(e map[string]any) {
			e["testSteps"] = append(e["testSteps"].([]any), map[string]any{"id": "missing", "hookId": "hook"})
		}}, "duplicate test step": {"testCase", func(e map[string]any) { steps := e["testSteps"].([]any); steps[1] = steps[0] }}, "result nonexistent": {"testStepFinished", func(e map[string]any) { e["testStepId"] = "missing" }}, "invalid duration": {"testStepFinished", func(e map[string]any) { e["testStepResult"].(map[string]any)["duration"] = nil }}, "ambiguous step": {"testCase", func(e map[string]any) {
			e["testSteps"].([]any)[1].(map[string]any)["stepDefinitionIds"] = []string{"def0", "def1"}
		}}, "unknown pickle step": {"testCase", func(e map[string]any) { e["testSteps"].([]any)[1].(map[string]any)["pickleStepId"] = "missing" }}, "unknown executable step": {"testCase", func(e map[string]any) { e["testSteps"].([]any)[0].(map[string]any)["hookId"] = "" }}, "missing Rust receipt": {"attachment", func(e map[string]any) { e["mediaType"] = "text/plain" }}, "invalid Rust receipt": {"attachment", func(e map[string]any) { e["body"] = "not JSON" }},
	}
	for name, m := range mutations {
		cases[name] = func(events []map[string]any) []map[string]any {
			found := false
			for _, e := range events {
				if body, ok := e[m.key].(map[string]any); ok {
					m.change(body)
					found = true
					break
				}
			}
			if !found {
				body := map[string]any{}
				m.change(body)
				events = append(events, map[string]any{m.key: body})
			}
			return events
		}
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMessages(encodeEvents(t, mutate(decodeEvents(t, raw))), s); err == nil {
				t.Fatal("invalid raw proof accepted")
			}
		})
	}
	if ValidateMessages(bytes.Repeat([]byte("x"), 4*1024*1024+1), s) == nil {
		t.Fatal("message line bound ignored")
	}
	mismatch := s
	mismatch.Verification.Cases = append([]Result{}, s.Verification.Cases...)
	mismatch.Verification.Cases[0].Duration = "1"
	if ValidateMessages(raw, mismatch) == nil {
		t.Fatal("copied result duration accepted")
	}
	// Passing run hooks are permitted; failed run hooks above are rejected.
	events := decodeEvents(t, raw)
	events = append(events, map[string]any{"testRunHookFinished": map[string]any{"result": map[string]any{"status": "PASSED"}}})
	if err := ValidateMessages(encodeEvents(t, events), s); err != nil {
		t.Fatal(err)
	}
}
