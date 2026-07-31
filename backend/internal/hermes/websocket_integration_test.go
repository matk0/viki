package hermes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"viki/internal/model"
)

func TestManagerRealWebSocketLifecycleAndCorrelation(t *testing.T) {
	t.Parallel()

	gateway := newFakeTUIGateway(t, "gateway-token")
	defer gateway.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager, err := NewManager(ctx, ManagerConfig{QA: ProfileConfig{
		URL: gateway.WebSocketURL(), Token: "gateway-token", Configured: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	waitForProfileConnection(t, manager, model.AssistantQA)
	waitForGatewayEvent(t, manager.Events(model.AssistantQA), "gateway.ready", "")

	callContext, callCancel := context.WithTimeout(ctx, 3*time.Second)
	defer callCancel()
	session, err := manager.CreateSession(callContext, model.AssistantQA)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeID != "runtime-1" || session.StoredID != "stored-1" {
		t.Fatalf("created session = %+v", session)
	}

	type historyResult struct {
		messages []HistoryMessage
		err      error
	}
	type statusResult struct {
		state SessionState
		err   error
	}
	historyResults := make(chan historyResult, 1)
	statusResults := make(chan statusResult, 1)
	go func() {
		messages, historyErr := manager.History(callContext, model.AssistantQA, session.RuntimeID)
		historyResults <- historyResult{messages: messages, err: historyErr}
	}()
	go func() {
		state, statusErr := manager.SessionStatus(callContext, model.AssistantQA, session.RuntimeID)
		statusResults <- statusResult{state: state, err: statusErr}
	}()
	history := <-historyResults
	status := <-statusResults
	if history.err != nil || status.err != nil {
		t.Fatalf("correlated calls failed: history=%v status=%v", history.err, status.err)
	}
	if len(history.messages) != 1 || history.messages[0].Text != "existing history" {
		t.Fatalf("history response crossed correlation: %+v", history.messages)
	}
	if status.state.Running || status.state.Status != "idle" {
		t.Fatalf("status response crossed correlation: %+v", status.state)
	}

	if err := manager.Submit(callContext, model.AssistantQA, session.RuntimeID, "question"); err != nil {
		t.Fatal(err)
	}
	delta := waitForGatewayEvent(t, manager.Events(model.AssistantQA), "message.delta", session.RuntimeID)
	var deltaPayload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(delta.Payload, &deltaPayload); err != nil || deltaPayload.Text != "partial" {
		t.Fatalf("unexpected streamed delta: payload=%s err=%v", delta.Payload, err)
	}
	clarification := waitForGatewayEvent(t, manager.Events(model.AssistantQA), "clarify.request", session.RuntimeID)
	var clarificationPayload struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(clarification.Payload, &clarificationPayload); err != nil || clarificationPayload.RequestID != "clarify-1" {
		t.Fatalf("unexpected clarification: payload=%s err=%v", clarification.Payload, err)
	}
	if err := manager.RespondClarification(callContext, model.AssistantQA, session.RuntimeID, "clarify-1", "answer"); err != nil {
		t.Fatal(err)
	}
	waitForGatewayEvent(t, manager.Events(model.AssistantQA), "message.complete", session.RuntimeID)
	if request := gateway.LastRequest("clarify.respond"); request == nil || request.Params["answer"] != "answer" {
		t.Fatalf("clarification response was not forwarded: %+v", request)
	}

	if err := manager.Submit(callContext, model.AssistantQA, session.RuntimeID, "wait"); err != nil {
		t.Fatal(err)
	}
	state, err := manager.SessionStatus(callContext, model.AssistantQA, session.RuntimeID)
	if err != nil || !state.Running {
		t.Fatalf("running session status = %+v, %v", state, err)
	}
	if err := manager.Interrupt(callContext, model.AssistantQA, session.RuntimeID); err != nil {
		t.Fatal(err)
	}
	interrupted := waitForGatewayEvent(t, manager.Events(model.AssistantQA), "message.complete", session.RuntimeID)
	var interruptedPayload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(interrupted.Payload, &interruptedPayload); err != nil || interruptedPayload.Status != "interrupted" {
		t.Fatalf("unexpected interrupt completion: payload=%s err=%v", interrupted.Payload, err)
	}
	if gateway.LastRequest("session.interrupt") == nil {
		t.Fatal("interrupt was not forwarded through JSON-RPC")
	}

	if method := gateway.LastUnexpectedMethod(); method != "config.changed" {
		t.Fatalf("fake gateway did not exercise ignored non-event envelope, got %q", method)
	}
	select {
	case event := <-manager.Events(model.AssistantQA):
		if event.Type == "config.changed" {
			t.Fatalf("non-event JSON-RPC notification leaked into gateway events: %+v", event)
		}
	default:
	}
	if !gateway.SawToken("gateway-token") {
		t.Fatal("websocket connector omitted its gateway token")
	}
}

func TestManagerRealWebSocketReconnectAndDurableRotation(t *testing.T) {
	t.Parallel()

	gateway := newFakeTUIGateway(t, "reconnect-token")
	defer gateway.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager, err := NewManager(ctx, ManagerConfig{Edit: ProfileConfig{
		URL: gateway.WebSocketURL(), Token: "reconnect-token", Configured: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	waitForProfileConnection(t, manager, model.AssistantEdit)
	waitForGatewayEvent(t, manager.Events(model.AssistantEdit), "gateway.ready", "")
	callContext, callCancel := context.WithTimeout(ctx, 5*time.Second)
	defer callCancel()
	created, err := manager.CreateSession(callContext, model.AssistantEdit)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := manager.ResumeSession(callContext, model.AssistantEdit, created.StoredID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.StoredID != "rotated-stored-1" || resumed.RuntimeID != "runtime-resumed-1" {
		t.Fatalf("resume did not surface durable rotation: %+v", resumed)
	}

	connections := gateway.ConnectionCount()
	gateway.DisconnectClients()
	waitForConnectionCount(t, gateway, connections+1)
	waitForProfileConnection(t, manager, model.AssistantEdit)
	waitForGatewayEvent(t, manager.Events(model.AssistantEdit), "gateway.ready", "")

	again, err := manager.ResumeSession(callContext, model.AssistantEdit, resumed.StoredID)
	if err != nil {
		t.Fatal(err)
	}
	if again.StoredID != resumed.StoredID || again.RuntimeID != "runtime-resumed-2" {
		t.Fatalf("session was not reusable after reconnect: before=%+v after=%+v", resumed, again)
	}
}

type fakeTUIRequest struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type fakeTUIConnection struct {
	connection *websocket.Conn
	writeMu    sync.Mutex
}

func (c *fakeTUIConnection) Write(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.connection.WriteJSON(value)
}

type fakeTUIGateway struct {
	t             *testing.T
	server        *httptest.Server
	expectedToken string

	mu               sync.Mutex
	connections      map[*fakeTUIConnection]struct{}
	connectionCount  int
	nextSession      int
	nextResume       int
	running          bool
	requests         []fakeTUIRequest
	tokens           []string
	unexpectedMethod string
}

func newFakeTUIGateway(t *testing.T, token string) *fakeTUIGateway {
	t.Helper()
	gateway := &fakeTUIGateway{
		t: t, expectedToken: token, connections: map[*fakeTUIConnection]struct{}{},
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	gateway.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/ws" {
			http.NotFound(w, request)
			return
		}
		connection, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			return
		}
		client := &fakeTUIConnection{connection: connection}
		gateway.mu.Lock()
		gateway.connections[client] = struct{}{}
		gateway.connectionCount++
		gateway.tokens = append(gateway.tokens, request.URL.Query().Get("token"))
		gateway.mu.Unlock()
		defer func() {
			gateway.mu.Lock()
			delete(gateway.connections, client)
			gateway.mu.Unlock()
			_ = connection.Close()
		}()
		_ = client.Write(map[string]any{
			"jsonrpc": "2.0", "method": "event",
			"params": map[string]any{"type": "gateway.ready", "payload": map[string]any{"skin": "test"}},
		})
		for {
			var rpcRequest fakeTUIRequest
			if err := connection.ReadJSON(&rpcRequest); err != nil {
				return
			}
			gateway.mu.Lock()
			gateway.requests = append(gateway.requests, rpcRequest)
			gateway.mu.Unlock()
			go gateway.handle(client, rpcRequest)
		}
	}))
	return gateway
}

func (g *fakeTUIGateway) handle(client *fakeTUIConnection, request fakeTUIRequest) {
	response := func(result any) {
		_ = client.Write(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}
	emit := func(eventType, sessionID string, payload any) {
		_ = client.Write(map[string]any{
			"jsonrpc": "2.0", "method": "event",
			"params": map[string]any{"type": eventType, "session_id": sessionID, "payload": payload},
		})
	}
	sessionID, _ := request.Params["session_id"].(string)
	switch request.Method {
	case "session.create":
		g.mu.Lock()
		g.nextSession++
		number := g.nextSession
		g.mu.Unlock()
		response(map[string]any{
			"session_id":        fmt.Sprintf("runtime-%d", number),
			"stored_session_id": fmt.Sprintf("stored-%d", number),
		})
	case "session.resume":
		g.mu.Lock()
		g.nextResume++
		number := g.nextResume
		g.mu.Unlock()
		storedID := sessionID
		if storedID == "stored-1" {
			storedID = "rotated-stored-1"
		}
		response(map[string]any{
			"session_id":  fmt.Sprintf("runtime-resumed-%d", number),
			"session_key": storedID,
			"resumed":     storedID,
		})
	case "session.history":
		time.Sleep(25 * time.Millisecond)
		response(map[string]any{"count": 1, "messages": []map[string]any{{"role": "assistant", "text": "existing history"}}})
	case "session.status":
		time.Sleep(time.Millisecond)
		g.mu.Lock()
		running := g.running
		g.mu.Unlock()
		runningText := "No"
		if running {
			runningText = "Yes"
		}
		response(map[string]any{"output": "Hermes TUI Status\nAgent Running: " + runningText})
	case "prompt.submit":
		text, _ := request.Params["text"].(string)
		g.mu.Lock()
		g.running = true
		g.mu.Unlock()
		response(map[string]any{"status": "accepted"})
		if text != "wait" {
			_ = client.Write([]byte(`not-json`))
			g.mu.Lock()
			g.unexpectedMethod = "config.changed"
			g.mu.Unlock()
			_ = client.Write(map[string]any{"jsonrpc": "2.0", "method": "config.changed", "params": map[string]any{"secret": "must-not-leak"}})
			emit("message.delta", sessionID, map[string]any{"text": "partial"})
			emit("clarify.request", sessionID, map[string]any{
				"question": "Which option?", "choices": []string{"A", "B"}, "request_id": "clarify-1",
			})
		}
	case "clarify.respond":
		response(map[string]any{"accepted": true})
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
		emit("message.complete", sessionID, map[string]any{"text": "complete", "status": "complete"})
	case "session.interrupt":
		response(map[string]any{"interrupted": true})
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
		emit("message.complete", sessionID, map[string]any{"text": "", "status": "interrupted"})
	default:
		_ = client.Write(map[string]any{
			"jsonrpc": "2.0", "id": request.ID,
			"error": map[string]any{"code": -32601, "message": "method not found"},
		})
	}
}

func (g *fakeTUIGateway) WebSocketURL() string {
	return "ws" + strings.TrimPrefix(g.server.URL, "http") + "/api/ws"
}

func (g *fakeTUIGateway) Close() {
	g.DisconnectClients()
	g.server.Close()
}

func (g *fakeTUIGateway) DisconnectClients() {
	g.mu.Lock()
	connections := make([]*fakeTUIConnection, 0, len(g.connections))
	for connection := range g.connections {
		connections = append(connections, connection)
	}
	g.mu.Unlock()
	for _, connection := range connections {
		_ = connection.connection.Close()
	}
}

func (g *fakeTUIGateway) ConnectionCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.connectionCount
}

func (g *fakeTUIGateway) LastRequest(method string) *fakeTUIRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	for index := len(g.requests) - 1; index >= 0; index-- {
		if g.requests[index].Method == method {
			request := g.requests[index]
			return &request
		}
	}
	return nil
}

func (g *fakeTUIGateway) SawToken(token string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, value := range g.tokens {
		if value == token && value == g.expectedToken {
			return true
		}
	}
	return false
}

func (g *fakeTUIGateway) LastUnexpectedMethod() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.unexpectedMethod
}

func waitForProfileConnection(t *testing.T, manager *Manager, mode model.AssistantMode) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if manager.Status().Profiles[string(mode)].Connected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("Hermes %s profile did not connect: %+v", mode, manager.Status())
}

func waitForConnectionCount(t *testing.T, gateway *fakeTUIGateway, minimum int) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if gateway.ConnectionCount() >= minimum {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("gateway connections = %d, want at least %d", gateway.ConnectionCount(), minimum)
}

func waitForGatewayEvent(t *testing.T, events <-chan Event, eventType, sessionID string) Event {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, open := <-events:
			if !open {
				t.Fatal("Hermes event stream closed")
			}
			if event.Type == eventType && (sessionID == "" || event.SessionID == sessionID) {
				return event
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for Hermes event %q for session %q", eventType, sessionID)
		}
	}
}
