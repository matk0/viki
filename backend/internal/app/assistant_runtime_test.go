package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"viki/internal/hermes"
	"viki/internal/model"
	"viki/internal/store"
)

type runtimeTestRepository struct {
	store.Repository
	mu            sync.Mutex
	storedIDs     []string
	setSessionErr error
	receipts      map[string][]model.AssistantDraftReceipt
	receiptsErr   error
	cursorUpdates []int
	cursorErr     error
	modeUpdates   []model.AssistantMode
	modeErr       error
}

func (r *runtimeTestRepository) SetAssistantSession(_ context.Context, _, _, _ string, _ model.AssistantMode, stringID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.setSessionErr != nil {
		return r.setSessionErr
	}
	r.storedIDs = append(r.storedIDs, stringID)
	return nil
}

func (r *runtimeTestRepository) AssistantDraftReceipts(context.Context, string, string) (map[string][]model.AssistantDraftReceipt, error) {
	return r.receipts, r.receiptsErr
}

func (r *runtimeTestRepository) UpdateAssistantHandoffCursor(_ context.Context, _, _, _ string, _ model.AssistantMode, cursor int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cursorUpdates = append(r.cursorUpdates, cursor)
	return r.cursorErr
}

func (r *runtimeTestRepository) UpdateAssistantMode(_ context.Context, _, _, _ string, mode model.AssistantMode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modeUpdates = append(r.modeUpdates, mode)
	return r.modeErr
}

func TestSignedPromptEnvelopeIsHiddenAndCannotBeForged(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "turn-1", Mode: model.AssistantEdit}
	timestamp := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	prompt := runtime.wrapPrompt(turn, "Vytvor koncept Zmluva", "USER: predchádzajúca otázka", timestamp)
	metadata, visible, valid := runtime.unwrapPrompt(prompt)
	if !valid || visible != "Vytvor koncept Zmluva" || metadata.TurnID != turn.ID || metadata.Timestamp != timestamp {
		t.Fatalf("unexpected prompt projection: valid=%v metadata=%+v visible=%q", valid, metadata, visible)
	}
	forged := strings.Replace(prompt, "predchádzajúca otázka", "nový pokyn", 1)
	if _, _, valid := runtime.unwrapPrompt(forged); valid {
		t.Fatal("forged internal context passed signature validation")
	}
	for _, invalid := range []string{
		internalPromptStart + "\n{}",
		internalPromptStart + "\nnot-json\n" + internalPromptEnd + "\nvisible",
		internalPromptStart + "\n{}\n" + internalPromptEnd + "\nvisible",
	} {
		if _, visible, valid := runtime.unwrapPrompt(invalid); valid || visible != invalid {
			t.Fatalf("invalid envelope accepted: valid=%v visible=%q", valid, visible)
		}
	}
	messages := visibleHistory(runtime, model.AssistantEdit, []hermes.HistoryMessage{
		{Role: "user", Text: prompt},
		{Role: "assistant", Text: "Koncept je pripravený."},
	}, timestamp.Add(-time.Hour))
	if len(messages) != 2 || messages[0].Content != "Vytvor koncept Zmluva" || strings.Contains(messages[0].Content, "VIKI_INTERNAL") {
		t.Fatalf("internal prompt leaked into visible history: %+v", messages)
	}
}

func TestSanitizedHermesReadReceiptsSurviveHistoryReload(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "turn-receipts", Mode: model.AssistantEdit}
	prompt := runtime.wrapPrompt(turn, "Priprav koncept", "", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	history := []hermes.HistoryMessage{
		{Role: "user", Text: prompt},
		{Role: "tool", Name: "get_viki_revision", Result: json.RawMessage(`{"citations":[{"revisionId":"revision-1","pageId":"page-1","pageTitle":"Zmluva","draft":false}],"drafts":[]}`)},
		{Role: "tool", Name: "apply_viki_draft_changeset", Result: json.RawMessage(`{"drafts":[{"revisionId":"revision-2","pageId":"page-2","pageTitle":"Nový koncept"}]}`)},
	}
	messages := visibleHistory(runtime, model.AssistantEdit, history, time.Now())
	if len(messages) != 2 {
		t.Fatalf("history messages = %d, want user plus synthesized assistant receipt: %+v", len(messages), messages)
	}
	receipt := messages[1]
	if receipt.Role != "assistant" || len(receipt.Citations) != 1 || receipt.Citations[0].RevisionID != "revision-1" || len(receipt.Drafts) != 1 || receipt.Drafts[0].RevisionID != "revision-2" {
		t.Fatalf("sanitized receipts were not reconstructed: %+v", receipt)
	}
}

func TestHandoffIsBoundedToTenRecentExchangesAndFourKilobytes(t *testing.T) {
	t.Parallel()

	messages := make([]model.AssistantMessage, 30)
	for index := range messages {
		messages[index] = model.AssistantMessage{Role: "user", Content: strings.Repeat("ž", 180) + " marker-" + string(rune('a'+index))}
	}
	handoff := formatHandoff(messages)
	if len(handoff) > maxHandoffBytes {
		t.Fatalf("handoff bytes = %d, want <= %d", len(handoff), maxHandoffBytes)
	}
	if strings.Contains(handoff, "marker-a") || !strings.Contains(handoff, "marker-~") {
		t.Fatalf("handoff did not retain only recent messages: %q", handoff)
	}
	readReceipt := formatHandoff([]model.AssistantMessage{{
		Role: "assistant", Content: "Podklady som overil.",
		Citations: []model.Citation{{RevisionID: "revision-1", PageID: "page-1", PageTitle: "Zmluva", Draft: true}},
		Drafts:    []model.AssistantDraftReceipt{{RevisionID: "revision-2", PageID: "page-2", PageTitle: "Dodatok"}},
	}})
	if !strings.Contains(readReceipt, "VIKI_RECEIPT: revisionId=revision-1 pageId=page-1 title=Zmluva draft=true") {
		t.Fatalf("handoff omitted exact read receipt: %q", readReceipt)
	}
	if !strings.Contains(readReceipt, "DRAFT_RECEIPT: revisionId=revision-2 pageId=page-2 title=Dodatok") {
		t.Fatalf("handoff omitted exact draft receipt: %q", readReceipt)
	}
	secretHandoff := formatHandoff([]model.AssistantMessage{{Role: "user", Content: "OPENAI_API_KEY=sk-1234567890 Bearer abcdef password:supersecret"}}) // gitleaks:allow -- synthetic sanitizer fixture
	for _, secret := range []string{"sk-1234567890", "abcdef", "supersecret"} {
		if strings.Contains(secretHandoff, secret) {
			t.Fatalf("handoff leaked %q: %q", secret, secretHandoff)
		}
	}
}

func TestDraftReceiptFallbackDoesNotDuplicateHistoryReceipts(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	messages := []model.AssistantMessage{
		{ID: "turn-1-user", Role: "user", Mode: model.AssistantEdit, Content: "Vytvor koncept", CreatedAt: createdAt},
		{ID: "turn-1-assistant-receipt", Role: "assistant", Mode: model.AssistantEdit, Content: "Hotovo.", Drafts: []model.AssistantDraftReceipt{{RevisionID: "revision-1", PageID: "page-1", PageTitle: "Zmluva"}}, CreatedAt: createdAt.Add(time.Nanosecond)},
	}
	receipts := map[string][]model.AssistantDraftReceipt{
		"turn-1": {
			{RevisionID: "revision-1", PageID: "page-1", PageTitle: "Zmluva"},
			{RevisionID: "revision-2", PageID: "page-2", PageTitle: "Dodatok"},
		},
	}

	messages = attachDraftReceipts(messages, receipts)
	counts := map[string]int{}
	for _, message := range messages {
		for _, draft := range message.Drafts {
			counts[draft.RevisionID]++
		}
	}
	if counts["revision-1"] != 1 || counts["revision-2"] != 1 {
		t.Fatalf("receipt counts = %#v, want each receipt once; messages=%+v", counts, messages)
	}
}

func TestReconnectInvalidatesCachedRuntimeIDsAndUnnamedProgressIsIgnored(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	runtime.runtimeSessions[assistantSessionKey(model.AssistantQA, "conversation")] = hermes.Session{RuntimeID: "stale", StoredID: "stored"}
	runtime.runtimeSessions[assistantSessionKey(model.AssistantEdit, "other")] = hermes.Session{RuntimeID: "edit-live", StoredID: "edit-stored"}
	runtime.reconcileProfile(model.AssistantQA)
	if _, exists := runtime.runtimeSessions[assistantSessionKey(model.AssistantQA, "conversation")]; exists {
		t.Fatal("gateway reconnect retained a stale Q&A runtime identifier")
	}
	if _, exists := runtime.runtimeSessions[assistantSessionKey(model.AssistantEdit, "other")]; !exists {
		t.Fatal("Q&A reconnect invalidated the independent Edit profile")
	}

	turn := &assistantTurn{ID: "turn", ConversationID: "conversation", Mode: model.AssistantQA, RuntimeID: "runtime", StoredID: "stored"}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)] = turn
	runtime.activeByStored[assistantSessionKey(turn.Mode, turn.StoredID)] = turn
	runtime.handleGatewayEvent(model.AssistantQA, hermes.Event{
		Type: "tool.progress", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"preview":"working"}`),
	})
	if _, active := runtime.activeGrant(turn.Mode, turn.StoredID); !active {
		t.Fatal("an unnamed progress event incorrectly revoked the active turn")
	}
	runtime.handleGatewayEvent(model.AssistantQA, hermes.Event{
		Type: "tool.start", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"name":"terminal"}`),
	})
	if _, active := runtime.activeGrant(turn.Mode, turn.StoredID); active {
		t.Fatal("a prohibited tool did not fail closed and revoke the turn grant")
	}
}

func TestClarifyToolEventsPreserveTheTurnUntilTheUserResponds(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "turn", ConversationID: "conversation", Mode: model.AssistantEdit, RuntimeID: "runtime", StoredID: "stored"}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)] = turn
	runtime.activeByStored[assistantSessionKey(turn.Mode, turn.StoredID)] = turn

	runtime.handleGatewayEvent(turn.Mode, hermes.Event{
		Type: "tool.start", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"name":"clarify"}`),
	})
	if _, active := runtime.activeGrant(turn.Mode, turn.StoredID); !active {
		t.Fatal("Hermes clarify tool start incorrectly revoked the active turn")
	}

	runtime.handleGatewayEvent(turn.Mode, hermes.Event{
		Type: "clarify.request", SessionID: turn.RuntimeID,
		Payload: json.RawMessage(`{"question":"Je televizia sucastou balika?","request_id":"clarify-1"}`),
	})
	clarification := runtime.clarification(turn.ConversationID)
	if clarification == nil || clarification.RequestID != "clarify-1" || clarification.Message != "Je televizia sucastou balika?" {
		t.Fatalf("clarification = %+v", clarification)
	}

	runtime.handleGatewayEvent(turn.Mode, hermes.Event{
		Type: "tool.complete", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"name":"clarify","result":{}}`),
	})
	if _, active := runtime.activeGrant(turn.Mode, turn.StoredID); !active {
		t.Fatal("Hermes clarify tool completion incorrectly revoked the active turn")
	}
}

func TestCompressionRotationUpdatesDurableBindingAfterTurnCompletion(t *testing.T) {
	t.Parallel()

	repository := &runtimeTestRepository{}
	runtime := newAssistantRuntime(context.Background(), repository, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{
		ID: "turn", ConversationID: "00000000-0000-4000-8000-000000000099",
		OrganizationID: "00000000-0000-4000-8000-000000000001", UserID: "00000000-0000-4000-8000-000000000011",
		Mode: model.AssistantEdit, RuntimeID: "runtime", StoredID: "old-stored",
	}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)] = turn
	runtime.activeByStored[assistantSessionKey(turn.Mode, turn.StoredID)] = turn
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{
		Type: "message.complete", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"status":"complete"}`),
	})
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{
		Type: "session.info", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"stored_session_id":"rotated-stored"}`),
	})
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if len(repository.storedIDs) != 1 || repository.storedIDs[0] != "rotated-stored" {
		t.Fatalf("durable rotation updates = %#v", repository.storedIDs)
	}
	if _, active := runtime.activeGrant(turn.Mode, "rotated-stored"); active {
		t.Fatal("post-completion rotation reactivated a revoked tool grant")
	}
}

func TestInternalMarkerFilterWorksAcrossStreamingDeltaBoundaries(t *testing.T) {
	t.Parallel()

	filter := internalStreamFilter{}
	parts := []string{
		"Pred ", "<<", "<VIKI_INTERNAL_CONTEXT_V1>>>\nsecret", " context\n<<<END_VIKI_", "INTERNAL_CONTEXT_V1>>> Po",
	}
	var visible strings.Builder
	for _, part := range parts {
		visible.WriteString(filter.Feed(part))
	}
	visible.WriteString(filter.Flush())
	if visible.String() != "Pred  Po" {
		t.Fatalf("stream filter output = %q", visible.String())
	}
}

func TestAlreadyStreamedInterimMessageIsNotPublishedAgain(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "turn", ConversationID: "conversation", Mode: model.AssistantQA, RuntimeID: "runtime", StoredID: "stored"}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)] = turn
	runtime.activeByStored[assistantSessionKey(turn.Mode, turn.StoredID)] = turn
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{
		Type: "message.interim", SessionID: turn.RuntimeID,
		Payload: json.RawMessage(`{"text":"už streamované","already_streamed":true}`),
	})
	if turn.HadDelta {
		t.Fatal("already-streamed interim text was published as a new delta")
	}
	if replay, _, stop := runtime.stream(turn.ConversationID).subscribe(0, true); len(replay) != 0 {
		stop()
		t.Fatalf("already-streamed interim text emitted events: %+v", replay)
	} else {
		stop()
	}
}

func TestCompletedDraftToolEmitsEachSafeDraftReceiptOnce(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "00000000-0000-4000-8000-000000000041", ConversationID: "conversation", Mode: model.AssistantEdit, RuntimeID: "runtime", StoredID: "stored", Citations: map[string]model.Citation{}}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)] = turn
	event := hermes.Event{
		Type:      "tool.complete",
		SessionID: turn.RuntimeID,
		Payload:   json.RawMessage(`{"name":"apply_viki_draft_changeset","result":{"drafts":[{"revisionId":"revision-2","pageId":"page-2","pageTitle":"Dodatok"},{"revisionId":"revision-1","pageId":"page-1","pageTitle":"Zmluva"},{"revisionId":"revision-1","pageId":"page-1","pageTitle":"must not replace"},{"revisionId":"","pageId":"unsafe","pageTitle":"ignored"}]}}`),
	}
	runtime.handleGatewayEvent(turn.Mode, event)
	runtime.handleGatewayEvent(turn.Mode, event)

	replay, _, unsubscribe := runtime.stream(turn.ConversationID).subscribe(0, true)
	defer unsubscribe()
	if len(replay) != 4 || replay[0].Type != "activity" || replay[1].Type != "draft_created" || replay[2].Type != "draft_created" || replay[3].Type != "activity" {
		t.Fatalf("draft tool events = %+v, want activity, two unique drafts, then duplicate activity only", replay)
	}
	if turn.Drafts == nil || len(turn.Drafts) != 2 || turn.Drafts["revision-1"].PageTitle != "Zmluva" || turn.Drafts["revision-2"].PageTitle != "Dodatok" {
		t.Fatalf("turn draft receipts = %+v", turn.Drafts)
	}
}

func TestFreshSSESubscriptionStartsAtTailAndReconnectReplaysFromID(t *testing.T) {
	t.Parallel()

	stream := &assistantEventStream{subscribers: map[chan assistantPublicEvent]struct{}{}}
	stream.publish("message_delta", map[string]string{"delta": "old"})
	replay, live, unsubscribe := stream.subscribe(0, false)
	defer unsubscribe()
	if len(replay) != 0 {
		t.Fatalf("fresh subscription replayed %d historical events", len(replay))
	}
	stream.publish("completed", map[string]string{"turnId": "turn"})
	select {
	case event := <-live:
		if event.ID != 2 || event.Type != "completed" {
			t.Fatalf("unexpected live event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("fresh subscription did not receive a new event")
	}
	replayed, _, stop := stream.subscribe(1, true)
	defer stop()
	if len(replayed) != 1 || replayed[0].ID != 2 {
		t.Fatalf("reconnect replay = %+v, want event 2", replayed)
	}
}

type failingResumeGateway struct{ *hermes.FakeGateway }

func (g *failingResumeGateway) ResumeSession(context.Context, model.AssistantMode, string) (hermes.Session, error) {
	return hermes.Session{}, errors.New("resume failed")
}

type rotatingResumeGateway struct{ *hermes.FakeGateway }

func (g *rotatingResumeGateway) ResumeSession(context.Context, model.AssistantMode, string) (hermes.Session, error) {
	return hermes.Session{RuntimeID: "new-runtime", StoredID: "new-stored"}, nil
}

func (g *rotatingResumeGateway) SessionStatus(context.Context, model.AssistantMode, string) (hermes.SessionState, error) {
	return hermes.SessionState{Running: true, Status: "streaming"}, nil
}

func TestReconnectRotationUpdatesDurableBindingAndActiveGrant(t *testing.T) {
	t.Parallel()

	repository := &runtimeTestRepository{}
	gateway := &rotatingResumeGateway{FakeGateway: hermes.NewFakeGateway()}
	runtime := newAssistantRuntime(context.Background(), repository, gateway, "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{
		ID: "turn", ConversationID: "00000000-0000-4000-8000-000000000099",
		OrganizationID: "00000000-0000-4000-8000-000000000001", UserID: "00000000-0000-4000-8000-000000000011",
		Mode: model.AssistantQA, RuntimeID: "old-runtime", StoredID: "old-stored",
	}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)] = turn
	runtime.activeByStored[assistantSessionKey(turn.Mode, turn.StoredID)] = turn

	runtime.reconcileProfile(turn.Mode)

	repository.mu.Lock()
	storedIDs := append([]string(nil), repository.storedIDs...)
	repository.mu.Unlock()
	if len(storedIDs) != 1 || storedIDs[0] != "new-stored" {
		t.Fatalf("durable rotation updates = %#v", storedIDs)
	}
	if _, active := runtime.activeGrant(turn.Mode, "old-stored"); active {
		t.Fatal("old durable session retained its tool grant")
	}
	if _, active := runtime.activeGrant(turn.Mode, "new-stored"); !active {
		t.Fatal("rotated durable session did not receive the active tool grant")
	}
	if _, active := runtime.activeGrant(turn.Mode, "new-runtime"); active {
		t.Fatal("ephemeral runtime session incorrectly received a native-tool grant")
	}
}

func TestReconnectFailureClearsActiveTurnInsteadOfLeavingItStuck(t *testing.T) {
	t.Parallel()

	gateway := &failingResumeGateway{FakeGateway: hermes.NewFakeGateway()}
	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, gateway, "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "turn", ConversationID: "conversation", Mode: model.AssistantQA, RuntimeID: "runtime", StoredID: "stored"}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)] = turn
	runtime.activeByStored[assistantSessionKey(turn.Mode, turn.StoredID)] = turn
	runtime.reconcileProfile(turn.Mode)
	if _, active := runtime.activeGrant(turn.Mode, turn.StoredID); active {
		t.Fatal("failed reconnect left the turn grant active")
	}
	if state := runtime.conversationState[turn.ConversationID]; state != model.AssistantStateError {
		t.Fatalf("conversation state = %q, want error", state)
	}
}

func TestAssistantProjectionHelpersCoverBoundaryAndNestedShapes(t *testing.T) {
	t.Parallel()

	if assistantMessageTurnID("assistant") != "" || assistantMessageTurnID("-assistant") != "" || assistantMessageTurnID("turn-1-assistant-2") != "turn-1" {
		t.Fatal("assistant message turn ID boundary handling failed")
	}
	if truncateRunes("krátke", 20) != "krátke" || truncateRunes("žluťoučký", 4) != "žluť…" {
		t.Fatal("rune truncation failed")
	}

	if got := stripInternalMarkers(" čistý text "); got != "čistý text" {
		t.Fatalf("plain marker stripping = %q", got)
	}
	wrapped := "pred " + internalPromptStart + "secret" + internalPromptEnd + " po " + internalPromptStart + "secret2" + internalPromptEnd
	if got := stripInternalMarkers(wrapped); got != "pred  po" {
		t.Fatalf("marker stripping = %q", got)
	}
	incomplete := "pred " + internalPromptStart + "secret"
	if got := stripInternalMarkers(incomplete); got != incomplete {
		t.Fatalf("incomplete marker stripping = %q", got)
	}

	plainFilter := internalStreamFilter{buffer: "tail"}
	if got := plainFilter.Flush(); got != "tail" || plainFilter.buffer != "" {
		t.Fatalf("plain filter flush = %q buffer=%q", got, plainFilter.buffer)
	}
	insideFilter := internalStreamFilter{buffer: "secret", inside: true}
	if got := insideFilter.Flush(); got != "" || insideFilter.buffer != "" {
		t.Fatalf("internal filter flush = %q buffer=%q", got, insideFilter.buffer)
	}

	for _, test := range []struct {
		value string
		limit int
		want  string
	}{
		{"abc", 0, "abc"},
		{"abc", 3, "abc"},
		{"žltá", 3, "žl"},
	} {
		if got := truncateUTF8(test.value, test.limit); got != test.want {
			t.Fatalf("truncateUTF8(%q, %d)=%q want=%q", test.value, test.limit, got, test.want)
		}
	}

	qaID, editID := "qa", "edit"
	conversation := model.AssistantConversation{QASessionID: &qaID, EditSessionID: &editID}
	if conversationSessionID(conversation, model.AssistantQA) != conversation.QASessionID || conversationSessionID(conversation, model.AssistantEdit) != conversation.EditSessionID {
		t.Fatal("conversation session selection failed")
	}

	for name, want := range map[string]string{
		"search_viki":                "Hľadám vo viki…",
		"get_viki_page":              "Čítam podklady vo viki…",
		"get_viki_revision":          "Čítam podklady vo viki…",
		"apply_viki_draft_changeset": "Vytváram drafty vo viki…",
		"unexpected":                 "Pracujem s viki…",
	} {
		if got := toolActivityLabel(name); got != want {
			t.Fatalf("tool label %q=%q want=%q", name, got, want)
		}
	}
	for _, test := range []struct {
		name, eventType, want string
	}{
		{name: "search_viki", eventType: "tool.start", want: "searching"},
		{name: "search_viki", eventType: "tool.complete", want: "searched"},
		{name: "get_viki_page", eventType: "tool.progress", want: "reading"},
		{name: "get_viki_revision", eventType: "tool.complete", want: "read"},
		{name: "apply_viki_draft_changeset", eventType: "tool.start", want: "drafting"},
		{name: "apply_viki_draft_changeset", eventType: "tool.complete", want: "drafted"},
		{name: "unexpected", eventType: "tool.start", want: "working"},
	} {
		if got := toolActivityState(test.name, test.eventType); got != test.want {
			t.Fatalf("tool state %q/%q=%q want=%q", test.name, test.eventType, got, test.want)
		}
	}

	if got := unwrapToolResult(nil); got != nil {
		t.Fatalf("nil tool result = %q", got)
	}
	plain := json.RawMessage(`{"value":1}`)
	if got := unwrapToolResult(plain); string(got) != string(plain) {
		t.Fatalf("plain tool result = %s", got)
	}
	encoded := json.RawMessage(`"{\"result\":{\"value\":2}}"`)
	if got := unwrapToolResult(encoded); string(got) != `{"value":2}` {
		t.Fatalf("encoded tool result = %s", got)
	}

	if citations := extractCitations(json.RawMessage(`not-json`)); citations != nil {
		t.Fatalf("invalid citations = %+v", citations)
	}
	citations := extractCitations(json.RawMessage(`{
		"results":[
			{"revisionId":"revision-2","pageId":"page-2","pageTitle":"Page 2","draft":true},
			{"id":"revision-1","pageId":"page-1","title":"Page 1","status":"draft"},
			{"revisionId":"","pageId":"page-x"},
			{"revisionId":"revision-no-page"}
		],
		"duplicate":{"revisionId":"revision-2","pageId":"page-2","title":"Updated"}
	}`))
	if len(citations) != 2 || citations[0].RevisionID != "revision-1" || !citations[0].Draft || citations[1].RevisionID != "revision-2" {
		t.Fatalf("nested citations = %+v", citations)
	}

	if drafts := extractDraftReceipts(json.RawMessage(`not-json`)); drafts != nil {
		t.Fatalf("invalid drafts = %+v", drafts)
	}
	drafts := extractDraftReceipts(json.RawMessage(`{"drafts":[{"revisionId":"revision-2","pageId":"page-2","pageTitle":"Dodatok"},{"revisionId":"revision-1","pageId":"page-1","pageTitle":"Zmluva"},{"revisionId":"revision-2","pageId":"page-2","pageTitle":"Duplicate"},{"revisionId":"","pageId":"page-x"},{"revisionId":"revision-x","pageId":""}]}`))
	if len(drafts) != 2 || drafts[0].RevisionID != "revision-1" || drafts[1].RevisionID != "revision-2" || drafts[1].PageTitle != "Dodatok" {
		t.Fatalf("safe drafts = %+v", drafts)
	}
}

func TestAssistantSortingAndReplayOverflowAreDeterministic(t *testing.T) {
	t.Parallel()

	citations := sortedCitations(map[string]model.Citation{
		"revision-2": {RevisionID: "revision-2"},
		"revision-1": {RevisionID: "revision-1"},
	})
	if citations[0].RevisionID != "revision-1" || citations[1].RevisionID != "revision-2" {
		t.Fatalf("citations = %+v", citations)
	}
	drafts := sortedDraftReceipts(map[string]model.AssistantDraftReceipt{
		"revision-2": {RevisionID: "revision-2"},
		"revision-1": {RevisionID: "revision-1"},
	})
	if drafts[0].RevisionID != "revision-1" || drafts[1].RevisionID != "revision-2" {
		t.Fatalf("drafts = %+v", drafts)
	}

	now := time.Now()
	merged := mergeAssistantHistory(
		[]model.AssistantMessage{{ID: "b", CreatedAt: now}, {ID: "later", CreatedAt: now.Add(time.Second)}},
		[]model.AssistantMessage{{ID: "a", CreatedAt: now}},
	)
	if merged[0].ID != "a" || merged[1].ID != "b" || merged[2].ID != "later" {
		t.Fatalf("merged history = %+v", merged)
	}

	stream := &assistantEventStream{subscribers: map[chan assistantPublicEvent]struct{}{}}
	blocked := make(chan assistantPublicEvent, 1)
	blocked <- assistantPublicEvent{}
	stream.subscribers[blocked] = struct{}{}
	for index := 0; index < maxReplayEvents+2; index++ {
		stream.publish("activity", index)
	}
	if len(stream.replay) != maxReplayEvents || stream.replay[0].ID != 3 {
		t.Fatalf("replay len=%d first=%d", len(stream.replay), stream.replay[0].ID)
	}
	if _, exists := stream.subscribers[blocked]; exists {
		t.Fatal("blocked subscriber was retained")
	}
	if _, open := <-blocked; !open {
		t.Fatal("buffered event disappeared when subscriber closed")
	}
	if _, open := <-blocked; open {
		t.Fatal("overflowed subscriber channel remained open")
	}
}

type historyErrorGateway struct {
	*hermes.FakeGateway
	err error
}

func (g *historyErrorGateway) History(context.Context, model.AssistantMode, string) ([]hermes.HistoryMessage, error) {
	return nil, g.err
}

type submitErrorGateway struct {
	*hermes.FakeGateway
	err error
}

func (g *submitErrorGateway) Submit(context.Context, model.AssistantMode, string, string) error {
	return g.err
}

type expiredResumeGateway struct{ *hermes.FakeGateway }

func (g *expiredResumeGateway) ResumeSession(context.Context, model.AssistantMode, string) (hermes.Session, error) {
	return hermes.Session{}, &hermes.RPCError{Code: 4007, Message: "expired"}
}

type statusErrorGateway struct {
	*rotatingResumeGateway
	err error
}

func (g *statusErrorGateway) SessionStatus(context.Context, model.AssistantMode, string) (hermes.SessionState, error) {
	return hermes.SessionState{}, g.err
}

func registerRuntimeTurn(runtime *assistantRuntime, mode model.AssistantMode) *assistantTurn {
	turn := &assistantTurn{
		ID: "turn-1", ConversationID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1",
		Mode: mode, RuntimeID: "runtime-1", StoredID: "stored-1",
		Citations: map[string]model.Citation{}, Drafts: map[string]model.AssistantDraftReceipt{},
	}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(mode, turn.RuntimeID)] = turn
	runtime.activeByStored[assistantSessionKey(mode, turn.StoredID)] = turn
	return turn
}

func runtimeReplay(runtime *assistantRuntime, conversationID string) []assistantPublicEvent {
	replay, _, stop := runtime.stream(conversationID).subscribe(0, true)
	stop()
	return replay
}

func TestBuildHandoffCoversModeCursorReceiptsAndGatewayFailures(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	qaStored := "qa-stored"
	conversation := model.AssistantConversation{
		ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1",
		LastMode: model.AssistantQA, QASessionID: &qaStored, QAHandoffCursor: -2, CreatedAt: createdAt,
	}
	repository := &runtimeTestRepository{receipts: map[string][]model.AssistantDraftReceipt{
		"qa-legacy-0": {{RevisionID: "revision-1", PageID: "page-1", PageTitle: "Zmluva"}},
	}}
	gateway := hermes.NewFakeGateway()
	gateway.Sessions[model.AssistantQA][qaStored] = []hermes.HistoryMessage{{Role: "user", Text: "Otázka"}, {Role: "assistant", Text: "Odpoveď"}}
	runtime := bareAssistantRuntime(repository, gateway)

	if handoff, cursor, source, err := runtime.buildHandoff(context.Background(), conversation, model.AssistantQA); err != nil || handoff != "" || cursor != -1 || source != "" {
		t.Fatalf("same-mode handoff = %q cursor=%d source=%q err=%v", handoff, cursor, source, err)
	}
	withoutSession := conversation
	withoutSession.QASessionID = nil
	if handoff, cursor, source, err := runtime.buildHandoff(context.Background(), withoutSession, model.AssistantEdit); err != nil || handoff != "" || cursor != -1 || source != "" {
		t.Fatalf("missing-session handoff = %q cursor=%d source=%q err=%v", handoff, cursor, source, err)
	}

	handoff, cursor, source, err := runtime.buildHandoff(context.Background(), conversation, model.AssistantEdit)
	if err != nil || cursor != 2 || source != model.AssistantQA || !strings.Contains(handoff, "USER: Otázka") || !strings.Contains(handoff, "DRAFT_RECEIPT: revisionId=revision-1") {
		t.Fatalf("handoff = %q cursor=%d source=%q err=%v", handoff, cursor, source, err)
	}

	repository.receiptsErr = errors.New("receipts failed")
	conversation.QAHandoffCursor = 1
	handoff, cursor, source, err = runtime.buildHandoff(context.Background(), conversation, model.AssistantEdit)
	if err != nil || cursor != 2 || source != model.AssistantQA || strings.Contains(handoff, "Otázka") || !strings.Contains(handoff, "ASSISTANT: Odpoveď") {
		t.Fatalf("cursor handoff = %q cursor=%d source=%q err=%v", handoff, cursor, source, err)
	}

	offline := hermes.NewFakeGateway()
	offline.Online = false
	runtime = bareAssistantRuntime(repository, offline)
	if _, _, _, err := runtime.buildHandoff(context.Background(), conversation, model.AssistantEdit); !errors.Is(err, hermes.ErrUnavailable) {
		t.Fatalf("resume error = %v", err)
	}

	historyFailure := &historyErrorGateway{FakeGateway: gateway, err: errors.New("history failed")}
	runtime = bareAssistantRuntime(repository, historyFailure)
	if _, _, _, err := runtime.buildHandoff(context.Background(), conversation, model.AssistantEdit); err == nil || err.Error() != "history failed" {
		t.Fatalf("history error = %v", err)
	}

	editStored := "edit-stored"
	editConversation := conversation
	editConversation.LastMode = model.AssistantEdit
	editConversation.EditSessionID = &editStored
	editConversation.EditHandoffCursor = 99
	gateway.Sessions[model.AssistantEdit][editStored] = []hermes.HistoryMessage{{Role: "user", Text: "Edit question"}}
	runtime = bareAssistantRuntime(repository, gateway)
	if handoff, cursor, source, err := runtime.buildHandoff(context.Background(), editConversation, model.AssistantQA); err != nil || cursor != 1 || source != model.AssistantEdit || !strings.Contains(handoff, "Edit question") {
		t.Fatalf("edit handoff = %q cursor=%d source=%q err=%v", handoff, cursor, source, err)
	}

	longHistory := make([]hermes.HistoryMessage, maxHandoffMessages+5)
	for index := range longHistory {
		longHistory[index] = hermes.HistoryMessage{Role: "user", Text: fmt.Sprintf("message-%02d", index)}
	}
	gateway.Sessions[model.AssistantQA][qaStored] = longHistory
	conversation.QAHandoffCursor = 0
	if handoff, _, _, err := runtime.buildHandoff(context.Background(), conversation, model.AssistantEdit); err != nil || strings.Contains(handoff, "message-00") || !strings.Contains(handoff, "message-24") {
		t.Fatalf("bounded handoff = %q err=%v", handoff, err)
	}
}

func TestSubmitAndEnsureSessionCoverLifecycleAndPersistenceFailures(t *testing.T) {
	t.Parallel()

	conversation := model.AssistantConversation{
		ID: "conversation-1", OrganizationID: "organization-1", UserID: "user-1",
		PrimaryMode: model.AssistantQA, LastMode: model.AssistantQA, CreatedAt: time.Now(),
	}

	repository := &runtimeTestRepository{}
	gateway := hermes.NewFakeGateway()
	runtime := bareAssistantRuntime(repository, gateway)
	turn, err := runtime.submit(context.Background(), conversation, model.AssistantQA, "Otázka")
	if err != nil || turn.RuntimeID == "" || turn.StoredID == "" || runtime.conversationState[conversation.ID] != model.AssistantStateRunning {
		t.Fatalf("successful submit turn=%+v state=%s err=%v", turn, runtime.conversationState[conversation.ID], err)
	}
	if len(runtimeReplay(runtime, conversation.ID)) != 1 || len(repository.modeUpdates) != 1 {
		t.Fatal("successful submit did not publish activity and persist mode")
	}
	if _, err := runtime.submit(context.Background(), conversation, model.AssistantQA, "Druhá"); !errors.Is(err, errAssistantTurnActive) {
		t.Fatalf("active submit error=%v", err)
	}

	storedConversation := conversation
	storedConversation.QASessionID = &turn.StoredID
	if cached, _, err := runtime.ensureSession(context.Background(), storedConversation, model.AssistantQA); err != nil || cached.StoredID != turn.StoredID {
		t.Fatalf("cached session=%+v err=%v", cached, err)
	}

	repository = &runtimeTestRepository{setSessionErr: errors.New("persist session failed")}
	runtime = bareAssistantRuntime(repository, hermes.NewFakeGateway())
	if _, _, err := runtime.ensureSession(context.Background(), conversation, model.AssistantEdit); err == nil || err.Error() != "persist session failed" {
		t.Fatalf("session persistence error=%v", err)
	}

	oldStored := "expired-stored"
	expiredConversation := conversation
	expiredConversation.QASessionID = &oldStored
	expired := &expiredResumeGateway{FakeGateway: hermes.NewFakeGateway()}
	runtime = bareAssistantRuntime(&runtimeTestRepository{}, expired)
	if session, updated, err := runtime.ensureSession(context.Background(), expiredConversation, model.AssistantQA); err != nil || session.StoredID == oldStored || updated.QASessionID == nil || *updated.QASessionID != session.StoredID {
		t.Fatalf("expired session=%+v updated=%+v err=%v", session, updated, err)
	}

	offline := hermes.NewFakeGateway()
	offline.Online = false
	runtime = bareAssistantRuntime(&runtimeTestRepository{}, offline)
	if _, err := runtime.submit(context.Background(), conversation, model.AssistantQA, "Otázka"); !errors.Is(err, hermes.ErrUnavailable) {
		t.Fatalf("offline submit error=%v", err)
	}

	submitFailure := &submitErrorGateway{FakeGateway: hermes.NewFakeGateway(), err: errors.New("submit failed")}
	runtime = bareAssistantRuntime(&runtimeTestRepository{}, submitFailure)
	if _, err := runtime.submit(context.Background(), conversation, model.AssistantQA, "Otázka"); err == nil || err.Error() != "submit failed" {
		t.Fatalf("gateway submit error=%v", err)
	}

	repository = &runtimeTestRepository{modeErr: errors.New("mode failed")}
	runtime = bareAssistantRuntime(repository, hermes.NewFakeGateway())
	if _, err := runtime.submit(context.Background(), conversation, model.AssistantQA, "Otázka"); err == nil || err.Error() != "mode failed" {
		t.Fatalf("mode persistence error=%v", err)
	}

	qaStored := "qa-stored"
	handoffConversation := conversation
	handoffConversation.LastMode = model.AssistantQA
	handoffConversation.QASessionID = &qaStored
	gateway = hermes.NewFakeGateway()
	gateway.Sessions[model.AssistantQA][qaStored] = []hermes.HistoryMessage{{Role: "user", Text: "Predošlá otázka"}}
	repository = &runtimeTestRepository{cursorErr: errors.New("cursor failed")}
	runtime = bareAssistantRuntime(repository, gateway)
	if _, err := runtime.submit(context.Background(), handoffConversation, model.AssistantEdit, "Uprav"); err != nil || len(repository.cursorUpdates) != 1 {
		t.Fatalf("handoff cursor warning submit err=%v cursors=%v", err, repository.cursorUpdates)
	}

	historyFailure := &historyErrorGateway{FakeGateway: gateway, err: errors.New("history failed")}
	runtime = bareAssistantRuntime(&runtimeTestRepository{}, historyFailure)
	if _, err := runtime.submit(context.Background(), handoffConversation, model.AssistantEdit, "Uprav"); err == nil || err.Error() != "history failed" {
		t.Fatalf("handoff failure submit error=%v", err)
	}
}

func TestGatewayEventsCoverEveryPublicStateAndFailClosedRequest(t *testing.T) {
	t.Parallel()

	newHarness := func() (*assistantRuntime, *assistantTurn) {
		runtime := bareAssistantRuntime(&runtimeTestRepository{}, hermes.NewFakeGateway())
		return runtime, registerRuntimeTurn(runtime, model.AssistantQA)
	}

	runtime, _ := newHarness()
	runtime.handleGatewayEvent(model.AssistantQA, hermes.Event{Type: "message.delta", SessionID: "missing", Payload: json.RawMessage(`{"text":"ignored"}`)})
	recent := &assistantTurn{ID: "recent", ConversationID: "recent", Mode: model.AssistantQA, RuntimeID: "recent-runtime"}
	runtime.recentByRuntime[assistantSessionKey(model.AssistantQA, recent.RuntimeID)] = recent
	runtime.handleGatewayEvent(model.AssistantQA, hermes.Event{Type: "message.delta", SessionID: recent.RuntimeID, Payload: json.RawMessage(`{"text":"ignored"}`)})
	if len(runtimeReplay(runtime, recent.ConversationID)) != 0 {
		t.Fatal("non-session event for recent turn was published")
	}

	for _, payload := range []json.RawMessage{json.RawMessage(`not-json`), json.RawMessage(`{"text":"` + internalPromptStart + `secret"}`)} {
		runtime, turn := newHarness()
		runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: "message.delta", SessionID: turn.RuntimeID, Payload: payload})
		if len(runtimeReplay(runtime, turn.ConversationID)) != 0 {
			t.Fatalf("invalid or hidden delta produced events for payload %s", payload)
		}
	}
	runtime, turn := newHarness()
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: "message.delta", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"text":"Ahoj"}`)})
	if !turn.HadDelta || len(runtimeReplay(runtime, turn.ConversationID)) != 1 {
		t.Fatal("visible delta was not published")
	}

	for _, test := range []struct {
		name, status string
		state        model.AssistantConversationState
		eventType    string
	}{
		{name: "completed text", status: "complete", state: model.AssistantStateIdle, eventType: "completed"},
		{name: "interrupted", status: "interrupted", state: model.AssistantStateStopped, eventType: "stopped"},
		{name: "error", status: "error", state: model.AssistantStateError, eventType: "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, turn := newHarness()
			turn.Filter.buffer = " tail"
			runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: "message.complete", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"text":"Hotovo","status":"` + test.status + `"}`)})
			events := runtimeReplay(runtime, turn.ConversationID)
			if runtime.conversationState[turn.ConversationID] != test.state || len(events) < 2 || events[len(events)-1].Type != test.eventType {
				t.Fatalf("state=%s events=%+v", runtime.conversationState[turn.ConversationID], events)
			}
		})
	}
	runtime, turn = newHarness()
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: "message.delta", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"text":"streamed"}`)})
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: "message.complete", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"text":"do not duplicate"}`)})
	if events := runtimeReplay(runtime, turn.ConversationID); len(events) != 2 {
		t.Fatalf("already-streamed completion events=%+v", events)
	}

	for _, test := range []struct {
		name, eventType string
		payload         json.RawMessage
	}{
		{name: "gateway error", eventType: "error"},
		{name: "approval", eventType: "approval.request"},
		{name: "sudo", eventType: "sudo.request"},
		{name: "secret", eventType: "secret.request"},
		{name: "terminal read", eventType: "terminal.read.request"},
		{name: "unknown request", eventType: "browser.request"},
		{name: "bad clarification", eventType: "clarify.request", payload: json.RawMessage(`{"question":"","request_id":""}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, turn := newHarness()
			runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: test.eventType, SessionID: turn.RuntimeID, Payload: test.payload})
			if runtime.conversationState[turn.ConversationID] != model.AssistantStateError {
				t.Fatalf("state=%s events=%+v", runtime.conversationState[turn.ConversationID], runtimeReplay(runtime, turn.ConversationID))
			}
		})
	}

	runtime, turn = newHarness()
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: "session.info", SessionID: turn.RuntimeID, Payload: json.RawMessage(`not-json`)})
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: "session.info", SessionID: turn.RuntimeID, Payload: json.RawMessage(`{"stored_session_id":"stored-1"}`)})
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{Type: "unknown", SessionID: turn.RuntimeID})
	if turn.StoredID != "stored-1" || len(runtimeReplay(runtime, turn.ConversationID)) != 0 {
		t.Fatal("ignored session/unknown event changed runtime")
	}
}

func TestToolEventsCoverProgressCompletionCitationsAndInvalidPayloads(t *testing.T) {
	t.Parallel()

	newHarness := func(mode model.AssistantMode) (*assistantRuntime, *assistantTurn) {
		runtime := bareAssistantRuntime(&runtimeTestRepository{}, hermes.NewFakeGateway())
		return runtime, registerRuntimeTurn(runtime, mode)
	}

	for _, test := range []struct {
		name      string
		eventType string
		payload   json.RawMessage
		want      int
	}{
		{name: "invalid", eventType: "tool.start", payload: json.RawMessage(`not-json`)},
		{name: "unnamed progress", eventType: "tool.progress", payload: json.RawMessage(`{}`)},
		{name: "clarify", eventType: "tool.start", payload: json.RawMessage(`{"name":"clarify"}`)},
		{name: "started", eventType: "tool.start", payload: json.RawMessage(`{"name":"search_viki"}`), want: 1},
		{name: "running", eventType: "tool.progress", payload: json.RawMessage(`{"name":"search_viki"}`), want: 1},
		{name: "completed no result", eventType: "tool.complete", payload: json.RawMessage(`{"name":"search_viki"}`), want: 1},
		{name: "completed empty drafts", eventType: "tool.complete", payload: json.RawMessage(`{"name":"apply_viki_draft_changeset","result":{"drafts":[]}}`), want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			mode := model.AssistantQA
			if strings.Contains(string(test.payload), "apply_viki_draft") {
				mode = model.AssistantEdit
			}
			runtime, turn := newHarness(mode)
			runtime.handleToolEvent(turn, hermes.Event{Type: test.eventType, Payload: test.payload})
			if events := runtimeReplay(runtime, turn.ConversationID); len(events) != test.want {
				t.Fatalf("events=%+v want=%d", events, test.want)
			}
		})
	}

	runtime, turn := newHarness(model.AssistantQA)
	payload := json.RawMessage(`{"name":"get_viki_page","result":{"citations":[{"revisionId":"revision-1","pageId":"page-1","title":"Zmluva"},{"revisionId":"revision-1","pageId":"page-1","title":"Zmluva"}]}}`)
	runtime.handleToolEvent(turn, hermes.Event{Type: "tool.complete", Payload: payload})
	runtime.handleToolEvent(turn, hermes.Event{Type: "tool.complete", Payload: payload})
	events := runtimeReplay(runtime, turn.ConversationID)
	if len(events) != 3 || events[0].Type != "activity" || events[1].Type != "citation" || events[2].Type != "activity" || len(turn.Citations) != 1 {
		t.Fatalf("citation tool events=%+v citations=%+v", events, turn.Citations)
	}
}

func TestRuntimeLifecycleCoversConsumeFinishRotationAndReconciliation(t *testing.T) {
	t.Parallel()

	closed := make(chan hermes.Event)
	close(closed)
	runtime := bareAssistantRuntime(&runtimeTestRepository{}, hermes.NewFakeGateway())
	done := make(chan struct{})
	go func() {
		runtime.consume(model.AssistantQA, closed)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("consume did not stop for closed events")
	}

	ctx, cancel := context.WithCancel(context.Background())
	runtime = bareAssistantRuntime(&runtimeTestRepository{}, hermes.NewFakeGateway())
	runtime.ctx = ctx
	events := make(chan hermes.Event)
	done = make(chan struct{})
	go func() {
		runtime.consume(model.AssistantQA, events)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("consume did not stop for context cancellation")
	}

	runtime = bareAssistantRuntime(&runtimeTestRepository{}, hermes.NewFakeGateway())
	turn := &assistantTurn{ID: "turn-empty", ConversationID: "conversation-empty", Mode: model.AssistantQA}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.finishTurn(turn, model.AssistantStateIdle)
	runtime.finishTurn(turn, model.AssistantStateError)
	if runtime.conversationState[turn.ConversationID] != model.AssistantStateIdle || len(runtime.recentByRuntime) != 0 {
		t.Fatalf("finalized empty turn state=%s recent=%v", runtime.conversationState[turn.ConversationID], runtime.recentByRuntime)
	}

	repository := &runtimeTestRepository{setSessionErr: errors.New("rotation failed")}
	runtime = bareAssistantRuntime(repository, hermes.NewFakeGateway())
	turn = registerRuntimeTurn(runtime, model.AssistantEdit)
	runtime.rotateStoredSession(turn, "new-stored")
	if turn.StoredID != "stored-1" {
		t.Fatalf("failed rotation changed stored ID to %q", turn.StoredID)
	}

	repository = &runtimeTestRepository{}
	runtime = bareAssistantRuntime(repository, hermes.NewFakeGateway())
	turn = &assistantTurn{ID: "inactive", ConversationID: "inactive", OrganizationID: "organization-1", UserID: "user-1", Mode: model.AssistantEdit, RuntimeID: "runtime", StoredID: "stored"}
	runtime.rotateStoredSession(turn, "new-stored")
	if _, active := runtime.activeByStored[assistantSessionKey(turn.Mode, "new-stored")]; active {
		t.Fatal("inactive rotated session received a tool grant")
	}

	resume := &rotatingResumeGateway{FakeGateway: hermes.NewFakeGateway()}
	resume.FakeGateway.Sessions[model.AssistantQA]["stored"] = []hermes.HistoryMessage{}
	statusFailure := &statusErrorGateway{rotatingResumeGateway: resume, err: errors.New("status failed")}
	runtime = bareAssistantRuntime(&runtimeTestRepository{}, statusFailure)
	turn = registerRuntimeTurn(runtime, model.AssistantQA)
	runtime.reconcileProfile(model.AssistantQA)
	if runtime.conversationState[turn.ConversationID] != model.AssistantStateError {
		t.Fatalf("status failure state=%s", runtime.conversationState[turn.ConversationID])
	}

	idleGateway := &rotatingResumeGateway{FakeGateway: hermes.NewFakeGateway()}
	idleGateway.FakeGateway.Sessions[model.AssistantQA]["stored-1"] = []hermes.HistoryMessage{}
	runtime = bareAssistantRuntime(&runtimeTestRepository{}, idleGateway)
	turn = registerRuntimeTurn(runtime, model.AssistantQA)
	// Override the running status behavior with the fake gateway's idle status.
	runtime.gateway = idleStatusGateway{rotatingResumeGateway: idleGateway}
	runtime.reconcileProfile(model.AssistantQA)
	if runtime.conversationState[turn.ConversationID] != model.AssistantStateIdle {
		t.Fatalf("idle reconciliation state=%s", runtime.conversationState[turn.ConversationID])
	}
}

func TestRuntimeRecentTurnExpiryClarificationNilAndGatewayReady(t *testing.T) {
	previous := assistantRecentTurnRetention
	assistantRecentTurnRetention = time.Millisecond
	t.Cleanup(func() { assistantRecentTurnRetention = previous })

	runtime := bareAssistantRuntime(&runtimeTestRepository{}, hermes.NewFakeGateway())
	if runtime.clarification("missing") != nil {
		t.Fatal("missing clarification was non-nil")
	}
	turn := registerRuntimeTurn(runtime, model.AssistantQA)
	runtime.finishTurn(turn, model.AssistantStateIdle)
	deadline := time.Now().Add(time.Second)
	for {
		runtime.mu.Lock()
		_, exists := runtime.recentByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)]
		runtime.mu.Unlock()
		if !exists {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recent turn did not expire")
		}
		time.Sleep(time.Millisecond)
	}

	runtime.handleGatewayEvent(model.AssistantQA, hermes.Event{Type: "gateway.ready"})
}

type idleStatusGateway struct{ rotatingResumeGateway *rotatingResumeGateway }

func (g idleStatusGateway) Status() hermes.GatewayStatus {
	return g.rotatingResumeGateway.Status()
}
func (g idleStatusGateway) CreateSession(ctx context.Context, mode model.AssistantMode) (hermes.Session, error) {
	return g.rotatingResumeGateway.CreateSession(ctx, mode)
}
func (g idleStatusGateway) ResumeSession(ctx context.Context, mode model.AssistantMode, id string) (hermes.Session, error) {
	return g.rotatingResumeGateway.ResumeSession(ctx, mode, id)
}
func (g idleStatusGateway) History(ctx context.Context, mode model.AssistantMode, id string) ([]hermes.HistoryMessage, error) {
	return g.rotatingResumeGateway.History(ctx, mode, id)
}
func (g idleStatusGateway) SessionStatus(context.Context, model.AssistantMode, string) (hermes.SessionState, error) {
	return hermes.SessionState{Running: false, Status: "idle"}, nil
}
func (g idleStatusGateway) Submit(ctx context.Context, mode model.AssistantMode, id, text string) error {
	return g.rotatingResumeGateway.Submit(ctx, mode, id, text)
}
func (g idleStatusGateway) Interrupt(ctx context.Context, mode model.AssistantMode, id string) error {
	return g.rotatingResumeGateway.Interrupt(ctx, mode, id)
}
func (g idleStatusGateway) RespondClarification(ctx context.Context, mode model.AssistantMode, id, requestID, answer string) error {
	return g.rotatingResumeGateway.RespondClarification(ctx, mode, id, requestID, answer)
}
func (g idleStatusGateway) Events(mode model.AssistantMode) <-chan hermes.Event {
	return g.rotatingResumeGateway.Events(mode)
}
func (g idleStatusGateway) Close() error { return g.rotatingResumeGateway.Close() }

func TestVisibleHistoryCoversIgnoredToolsReceiptsEmptyMessagesAndNumbering(t *testing.T) {
	t.Parallel()

	runtime := bareAssistantRuntime(&runtimeTestRepository{}, hermes.NewFakeGateway())
	timestamp := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	turn := &assistantTurn{ID: "turn-1", Mode: model.AssistantEdit}
	prompt := runtime.wrapPrompt(turn, "Používateľ", "", timestamp)
	history := []hermes.HistoryMessage{
		{Role: "tool", Name: "search_viki", Result: json.RawMessage(`{"citations":[]}`)},
		{Role: "system", Text: "ignore"},
		{Role: "user", Text: prompt},
		{Role: "tool", Name: "terminal", Result: json.RawMessage(`{"citations":[{"revisionId":"bad","pageId":"bad"}]}`)},
		{Role: "tool", Name: "search_viki"},
		{Role: "tool", Name: "search_viki", Result: json.RawMessage(`not-json`)},
		{Role: "tool", Name: "search_viki", Result: json.RawMessage(`{"citations":[{"revisionId":"","pageId":"page"},{"revisionId":"revision-no-page"}],"drafts":[{"revisionId":"","pageId":"page"},{"revisionId":"draft-no-page"}]}`)},
		{Role: "tool", Name: "get_viki_page", Result: json.RawMessage(`{"citations":[{"revisionId":"revision-1","pageId":"page-1","pageTitle":"Zmluva"}],"drafts":[{"revisionId":"draft-1","pageId":"page-2","pageTitle":"Dodatok"}]}`)},
		{Role: "assistant", Text: ""},
		{Role: "assistant", Text: "Druhá odpoveď"},
		{Role: "user", Text: "Legacy"},
		{Role: "tool", Name: "get_viki_revision", Result: json.RawMessage(`{"drafts":[{"revisionId":"draft-2","pageId":"page-3"}]}`)},
		{Role: "user", Text: "Ďalšia otázka"},
	}
	messages := visibleHistory(runtime, model.AssistantEdit, history, timestamp.Add(-time.Hour))
	if len(messages) != 6 {
		t.Fatalf("visible messages=%d %+v", len(messages), messages)
	}
	if messages[1].Content != "Vytvoril som drafty vo viki." || len(messages[1].Citations) != 1 || len(messages[1].Drafts) != 1 {
		t.Fatalf("empty assistant receipt=%+v", messages[1])
	}
	if messages[2].ID != "turn-1-assistant-2" || messages[2].Content != "Druhá odpoveď" {
		t.Fatalf("numbered assistant=%+v", messages[2])
	}
	if messages[4].Role != "assistant" || messages[4].Content != "Vytvoril som drafty vo viki." || len(messages[4].Drafts) != 1 {
		t.Fatalf("flushed receipt=%+v", messages[4])
	}

	hidden := visibleHistory(runtime, model.AssistantQA, []hermes.HistoryMessage{{Role: "user", Text: ""}, {Role: "assistant", Text: ""}}, timestamp)
	if len(hidden) != 0 {
		t.Fatalf("empty messages visible=%+v", hidden)
	}

	filter := internalStreamFilter{inside: true, buffer: strings.Repeat("x", maxHandoffBytes+4096)}
	if visible := filter.Feed("y"); visible != "" || len(filter.buffer) >= maxHandoffBytes+2048 {
		t.Fatalf("oversized internal filter output=%q buffer=%d", visible, len(filter.buffer))
	}
	if got := truncateUTF8("žltá", 1); got != "" {
		t.Fatalf("continuation truncation=%q", got)
	}
}
