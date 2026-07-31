package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"viki/internal/app"
	"viki/internal/hermes"
	"viki/internal/model"
	"viki/internal/security"
	"viki/internal/store"
)

type assistantRepository struct {
	store.Repository
	mu            sync.Mutex
	session       model.Session
	conversation  model.AssistantConversation
	documents     []model.RetrievedDocument
	includeDrafts bool
}

func (r *assistantRepository) Retrieve(_ context.Context, organizationID, _ string, includeDrafts bool, _ int) ([]model.RetrievedDocument, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if organizationID != r.conversation.OrganizationID {
		return nil, store.ErrNotFound
	}
	r.includeDrafts = includeDrafts
	return append([]model.RetrievedDocument(nil), r.documents...), nil
}

func (r *assistantRepository) SessionByHash(context.Context, []byte) (model.Session, error) {
	return r.session, nil
}

func (r *assistantRepository) AssistantConversation(_ context.Context, organizationID, userID, conversationID string) (model.AssistantConversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conversation.ID != conversationID || r.conversation.OrganizationID != organizationID || r.conversation.UserID != userID {
		return model.AssistantConversation{}, store.ErrNotFound
	}
	return r.conversation, nil
}

func (r *assistantRepository) ListAssistantConversations(_ context.Context, organizationID, userID string) ([]model.AssistantConversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conversation.OrganizationID != organizationID || r.conversation.UserID != userID {
		return []model.AssistantConversation{}, nil
	}
	return []model.AssistantConversation{r.conversation}, nil
}

func (r *assistantRepository) AssistantDraftReceipts(context.Context, string, string) (map[string][]model.AssistantDraftReceipt, error) {
	return map[string][]model.AssistantDraftReceipt{}, nil
}

func (r *assistantRepository) SetAssistantSession(_ context.Context, organizationID, userID, conversationID string, mode model.AssistantMode, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conversation.ID != conversationID || r.conversation.OrganizationID != organizationID || r.conversation.UserID != userID {
		return store.ErrNotFound
	}
	if mode == model.AssistantQA {
		r.conversation.QASessionID = &sessionID
	} else {
		r.conversation.EditSessionID = &sessionID
	}
	return nil
}

func (r *assistantRepository) UpdateAssistantMode(_ context.Context, _, _, _ string, mode model.AssistantMode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conversation.LastMode = mode
	return nil
}

func TestAssistantMessageIsAcceptedOnceAndHermesOwnsItsBody(t *testing.T) {
	t.Parallel()

	csrf := "assistant-csrf"
	conversationID := "00000000-0000-4000-8000-000000000099"
	repository := &assistantRepository{
		session: model.Session{
			User:           model.User{ID: "00000000-0000-4000-8000-000000000011", Email: "matej@matejlukasik.com"},
			OrganizationID: "00000000-0000-4000-8000-000000000001",
			CSRFHash:       security.HashToken(csrf),
			Expires:        time.Now().Add(time.Hour),
		},
		conversation: model.AssistantConversation{
			ID:             conversationID,
			OrganizationID: "00000000-0000-4000-8000-000000000001",
			UserID:         "00000000-0000-4000-8000-000000000011",
			PrimaryMode:    model.AssistantQA,
			LastMode:       model.AssistantQA,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
	}
	gateway := hermes.NewFakeGateway()
	application := app.NewApplication(repository, gateway, app.Options{HandoffSigningKey: "test-signing-key"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer application.Close()

	request := authenticatedJSONRequest(t, http.MethodPost, "/api/v1/assistant/conversations/"+conversationID+"/messages", csrf, map[string]any{
		"content": "Čo znamená zmluva?",
		"mode":    "qa",
	})
	recorder := httptest.NewRecorder()
	application.PublicHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}
	var accepted struct {
		TurnID string `json:"turnId"`
		Mode   string `json:"mode"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.TurnID == "" || accepted.Mode != "qa" {
		t.Fatalf("unexpected acceptance response: %+v", accepted)
	}

	request = authenticatedJSONRequest(t, http.MethodPost, "/api/v1/assistant/conversations/"+conversationID+"/messages", csrf, map[string]any{
		"content": "Druhá správa",
		"mode":    "qa",
	})
	recorder = httptest.NewRecorder()
	application.PublicHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("concurrent status = %d, want 409; body=%s", recorder.Code, recorder.Body.String())
	}

	repository.mu.Lock()
	storedID := *repository.conversation.QASessionID
	repository.mu.Unlock()
	history, err := gateway.History(context.Background(), model.AssistantQA, "runtime-"+storedID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Role != "user" || !bytes.Contains([]byte(history[0].Text), []byte("Čo znamená zmluva?")) {
		t.Fatalf("Hermes did not receive the user body: %+v", history)
	}
}

func TestHermesToolBridgeRequiresAnExactActiveTurnGrant(t *testing.T) {
	t.Parallel()

	csrf := "tool-csrf"
	conversationID := "00000000-0000-4000-8000-000000000098"
	repository := &assistantRepository{
		session: model.Session{
			User:           model.User{ID: "00000000-0000-4000-8000-000000000011", Email: "matej@matejlukasik.com"},
			OrganizationID: "00000000-0000-4000-8000-000000000001",
			CSRFHash:       security.HashToken(csrf),
			Expires:        time.Now().Add(time.Hour),
		},
		conversation: model.AssistantConversation{
			ID: conversationID, OrganizationID: "00000000-0000-4000-8000-000000000001",
			UserID: "00000000-0000-4000-8000-000000000011", PrimaryMode: model.AssistantQA, LastMode: model.AssistantQA,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
		documents: []model.RetrievedDocument{{
			RevisionID: "00000000-0000-4000-8000-000000000077",
			PageID:     "00000000-0000-4000-8000-000000000066", PageTitle: "Zmluva", Content: "Prijatý obsah",
		}},
	}
	gateway := hermes.NewFakeGateway()
	application := app.NewApplication(repository, gateway, app.Options{
		HermesToolToken: "service-secret", HandoffSigningKey: "test-signing-key",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer application.Close()

	submit := authenticatedJSONRequest(t, http.MethodPost, "/api/v1/assistant/conversations/"+conversationID+"/messages", csrf, map[string]any{
		"content": "Nájdi zmluvu", "mode": "qa",
	})
	submitRecorder := httptest.NewRecorder()
	application.PublicHandler().ServeHTTP(submitRecorder, submit)
	if submitRecorder.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d; body=%s", submitRecorder.Code, submitRecorder.Body.String())
	}
	repository.mu.Lock()
	storedID := *repository.conversation.QASessionID
	repository.mu.Unlock()

	search := internalToolRequest(t, "search_viki", "service-secret", "qa", storedID, map[string]any{"query": "zmluva"})
	searchRecorder := httptest.NewRecorder()
	application.InternalHandler().ServeHTTP(searchRecorder, search)
	if searchRecorder.Code != http.StatusOK {
		t.Fatalf("active search status = %d; body=%s", searchRecorder.Code, searchRecorder.Body.String())
	}
	repository.mu.Lock()
	includeDrafts := repository.includeDrafts
	repository.mu.Unlock()
	if !includeDrafts {
		t.Fatal("Q&A tool did not include current concept revisions")
	}
	runtimeIdentity := internalToolRequest(t, "search_viki", "service-secret", "qa", "runtime-"+storedID, map[string]any{"query": "zmluva"})
	runtimeIdentityRecorder := httptest.NewRecorder()
	application.InternalHandler().ServeHTTP(runtimeIdentityRecorder, runtimeIdentity)
	if runtimeIdentityRecorder.Code != http.StatusForbidden {
		t.Fatalf("ephemeral runtime identity status = %d, want 403; body=%s", runtimeIdentityRecorder.Code, runtimeIdentityRecorder.Body.String())
	}

	mutation := internalToolRequest(t, "propose_viki_changeset", "service-secret", "qa", storedID, map[string]any{"summary": "no", "operations": []any{}})
	mutationRecorder := httptest.NewRecorder()
	application.InternalHandler().ServeHTTP(mutationRecorder, mutation)
	if mutationRecorder.Code != http.StatusForbidden {
		t.Fatalf("Q&A mutation status = %d, want 403; body=%s", mutationRecorder.Code, mutationRecorder.Body.String())
	}

	gateway.Emit(model.AssistantQA, hermes.Event{
		Type: "message.complete", SessionID: "runtime-" + storedID,
		Payload: json.RawMessage(`{"text":"Hotovo","status":"complete"}`),
	})
	deadline := time.Now().Add(time.Second)
	for {
		search = internalToolRequest(t, "search_viki", "service-secret", "qa", storedID, map[string]any{"query": "zmluva"})
		searchRecorder = httptest.NewRecorder()
		application.InternalHandler().ServeHTTP(searchRecorder, search)
		if searchRecorder.Code == http.StatusForbidden {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("completed turn retained its tool grant; status=%d", searchRecorder.Code)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAssistantConversationListDerivesDisplayTitleFromHermesHistory(t *testing.T) {
	t.Parallel()

	storedID := "qa-stored-existing"
	repository := &assistantRepository{
		session: model.Session{
			User:           model.User{ID: "00000000-0000-4000-8000-000000000011", Email: "matej@matejlukasik.com"},
			OrganizationID: "00000000-0000-4000-8000-000000000001",
			Expires:        time.Now().Add(time.Hour),
		},
		conversation: model.AssistantConversation{
			ID: "00000000-0000-4000-8000-000000000097", OrganizationID: "00000000-0000-4000-8000-000000000001",
			UserID: "00000000-0000-4000-8000-000000000011", QASessionID: &storedID,
			PrimaryMode: model.AssistantQA, LastMode: model.AssistantQA, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
	}
	gateway := hermes.NewFakeGateway()
	gateway.Sessions[model.AssistantQA][storedID] = []hermes.HistoryMessage{
		{Role: "user", Text: "Ako funguje príprava zmluvy pre firmu?"},
		{Role: "assistant", Text: "Podľa prijatých podkladov…"},
	}
	application := app.NewApplication(repository, gateway, app.Options{HandoffSigningKey: "test-signing-key"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer application.Close()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/assistant/conversations", nil)
	request.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
	recorder := httptest.NewRecorder()
	application.PublicHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Conversations []model.AssistantConversation `json:"conversations"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Conversations) != 1 || response.Conversations[0].Title != "Ako funguje príprava zmluvy pre firmu?" {
		t.Fatalf("unexpected conversation summaries: %+v", response.Conversations)
	}
	if response.Conversations[0].Messages != nil {
		t.Fatalf("conversation list leaked transcript bodies: %+v", response.Conversations[0].Messages)
	}
}

func internalToolRequest(t *testing.T, tool, token, profile, sessionID string, body any) *http.Request {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/hermes/tools/"+tool, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Hermes-Profile", profile)
	request.Header.Set("X-Hermes-Session-ID", sessionID)
	return request
}

func authenticatedJSONRequest(t *testing.T, method, path, csrf string, body any) *http.Request {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
	request.AddCookie(&http.Cookie{Name: "viki_csrf", Value: csrf})
	request.Header.Set("X-CSRF-Token", csrf)
	return request
}
