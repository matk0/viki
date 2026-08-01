package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"viki/internal/hermes"
	"viki/internal/model"
	"viki/internal/store"
)

type assistantHTTPRepository struct {
	store.Repository
	conversation      model.AssistantConversation
	conversations     []model.AssistantConversation
	conversationErr   error
	reloadErr         error
	conversationCalls int
	listErr           error
	createErr         error
	setPrimaryErr     error
	receipts          map[string][]model.AssistantDraftReceipt
	receiptsErr       error
	setPrimaryMode    model.AssistantMode
}

func (r *assistantHTTPRepository) ListAssistantConversations(context.Context, string, string) ([]model.AssistantConversation, error) {
	return append([]model.AssistantConversation(nil), r.conversations...), r.listErr
}

func (r *assistantHTTPRepository) CreateAssistantConversation(_ context.Context, organizationID, userID string, mode model.AssistantMode) (model.AssistantConversation, error) {
	conversation := r.conversation
	conversation.OrganizationID = organizationID
	conversation.UserID = userID
	conversation.PrimaryMode = mode
	conversation.LastMode = mode
	return conversation, r.createErr
}

func (r *assistantHTTPRepository) AssistantConversation(context.Context, string, string, string) (model.AssistantConversation, error) {
	r.conversationCalls++
	if r.conversationCalls > 1 && r.reloadErr != nil {
		return model.AssistantConversation{}, r.reloadErr
	}
	return r.conversation, r.conversationErr
}

func (r *assistantHTTPRepository) SetAssistantPrimaryMode(_ context.Context, _, _, _ string, mode model.AssistantMode) error {
	r.setPrimaryMode = mode
	return r.setPrimaryErr
}

func (r *assistantHTTPRepository) AssistantDraftReceipts(context.Context, string, string) (map[string][]model.AssistantDraftReceipt, error) {
	return r.receipts, r.receiptsErr
}

func bareAssistantRuntime(repository store.Repository, gateway hermes.Gateway) *assistantRuntime {
	return &assistantRuntime{
		ctx:                  context.Background(),
		repository:           repository,
		gateway:              gateway,
		signingKey:           []byte("test-signing-key"),
		logger:               discardLogger(),
		activeByConversation: map[string]*assistantTurn{},
		activeByRuntime:      map[string]*assistantTurn{},
		activeByStored:       map[string]*assistantTurn{},
		recentByRuntime:      map[string]*assistantTurn{},
		runtimeSessions:      map[string]hermes.Session{},
		conversationState:    map[string]model.AssistantConversationState{},
		clarifications:       map[string]*model.AssistantClarification{},
		streams:              map[string]*assistantEventStream{},
	}
}

func newAssistantHTTPServer(repository *assistantHTTPRepository, gateway hermes.Gateway) *Server {
	runtime := bareAssistantRuntime(repository, gateway)
	return &Server{repository: repository, gateway: gateway, assistant: runtime, logger: discardLogger()}
}

func assistantHTTPAuth() authState {
	return authState{Session: model.Session{
		User:           model.User{ID: "user-1"},
		OrganizationID: "organization-1",
	}}
}

func TestAssistantStatusDistinguishesConfigurationConnectivityAndReadiness(t *testing.T) {
	t.Parallel()
	auth := assistantHTTPAuth()

	for _, test := range []struct {
		name    string
		gateway hermes.Gateway
		body    []string
	}{
		{name: "unconfigured", gateway: nil, body: []string{`"available":false`, "nie je nakonfigurovaný"}},
		{name: "disconnected", gateway: func() hermes.Gateway { gateway := hermes.NewFakeGateway(); gateway.Online = false; return gateway }(), body: []string{`"available":false`, "momentálne nie je dostupný"}},
		{name: "ready", gateway: hermes.NewFakeGateway(), body: []string{`"available":true`, `"ready":true`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &assistantHTTPRepository{}
			server := newAssistantHTTPServer(repository, test.gateway)
			recorder := httptest.NewRecorder()
			server.assistantStatus(recorder, httptest.NewRequest(http.MethodGet, "/", nil), auth)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			for _, expected := range test.body {
				if !strings.Contains(recorder.Body.String(), expected) {
					t.Fatalf("body %s does not contain %q", recorder.Body.String(), expected)
				}
			}
		})
	}
}

func TestCreateAssistantConversationDefaultsValidatesAndPersistsMode(t *testing.T) {
	t.Parallel()
	repository := &assistantHTTPRepository{conversation: model.AssistantConversation{ID: "conversation-1"}}
	server := newAssistantHTTPServer(repository, hermes.NewFakeGateway())
	auth := assistantHTTPAuth()

	invoke := func(body string, contentLength int64) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.ContentLength = contentLength
		recorder := httptest.NewRecorder()
		server.createAssistantConversation(recorder, request, auth)
		return recorder
	}

	if recorder := invoke("", 0); recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"primaryMode":"qa"`) || !strings.Contains(recorder.Body.String(), `"title":"Nový rozhovor"`) {
		t.Fatalf("default create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := invoke(`{"primaryMode":"edit"}`, -1); recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"primaryMode":"edit"`) {
		t.Fatalf("edit create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := invoke(`{`, -1); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder := invoke(`{"primaryMode":"other"}`, -1); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid mode status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.createErr = errors.New("create failed")
	if recorder := invoke("", 0); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAssistantConversationDetailAuthorizesAndProjectsEmptyHistory(t *testing.T) {
	t.Parallel()
	conversation := model.AssistantConversation{
		ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1",
		LastMode: model.AssistantQA, CreatedAt: time.Now(),
	}
	repository := &assistantHTTPRepository{conversation: conversation, receipts: map[string][]model.AssistantDraftReceipt{}}
	server := newAssistantHTTPServer(repository, nil)
	server.assistant.clarifications[conversation.ID] = &model.AssistantClarification{RequestID: "clarify-1", Choices: []string{"Áno"}}
	auth := assistantHTTPAuth()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("conversationID", conversation.ID)
	recorder := httptest.NewRecorder()
	server.assistantConversationDetail(recorder, request, auth)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"title":"Nový rozhovor"`) || !strings.Contains(recorder.Body.String(), `"requestId":"clarify-1"`) {
		t.Fatalf("detail status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	missing := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder = httptest.NewRecorder()
	server.assistantConversationDetail(recorder, missing, auth)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing conversation ID status=%d", recorder.Code)
	}
	repository.conversationErr = store.ErrNotFound
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("conversationID", conversation.ID)
	recorder = httptest.NewRecorder()
	server.assistantConversationDetail(recorder, request, auth)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unauthorized detail status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUpdateAssistantConversationValidatesStateAndReloadsStoredSettings(t *testing.T) {
	t.Parallel()
	conversation := model.AssistantConversation{
		ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1",
		PrimaryMode: model.AssistantQA, LastMode: model.AssistantQA,
	}
	repository := &assistantHTTPRepository{conversation: conversation}
	server := newAssistantHTTPServer(repository, hermes.NewFakeGateway())
	auth := assistantHTTPAuth()
	missingRequest := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"primaryMode":"edit"}`))
	missingRecorder := httptest.NewRecorder()
	server.updateAssistantConversation(missingRecorder, missingRequest, auth)
	if missingRecorder.Code != http.StatusBadRequest {
		t.Fatalf("missing update conversation status=%d", missingRecorder.Code)
	}

	invoke := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
		request.SetPathValue("conversationID", conversation.ID)
		recorder := httptest.NewRecorder()
		server.updateAssistantConversation(recorder, request, auth)
		return recorder
	}

	for _, test := range []struct {
		body   string
		status int
	}{
		{`{`, http.StatusBadRequest},
		{`{}`, http.StatusBadRequest},
		{`{"primaryMode":"other"}`, http.StatusUnprocessableEntity},
	} {
		if recorder := invoke(test.body); recorder.Code != test.status {
			t.Fatalf("update body=%q status=%d want=%d response=%s", test.body, recorder.Code, test.status, recorder.Body.String())
		}
	}

	for _, state := range []model.AssistantConversationState{model.AssistantStateRunning, model.AssistantStateAwaitingClarification} {
		server.assistant.conversationState[conversation.ID] = state
		if recorder := invoke(`{"primaryMode":"edit"}`); recorder.Code != http.StatusConflict {
			t.Fatalf("busy state=%s status=%d body=%s", state, recorder.Code, recorder.Body.String())
		}
	}
	delete(server.assistant.conversationState, conversation.ID)

	repository.setPrimaryErr = errors.New("set failed")
	if recorder := invoke(`{"primaryMode":"edit"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("set error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.setPrimaryErr = nil
	repository.conversationCalls = 0
	repository.reloadErr = errors.New("reload failed")
	if recorder := invoke(`{"primaryMode":"edit"}`); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reload error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	repository.reloadErr = nil
	repository.conversationCalls = 0
	repository.conversation.PrimaryMode = model.AssistantEdit
	if recorder := invoke(`{"primaryMode":"edit"}`); recorder.Code != http.StatusOK || repository.setPrimaryMode != model.AssistantEdit {
		t.Fatalf("update status=%d mode=%s body=%s", recorder.Code, repository.setPrimaryMode, recorder.Body.String())
	}
}

func TestHandleAssistantErrorMapsEveryPublicFailure(t *testing.T) {
	t.Parallel()
	server := &Server{logger: discardLogger()}
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{errAssistantTurnActive, http.StatusConflict, "assistant_busy"},
		{errAssistantTurnNotActive, http.StatusConflict, "assistant_idle"},
		{errAssistantClarification, http.StatusConflict, "clarification_mismatch"},
		{errAssistantCommandForbidden, http.StatusUnprocessableEntity, "management_command_forbidden"},
		{hermes.ErrUnavailable, http.StatusServiceUnavailable, "assistant_unavailable"},
		{hermes.ErrDisconnected, http.StatusServiceUnavailable, "assistant_unavailable"},
		{store.ErrNotFound, http.StatusNotFound, "not_found"},
		{errors.New("unexpected"), http.StatusServiceUnavailable, "assistant_unavailable"},
	} {
		recorder := httptest.NewRecorder()
		server.handleAssistantError(recorder, test.err)
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("error=%v status=%d body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

type interruptErrorGateway struct{ *hermes.FakeGateway }

func (g *interruptErrorGateway) Interrupt(context.Context, model.AssistantMode, string) error {
	return errors.New("interrupt failed")
}

type clarificationErrorGateway struct{ *hermes.FakeGateway }

func (g *clarificationErrorGateway) RespondClarification(context.Context, model.AssistantMode, string, string, string) error {
	return errors.New("clarification failed")
}

type blockingHistoryGateway struct {
	*hermes.FakeGateway
	entered chan struct{}
	release chan struct{}
}

func (g *blockingHistoryGateway) History(ctx context.Context, _ model.AssistantMode, _ string) ([]hermes.HistoryMessage, error) {
	g.entered <- struct{}{}
	select {
	case <-g.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestListAssistantConversationsHandlesRepositoryHistoryAndCancellation(t *testing.T) {
	auth := assistantHTTPAuth()
	repository := &assistantHTTPRepository{listErr: errors.New("list failed")}
	server := newAssistantHTTPServer(repository, hermes.NewFakeGateway())
	recorder := httptest.NewRecorder()
	server.listAssistantConversations(recorder, httptest.NewRequest(http.MethodGet, "/", nil), auth)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("list error status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	repository.listErr = nil
	repository.conversations = []model.AssistantConversation{{ID: "no-session", LastMode: model.AssistantQA}}
	server = newAssistantHTTPServer(repository, nil)
	recorder = httptest.NewRecorder()
	server.listAssistantConversations(recorder, httptest.NewRequest(http.MethodGet, "/", nil), auth)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"title":"Nový rozhovor"`) || strings.Contains(recorder.Body.String(), `"messages"`) {
		t.Fatalf("unavailable list status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	storedID := "stored-1"
	repository.conversations = []model.AssistantConversation{{ID: "bad-session", LastMode: model.AssistantQA, QASessionID: &storedID}}
	server = newAssistantHTTPServer(repository, hermes.NewFakeGateway())
	recorder = httptest.NewRecorder()
	server.listAssistantConversations(recorder, httptest.NewRequest(http.MethodGet, "/", nil), auth)
	if recorder.Code != http.StatusOK {
		t.Fatalf("history failure list status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	fake := hermes.NewFakeGateway()
	fake.Sessions[model.AssistantQA][storedID] = []hermes.HistoryMessage{{Role: "user", Text: "Moja otázka"}}
	repository.conversations = []model.AssistantConversation{{ID: "conversation-1", LastMode: model.AssistantQA, QASessionID: &storedID}}
	server = newAssistantHTTPServer(repository, fake)
	recorder = httptest.NewRecorder()
	server.listAssistantConversations(recorder, httptest.NewRequest(http.MethodGet, "/", nil), auth)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"title":"Moja otázka"`) {
		t.Fatalf("history list status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	blockingFake := hermes.NewFakeGateway()
	blockingFake.Sessions[model.AssistantQA][storedID] = []hermes.HistoryMessage{}
	blocking := &blockingHistoryGateway{FakeGateway: blockingFake, entered: make(chan struct{}, 4), release: make(chan struct{})}
	repository.conversations = make([]model.AssistantConversation, 5)
	for index := range repository.conversations {
		repository.conversations[index] = model.AssistantConversation{ID: "conversation-" + strconv.Itoa(index), LastMode: model.AssistantQA, QASessionID: &storedID}
	}
	server = newAssistantHTTPServer(repository, blocking)
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	recorder = httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.listAssistantConversations(recorder, request, auth)
		close(done)
	}()
	for index := 0; index < 4; index++ {
		select {
		case <-blocking.entered:
		case <-time.After(time.Second):
			t.Fatal("history workers did not reach the semaphore limit")
		}
	}
	cancel()
	close(blocking.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled conversation listing did not finish")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("canceled list status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAcquireAssistantHistorySlot(t *testing.T) {
	semaphore := make(chan struct{}, 1)
	if !acquireAssistantHistorySlot(context.Background(), semaphore) {
		t.Fatal("expected an available slot")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if acquireAssistantHistorySlot(ctx, semaphore) {
		t.Fatal("expected cancellation while the semaphore is full")
	}
}

type assistantStreamWriter struct {
	header   http.Header
	status   int
	body     strings.Builder
	writeErr error
	onWrite  func(string)
	flushes  int
}

func (w *assistantStreamWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *assistantStreamWriter) WriteHeader(status int) {
	w.status = status
}

func (w *assistantStreamWriter) Write(value []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	text := string(value)
	w.body.WriteString(text)
	if w.onWrite != nil {
		w.onWrite(text)
	}
	return len(value), nil
}

func (w *assistantStreamWriter) Flush() {
	w.flushes++
}

type nonFlushingAssistantWriter struct {
	header http.Header
	status int
	body   strings.Builder
}

func (w *nonFlushingAssistantWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *nonFlushingAssistantWriter) WriteHeader(status int) {
	w.status = status
}

func (w *nonFlushingAssistantWriter) Write(value []byte) (int, error) {
	return w.body.Write(value)
}

func TestStreamAssistantEventsCoversReplayLiveCloseAndTransportFailures(t *testing.T) {
	conversation := model.AssistantConversation{ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1"}
	repository := &assistantHTTPRepository{conversation: conversation}
	server := newAssistantHTTPServer(repository, hermes.NewFakeGateway())
	auth := assistantHTTPAuth()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	writer := &nonFlushingAssistantWriter{}
	server.streamAssistantEvents(writer, request, auth)
	if writer.status != http.StatusBadRequest {
		t.Fatalf("missing conversation status=%d body=%s", writer.status, writer.body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("conversationID", conversation.ID)
	writer = &nonFlushingAssistantWriter{}
	server.streamAssistantEvents(writer, request, auth)
	if writer.status != http.StatusInternalServerError || !strings.Contains(writer.body.String(), "streaming_unavailable") {
		t.Fatalf("non-flushing writer status=%d body=%s", writer.status, writer.body.String())
	}

	stream := server.assistant.stream(conversation.ID)
	stream.publish("message_delta", map[string]string{"delta": "Ahoj"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	request.SetPathValue("conversationID", conversation.ID)
	request.Header.Set("Last-Event-ID", "0")
	flushing := &assistantStreamWriter{}
	server.streamAssistantEvents(flushing, request, auth)
	if flushing.status != http.StatusOK || flushing.flushes < 2 || !strings.Contains(flushing.body.String(), "event: message_delta") || !strings.Contains(flushing.body.String(), `"delta":"Ahoj"`) {
		t.Fatalf("replay status=%d flushes=%d body=%s", flushing.status, flushing.flushes, flushing.body.String())
	}
	for name, want := range map[string]string{
		"Content-Type":      "text/event-stream; charset=utf-8",
		"Cache-Control":     "no-cache, no-transform",
		"X-Accel-Buffering": "no",
	} {
		if got := flushing.Header().Get(name); got != want {
			t.Fatalf("%s=%q want=%q", name, got, want)
		}
	}

	stream.publish("error", make(chan int))
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("conversationID", conversation.ID)
	request.Header.Set("Last-Event-ID", "1")
	flushing = &assistantStreamWriter{}
	server.streamAssistantEvents(flushing, request, auth)
	if strings.Contains(flushing.body.String(), "event: error") {
		t.Fatalf("unencodable replay event was written: %s", flushing.body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("conversationID", conversation.ID)
	request.Header.Set("Last-Event-ID", "0")
	flushing = &assistantStreamWriter{writeErr: errors.New("client disconnected")}
	server.streamAssistantEvents(flushing, request, auth)
	if flushing.status != http.StatusOK {
		t.Fatalf("write failure status=%d", flushing.status)
	}

	for _, test := range []struct {
		name  string
		close bool
	}{
		{name: "live event"},
		{name: "closed subscription", close: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestContext, stop := context.WithCancel(context.Background())
			defer stop()
			request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(requestContext)
			request.SetPathValue("conversationID", conversation.ID)
			request.Header.Set("Last-Event-ID", "invalid")
			writer := &assistantStreamWriter{}
			writer.onWrite = func(value string) {
				if strings.Contains(value, "event: completed") {
					stop()
				}
			}
			done := make(chan struct{})
			go func() {
				server.streamAssistantEvents(writer, request, auth)
				close(done)
			}()
			deadline := time.Now().Add(time.Second)
			for {
				stream.mu.Lock()
				count := len(stream.subscribers)
				if count > 0 && test.close {
					for subscriber := range stream.subscribers {
						delete(stream.subscribers, subscriber)
						close(subscriber)
					}
				}
				stream.mu.Unlock()
				if count > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("stream did not subscribe")
				}
				time.Sleep(time.Millisecond)
			}
			if !test.close {
				stream.publish("completed", map[string]string{"turnId": "turn-1"})
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("stream did not finish")
			}
			if !test.close && !strings.Contains(writer.body.String(), "event: completed") {
				t.Fatalf("live body=%s", writer.body.String())
			}
		})
	}
}

func TestStreamAssistantEventsKeepalive(t *testing.T) {
	previous := assistantKeepaliveInterval
	assistantKeepaliveInterval = time.Millisecond
	t.Cleanup(func() { assistantKeepaliveInterval = previous })

	conversation := model.AssistantConversation{ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1"}
	repository := &assistantHTTPRepository{conversation: conversation}
	server := newAssistantHTTPServer(repository, hermes.NewFakeGateway())
	auth := assistantHTTPAuth()

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	request.SetPathValue("conversationID", conversation.ID)
	writer := &assistantStreamWriter{onWrite: func(value string) {
		if strings.Contains(value, ": keepalive") {
			cancel()
		}
	}}
	server.streamAssistantEvents(writer, request, auth)
	if !strings.Contains(writer.body.String(), ": keepalive\n\n") {
		t.Fatalf("keepalive body=%q", writer.body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("conversationID", conversation.ID)
	writer = &assistantStreamWriter{writeErr: errors.New("client disconnected")}
	server.streamAssistantEvents(writer, request, auth)
}

func TestLoadAssistantHistoryMergesProfilesTitlesReceiptsAndFailures(t *testing.T) {
	t.Parallel()

	qaStored, editStored := "qa-stored", "edit-stored"
	conversation := model.AssistantConversation{
		ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1",
		LastMode: model.AssistantEdit, QASessionID: &qaStored, EditSessionID: &editStored,
		CreatedAt: time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC),
	}
	repository := &assistantHTTPRepository{receipts: map[string][]model.AssistantDraftReceipt{}}
	fake := hermes.NewFakeGateway()
	fake.Sessions[model.AssistantQA][qaStored] = []hermes.HistoryMessage{{Role: "user", Text: "  " + strings.Repeat("a", 90) + "  "}}
	fake.Sessions[model.AssistantEdit][editStored] = []hermes.HistoryMessage{{Role: "user", Text: "Edit question"}}
	server := newAssistantHTTPServer(repository, fake)
	if err := server.loadAssistantHistory(context.Background(), &conversation); err != nil {
		t.Fatal(err)
	}
	if len(conversation.Messages) != 2 || conversation.Title != "Edit question" {
		t.Fatalf("history messages=%+v title=%q", conversation.Messages, conversation.Title)
	}

	offline := hermes.NewFakeGateway()
	offline.Online = false
	server = newAssistantHTTPServer(repository, offline)
	failed := conversation
	failed.Messages = nil
	if err := server.loadAssistantHistory(context.Background(), &failed); !errors.Is(err, hermes.ErrUnavailable) {
		t.Fatalf("session load error=%v", err)
	}

	historyFailure := &historyErrorGateway{FakeGateway: fake, err: errors.New("history failed")}
	server = newAssistantHTTPServer(repository, historyFailure)
	failed = conversation
	failed.Messages = nil
	if err := server.loadAssistantHistory(context.Background(), &failed); err == nil || !strings.Contains(err.Error(), "history failed") {
		t.Fatalf("history load error=%v", err)
	}

	repository.receiptsErr = errors.New("receipts failed")
	server = newAssistantHTTPServer(repository, fake)
	failed = conversation
	failed.Messages = nil
	if err := server.loadAssistantHistory(context.Background(), &failed); err == nil || !strings.Contains(err.Error(), "receipts failed") {
		t.Fatalf("receipt load error=%v", err)
	}
}

func TestAssistantConversationDetailLogsNonAvailabilityIndependentHistoryFailure(t *testing.T) {
	t.Parallel()

	conversation := model.AssistantConversation{ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1", LastMode: model.AssistantQA}
	repository := &assistantHTTPRepository{conversation: conversation, receiptsErr: errors.New("receipts failed")}
	server := newAssistantHTTPServer(repository, hermes.NewFakeGateway())
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("conversationID", conversation.ID)
	recorder := httptest.NewRecorder()
	server.assistantConversationDetail(recorder, request, assistantHTTPAuth())
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"title":"Nový rozhovor"`) {
		t.Fatalf("detail status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAttachDraftReceiptsCoversAllIgnoredAndFallbackShapes(t *testing.T) {
	t.Parallel()

	now := time.Now()
	messages := []model.AssistantMessage{
		{ID: "system", Role: "system", CreatedAt: now},
		{ID: "assistant", Role: "assistant", CreatedAt: now},
		{ID: "turn-1-user", Role: "user", Mode: model.AssistantEdit, CreatedAt: now},
		{ID: "turn-1-assistant", Role: "assistant", Mode: model.AssistantEdit, Drafts: []model.AssistantDraftReceipt{{}}, CreatedAt: now},
	}
	receipts := map[string][]model.AssistantDraftReceipt{
		"empty":   {},
		"missing": {{RevisionID: "unattached", PageID: "page-x"}},
		"turn-1": {
			{},
			{RevisionID: "revision-2", PageID: "page-2"},
			{RevisionID: "revision-1", PageID: "page-1"},
		},
		"turn-2": {{RevisionID: "revision-3", PageID: "page-3"}},
	}
	messages = append(messages, model.AssistantMessage{ID: "turn-2-user", Role: "user", Mode: model.AssistantQA, CreatedAt: now})
	result := attachDraftReceipts(messages, receipts)
	if len(result) != 6 {
		t.Fatalf("attached messages=%d %+v", len(result), result)
	}
	var attached, fallback *model.AssistantMessage
	for index := range result {
		message := &result[index]
		if message.ID == "turn-1-assistant" {
			attached = message
		}
		if message.ID == "turn-2-assistant-receipt" {
			fallback = message
		}
	}
	if attached == nil || len(attached.Drafts) != 3 || attached.Drafts[1].RevisionID != "revision-1" || attached.Drafts[2].RevisionID != "revision-2" {
		t.Fatalf("attached receipt=%+v", attached)
	}
	if fallback == nil || fallback.Mode != model.AssistantQA || len(fallback.Drafts) != 1 {
		t.Fatalf("fallback receipt=%+v", fallback)
	}
}

func TestSubmitAssistantMessageValidatesBodyModeAndCommands(t *testing.T) {
	t.Parallel()
	conversation := model.AssistantConversation{
		ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1",
		PrimaryMode: model.AssistantQA, LastMode: model.AssistantQA,
	}
	repository := &assistantHTTPRepository{conversation: conversation}
	server := newAssistantHTTPServer(repository, nil)
	auth := assistantHTTPAuth()

	invoke := func(body string, includeID bool) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		if includeID {
			request.SetPathValue("conversationID", conversation.ID)
		}
		recorder := httptest.NewRecorder()
		server.submitAssistantMessage(recorder, request, auth)
		return recorder
	}

	for _, test := range []struct {
		body      string
		includeID bool
		status    int
	}{
		{`{"content":"question","mode":"qa"}`, false, http.StatusBadRequest},
		{`{`, true, http.StatusBadRequest},
		{`{"content":"   ","mode":"qa"}`, true, http.StatusUnprocessableEntity},
		{`{"content":"` + strings.Repeat("x", 12001) + `","mode":"qa"}`, true, http.StatusUnprocessableEntity},
		{`{"content":"question","mode":"other"}`, true, http.StatusUnprocessableEntity},
		{`{"content":"/model unsafe","mode":"qa"}`, true, http.StatusUnprocessableEntity},
		{`{"content":"question","mode":"qa"}`, true, http.StatusServiceUnavailable},
	} {
		if recorder := invoke(test.body, test.includeID); recorder.Code != test.status {
			t.Fatalf("submit body length=%d includeID=%t status=%d want=%d response=%s", len(test.body), test.includeID, recorder.Code, test.status, recorder.Body.String())
		}
	}
}

func TestStopAssistantTurnHandlesIdleSuccessAndGatewayFailure(t *testing.T) {
	t.Parallel()
	conversation := model.AssistantConversation{ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1"}
	repository := &assistantHTTPRepository{conversation: conversation}
	auth := assistantHTTPAuth()

	invoke := func(server *Server, includeID bool) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		if includeID {
			request.SetPathValue("conversationID", conversation.ID)
		}
		recorder := httptest.NewRecorder()
		server.stopAssistantTurn(recorder, request, auth)
		return recorder
	}

	gateway := hermes.NewFakeGateway()
	server := newAssistantHTTPServer(repository, gateway)
	if recorder := invoke(server, false); recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing conversation status=%d", recorder.Code)
	}
	if recorder := invoke(server, true); recorder.Code != http.StatusConflict {
		t.Fatalf("idle stop status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	turn := &assistantTurn{ID: "turn-1", ConversationID: conversation.ID, Mode: model.AssistantQA, RuntimeID: "runtime-1"}
	server.assistant.activeByConversation[conversation.ID] = turn
	if recorder := invoke(server, true); recorder.Code != http.StatusAccepted {
		t.Fatalf("stop status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	failingGateway := &interruptErrorGateway{FakeGateway: hermes.NewFakeGateway()}
	server = newAssistantHTTPServer(repository, failingGateway)
	server.assistant.activeByConversation[conversation.ID] = turn
	if recorder := invoke(server, true); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed stop status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestClarificationResponseValidatesRequestAndResumesTurn(t *testing.T) {
	t.Parallel()
	conversation := model.AssistantConversation{ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1"}
	repository := &assistantHTTPRepository{conversation: conversation}
	auth := assistantHTTPAuth()

	invoke := func(server *Server, body, conversationID, requestID string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.SetPathValue("conversationID", conversationID)
		request.SetPathValue("requestID", requestID)
		recorder := httptest.NewRecorder()
		server.respondAssistantClarification(recorder, request, auth)
		return recorder
	}

	server := newAssistantHTTPServer(repository, hermes.NewFakeGateway())
	for _, test := range []struct {
		body, conversationID, requestID string
		status                          int
	}{
		{`{"answer":"yes"}`, "", "request-1", http.StatusBadRequest},
		{`{"answer":"yes"}`, conversation.ID, "", http.StatusBadRequest},
		{`{`, conversation.ID, "request-1", http.StatusBadRequest},
		{`{"answer":"   "}`, conversation.ID, "request-1", http.StatusUnprocessableEntity},
		{`{"answer":"` + strings.Repeat("x", 12001) + `"}`, conversation.ID, "request-1", http.StatusUnprocessableEntity},
		{`{"answer":"yes"}`, conversation.ID, "request-1", http.StatusConflict},
	} {
		if recorder := invoke(server, test.body, test.conversationID, test.requestID); recorder.Code != test.status {
			t.Fatalf("clarification body length=%d ids=%q/%q status=%d want=%d response=%s", len(test.body), test.conversationID, test.requestID, recorder.Code, test.status, recorder.Body.String())
		}
	}

	turn := &assistantTurn{ID: "turn-1", ConversationID: conversation.ID, Mode: model.AssistantEdit, RuntimeID: "runtime-1"}
	server.assistant.activeByConversation[conversation.ID] = turn
	server.assistant.clarifications[conversation.ID] = &model.AssistantClarification{RequestID: "request-1", Mode: model.AssistantEdit}
	if recorder := invoke(server, `{"answer":"  yes  "}`, conversation.ID, "request-1"); recorder.Code != http.StatusAccepted {
		t.Fatalf("clarification success status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	failingGateway := &clarificationErrorGateway{FakeGateway: hermes.NewFakeGateway()}
	server = newAssistantHTTPServer(repository, failingGateway)
	server.assistant.activeByConversation[conversation.ID] = turn
	server.assistant.clarifications[conversation.ID] = &model.AssistantClarification{RequestID: "request-1", Mode: model.AssistantEdit}
	if recorder := invoke(server, `{"answer":"yes"}`, conversation.ID, "request-1"); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("clarification failure status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
