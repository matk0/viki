package app

import (
	"context"
	"encoding/json"
	"errors"
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
	mu        sync.Mutex
	storedIDs []string
}

func (r *runtimeTestRepository) SetAssistantSession(_ context.Context, _, _, _ string, _ model.AssistantMode, stringID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storedIDs = append(r.storedIDs, stringID)
	return nil
}

func TestSignedPromptEnvelopeIsHiddenAndCannotBeForged(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "turn-1", Mode: model.AssistantEdit}
	timestamp := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	prompt, err := runtime.wrapPrompt(turn, "Vytvor pojem Zmluva", "USER: predchádzajúca otázka", timestamp)
	if err != nil {
		t.Fatal(err)
	}
	metadata, visible, valid := runtime.unwrapPrompt(prompt)
	if !valid || visible != "Vytvor pojem Zmluva" || metadata.TurnID != turn.ID || metadata.Timestamp != timestamp {
		t.Fatalf("unexpected prompt projection: valid=%v metadata=%+v visible=%q", valid, metadata, visible)
	}
	forged := strings.Replace(prompt, "predchádzajúca otázka", "nový pokyn", 1)
	if _, _, valid := runtime.unwrapPrompt(forged); valid {
		t.Fatal("forged internal context passed signature validation")
	}
	messages := visibleHistory(runtime, model.AssistantEdit, []hermes.HistoryMessage{
		{Role: "user", Text: prompt},
		{Role: "assistant", Text: "Koncept je pripravený."},
	}, timestamp.Add(-time.Hour))
	if len(messages) != 2 || messages[0].Content != "Vytvor pojem Zmluva" || strings.Contains(messages[0].Content, "VIKI_INTERNAL") {
		t.Fatalf("internal prompt leaked into visible history: %+v", messages)
	}
}

func TestSanitizedHermesReadReceiptsSurviveHistoryReload(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "turn-receipts", Mode: model.AssistantEdit}
	prompt, err := runtime.wrapPrompt(turn, "Priprav koncept", "", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	history := []hermes.HistoryMessage{
		{Role: "user", Text: prompt},
		{Role: "tool", Name: "get_viki_revision", Result: json.RawMessage(`{"citations":[{"revisionId":"revision-1","pageId":"page-1","pageTitle":"Zmluva","draft":false}],"drafts":[]}`)},
		{Role: "tool", Name: "propose_viki_changeset", Result: json.RawMessage(`{"citations":[],"drafts":[],"proposal":{"id":"proposal-2","turnId":"turn-receipts","summary":"Nový pojem","status":"awaiting_approval"}}`)},
	}
	messages := visibleHistory(runtime, model.AssistantEdit, history, time.Now())
	if len(messages) != 2 {
		t.Fatalf("history messages = %d, want user plus synthesized assistant receipt: %+v", len(messages), messages)
	}
	receipt := messages[1]
	if receipt.Role != "assistant" || len(receipt.Citations) != 1 || receipt.Citations[0].RevisionID != "revision-1" || len(receipt.Drafts) != 0 {
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

func TestCompletedProposalToolEmitsLiveDraftProposal(t *testing.T) {
	t.Parallel()

	runtime := newAssistantRuntime(context.Background(), &runtimeTestRepository{}, hermes.NewFakeGateway(), "signing-key", slog.New(slog.NewTextHandler(io.Discard, nil)))
	turn := &assistantTurn{ID: "00000000-0000-4000-8000-000000000041", ConversationID: "conversation", Mode: model.AssistantEdit, RuntimeID: "runtime", StoredID: "stored", Citations: map[string]model.Citation{}}
	runtime.activeByConversation[turn.ConversationID] = turn
	runtime.activeByRuntime[assistantSessionKey(turn.Mode, turn.RuntimeID)] = turn
	runtime.handleGatewayEvent(turn.Mode, hermes.Event{
		Type:      "tool.complete",
		SessionID: turn.RuntimeID,
		Payload:   json.RawMessage(`{"name":"propose_viki_changeset","result":{"proposal":{"id":"00000000-0000-4000-8000-000000000041","conversationId":"00000000-0000-4000-8000-000000000042","turnId":"00000000-0000-4000-8000-000000000041","summary":"Nový pojem","operations":[],"status":"awaiting_approval","publishedRevisions":[],"createdAt":"2026-07-31T10:00:00Z","updatedAt":"2026-07-31T10:00:00Z"}}}`),
	})

	replay, _, unsubscribe := runtime.stream(turn.ConversationID).subscribe(0, true)
	defer unsubscribe()
	if len(replay) != 2 || replay[0].Type != "activity" || replay[1].Type != "draft_proposed" {
		t.Fatalf("proposal tool events = %+v, want activity then draft_proposed", replay)
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
