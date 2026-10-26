package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"viki/internal/hermes"
	"viki/internal/model"
	"viki/internal/security"
	"viki/internal/specsync"
)

const syncConversationID = "00000000-0000-4000-8000-000000000099"

type syncRepository struct {
	*assistantToolRepository
	status    model.WorkspaceSyncStatus
	statusErr error
	importErr error
	recordErr error
	imports   int
	records   []string
	lastOrg   string
}

func (r *syncRepository) WorkspaceSyncStatus(_ context.Context, org string) (model.WorkspaceSyncStatus, error) {
	r.lastOrg = org
	return r.status, r.statusErr
}
func (r *syncRepository) ImportRepositorySnapshot(_ context.Context, org string, _ specsync.Snapshot) (model.WorkspaceSyncStatus, error) {
	r.imports++
	r.lastOrg = org
	return r.status, r.importErr
}
func (r *syncRepository) RecordRepositoryPoll(_ context.Context, org, code string) error {
	r.records = append(r.records, code)
	r.lastOrg = org
	return r.recordErr
}
func (r *syncRepository) SessionByHash(context.Context, []byte) (model.Session, error) {
	return model.Session{User: model.User{ID: "user"}, OrganizationID: "workspace-session", CSRFHash: security.HashToken("csrf"), Expires: time.Now().Add(time.Hour)}, nil
}
func (r *syncRepository) CreateAssistantConversation(context.Context, string, string, model.AssistantMode) (model.AssistantConversation, error) {
	return r.conversation, nil
}
func (r *syncRepository) SetAssistantPrimaryMode(context.Context, string, string, string, model.AssistantMode) error {
	return nil
}
func newSyncApp(t *testing.T) (*Application, *syncRepository) {
	t.Helper()
	r := &syncRepository{assistantToolRepository: &assistantToolRepository{conversation: model.AssistantConversation{ID: syncConversationID, OrganizationID: "workspace-session", UserID: "user", PrimaryMode: model.AssistantQA, LastMode: model.AssistantQA}}, status: model.WorkspaceSyncStatus{Mode: "git_managed", Repository: specsync.Repository}}
	a := NewApplication(r, hermes.NewFakeGateway(), Options{SnapshotImportToken: strings.Repeat("s", 64), HermesToolToken: "separate-hermes-token"}, discardLogger())
	t.Cleanup(a.Close)
	return a, r
}
func invokeSync(handler http.Handler, method, path, body, token string, session bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	if session {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
		req.Header.Set("X-CSRF-Token", "csrf")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
func liveTestSnapshot(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../specsync/testdata/snapshot-v1.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := specsync.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	s.Source.Commit = strings.Repeat("1", 40)
	s.Verification.SourceCommit = s.Source.Commit
	s.Source.Dirty = false
	s.Publication = specsync.Publication{Kind: "main", Event: "push", Ref: "refs/heads/main", Workflow: specsync.Workflow, RunID: "10", RunAttempt: 1, Eligible: true}
	s.Verification.MessagesHash = "sha256:" + strings.Repeat("a", 64)
	data, _ = json.Marshal(s)
	return string(data)
}
func TestPrivateImportAuthenticationAndLastGoodPreservation(t *testing.T) {
	a, r := newSyncApp(t)
	valid := liveTestSnapshot(t)
	token := "Bearer " + strings.Repeat("s", 64)
	for _, credential := range []string{"", "Bearer separate-hermes-token", "Bearer wrong"} {
		response := invokeSync(a.InternalHandler(), "POST", "/internal/v1/repository-snapshots", valid, credential, true)
		if response.Code != 401 || r.imports != 0 || len(r.records) != 0 {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	if response := invokeSync(a.PublicHandler(), "POST", "/internal/v1/repository-snapshots", valid, token, true); response.Code != 404 {
		t.Fatal("private importer exposed", response.Code)
	}
	if response := invokeSync(a.InternalHandler(), "POST", "/internal/v1/repository-snapshots", valid, token, false); response.Code != 200 || r.imports != 1 {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, body := range []string{"{", strings.Replace(valid, `"name":"Add a customer`, `"name":"Changed customer`, 1), strings.Replace(valid, `"kind":"main"`, `"kind":"local"`, 1)} {
		response := invokeSync(a.InternalHandler(), "POST", "/internal/v1/repository-snapshots", body, token, false)
		if response.Code != 422 || r.imports != 1 {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	r.importErr = errors.New("stale snapshot")
	response := invokeSync(a.InternalHandler(), "POST", "/internal/v1/repository-snapshots", valid, token, false)
	if response.Code != 409 || r.records[len(r.records)-1] != "snapshot_rejected" {
		t.Fatal(response.Code, r.records)
	}
	large := strings.Repeat("x", 8*1024*1024+1)
	if response = invokeSync(a.InternalHandler(), "POST", "/internal/v1/repository-snapshots", large, token, false); response.Code != 400 {
		t.Fatal(response.Code)
	}
}
func TestGitManagedPublicWritesAndAssistantEditAreDeniedWhileQAStaysAvailable(t *testing.T) {
	a, r := newSyncApp(t)
	for _, path := range []string{"/api/v1/pages", "/api/v1/pages/" + syncConversationID + "/revisions", "/api/v1/revisions/" + syncConversationID + "/approve", "/api/v1/revisions/" + syncConversationID + "/objections", "/api/v1/comments", "/api/v1/objections/" + syncConversationID + "/resolve"} {
		response := invokeSync(a.PublicHandler(), "POST", path, "{}", "", true)
		if response.Code != 403 || !strings.Contains(response.Body.String(), "git_managed_read_only") {
			t.Fatal(path, response.Code, response.Body.String())
		}
	}
	for _, item := range []struct{ method, path, body string }{{"POST", "/api/v1/assistant/conversations", `{"primaryMode":"edit"}`}, {"PATCH", "/api/v1/assistant/conversations/" + syncConversationID, `{"primaryMode":"edit"}`}, {"POST", "/api/v1/assistant/conversations/" + syncConversationID + "/messages", `{"mode":"edit","content":"create a draft"}`}} {
		response := invokeSync(a.PublicHandler(), item.method, item.path, item.body, "", true)
		if response.Code != 403 {
			t.Fatal(item, response.Code, response.Body.String())
		}
	}
	response := invokeSync(a.PublicHandler(), "POST", "/api/v1/assistant/conversations", `{"primaryMode":"qa"}`, "", true)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	response = invokeSync(a.PublicHandler(), "GET", "/api/v1/assistant/status", "", "", true)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"ready":false`) || !strings.Contains(response.Body.String(), `"ready":true`) {
		t.Fatal(response.Code, response.Body.String())
	}
	if r.lastOrg != "workspace-session" {
		t.Fatal("status ignored authenticated workspace", r.lastOrg)
	}
	response = invokeSync(a.PublicHandler(), "GET", "/api/v1/workspace/sync", "", "", true)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "git_managed") {
		t.Fatal(response.Code, response.Body.String())
	}
	r.statusErr = errors.New("status unavailable")
	response = invokeSync(a.PublicHandler(), "POST", "/api/v1/pages", "{}", "", true)
	if response.Code == 200 {
		t.Fatal("status failure allowed writes")
	}
	for _, path := range []string{"/api/v1/workspace/sync", "/api/v1/assistant/status"} {
		if response = invokeSync(a.PublicHandler(), "GET", path, "", "", true); response.Code == 200 {
			t.Fatal("status failure hidden")
		}
	}
}
func TestPrivatePollStatusAndErrors(t *testing.T) {
	a, r := newSyncApp(t)
	token := "Bearer " + strings.Repeat("s", 64)
	for _, code := range []string{"", "github_authentication_failed", "github_unavailable", "artifact_unavailable", "candidate_rejected", "import_unavailable", "no_eligible_main_run"} {
		body, _ := json.Marshal(map[string]string{"error": code})
		response := invokeSync(a.InternalHandler(), "POST", "/internal/v1/repository-sync/status", string(body), token, false)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	for _, body := range []string{"{", `{"error":"unknown"}`} {
		response := invokeSync(a.InternalHandler(), "POST", "/internal/v1/repository-sync/status", body, token, false)
		if response.Code != 400 {
			t.Fatal(response.Code)
		}
	}
	if response := invokeSync(a.InternalHandler(), "GET", "/internal/v1/repository-sync", "", token, false); response.Code != 200 {
		t.Fatal(response.Code)
	}
	r.statusErr = io.EOF
	if response := invokeSync(a.InternalHandler(), "GET", "/internal/v1/repository-sync", "", token, false); response.Code == 200 {
		t.Fatal(response.Code)
	}
	r.recordErr = io.EOF
	if response := invokeSync(a.InternalHandler(), "POST", "/internal/v1/repository-sync/status", `{"error":""}`, token, false); response.Code == 200 {
		t.Fatal(response.Code)
	}
}
func TestGitManagedHermesToolsCannotForkAndQACitesBaseline(t *testing.T) {
	_, r := newSyncApp(t)
	conversation := toolConversation()
	r.conversation = conversation
	revisionID := "repo-version"
	r.pageDetail = model.PageDetail{Page: model.Page{ID: "repo-page", RepositoryRevisionID: &revisionID}, RepositoryRevision: &model.Revision{ID: revisionID, PageID: "repo-page", Status: model.RevisionRepositoryBaseline}}
	r.revision = *r.pageDetail.RepositoryRevision
	runtime := bareAssistantRuntime(r, nil)
	for _, mode := range []model.AssistantMode{model.AssistantQA, model.AssistantEdit} {
		stored := string(mode) + "-stored"
		runtime.activeByStored[assistantSessionKey(mode, stored)] = &assistantTurn{ID: "turn-" + string(mode), Mode: mode, StoredID: stored, ConversationID: conversation.ID, OrganizationID: conversation.OrganizationID, UserID: conversation.UserID}
	}
	server := &Server{repository: r, assistant: runtime, options: Options{HermesToolToken: "service-secret"}, logger: discardLogger()}
	invoke := func(profile, tool, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		req.SetPathValue("tool", tool)
		req.Header.Set("Authorization", "Bearer service-secret")
		req.Header.Set("X-Hermes-Profile", profile)
		req.Header.Set("X-Hermes-Session-ID", profile+"-stored")
		response := httptest.NewRecorder()
		server.handleHermesTool(response, req)
		return response
	}
	response := invoke("edit", "apply_viki_draft_changeset", `{"operations":[]}`)
	if response.Code != 403 || !strings.Contains(response.Body.String(), "git_managed_read_only") {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, tool := range []string{"get_viki_page", "get_viki_revision"} {
		body := `{"pageId":"repo-page"}`
		if tool == "get_viki_revision" {
			body = `{"revisionId":"repo-version"}`
		}
		response = invoke("qa", tool, body)
		if response.Code != 200 || !strings.Contains(response.Body.String(), "repository_baseline") {
			t.Fatal(tool, response.Code, response.Body.String())
		}
	}
	r.pageDetail.Page.Archived = true
	for _, tool := range []string{"get_viki_page", "get_viki_revision"} {
		body := `{"pageId":"repo-page"}`
		if tool == "get_viki_revision" {
			body = `{"revisionId":"repo-version"}`
		}
		if response = invoke("qa", tool, body); response.Code != 404 {
			t.Fatal("removed source remains in Q&A", tool, response.Code)
		}
	}
	r.statusErr = io.EOF
	if response = invoke("edit", "apply_viki_draft_changeset", `{}`); response.Code == 200 {
		t.Fatal("status failure allowed Edit")
	}
}

func TestSyncFailsClosedWhenCapabilityOrAuthorityIsUnavailable(t *testing.T) {
	server := &Server{repository: &assistantToolRepository{}, options: Options{SnapshotImportToken: strings.Repeat("s", 64)}, logger: discardLogger()}
	token := "Bearer " + strings.Repeat("s", 64)
	for _, path := range []string{"/internal/v1/repository-snapshots", "/internal/v1/repository-sync/status"} {
		response := invokeSync(server.internalHandler(), "POST", path, "{}", token, false)
		if response.Code != 503 {
			t.Fatal(path, response.Code)
		}
	}
	for _, item := range []struct{ method, path string }{{"GET", "/internal/v1/repository-sync"}, {"POST", "/internal/v1/repository-sync/status"}} {
		response := invokeSync(server.internalHandler(), item.method, item.path, "{}", "Bearer wrong", false)
		if response.Code != 401 {
			t.Fatal(item, response.Code)
		}
	}
	_, repo := newSyncApp(t)
	repo.statusErr = io.EOF
	server.repository = repo
	response := httptest.NewRecorder()
	if server.permitAssistantMode(response, httptest.NewRequest("POST", "/", nil), authState{Session: model.Session{OrganizationID: "workspace"}}, model.AssistantEdit) || response.Code == 200 {
		t.Fatal("unknown authority permitted Edit")
	}
}
