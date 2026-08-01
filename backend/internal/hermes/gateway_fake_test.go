package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"viki/internal/model"
)

func TestFakeGatewayModelsSessionLifecycleAndAvailability(t *testing.T) {
	gateway := NewFakeGateway()
	ctx := context.Background()

	status := gateway.Status()
	if !status.Available || !status.Profiles["qa"].Connected || !status.Profiles["edit"].Configured {
		t.Fatalf("unexpected online status: %+v", status)
	}

	session, err := gateway.CreateSession(ctx, model.AssistantQA)
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.Submit(ctx, model.AssistantQA, session.RuntimeID, "question"); err != nil {
		t.Fatal(err)
	}
	history, err := gateway.History(ctx, model.AssistantQA, session.RuntimeID)
	if err != nil || len(history) != 1 || history[0].Text != "question" {
		t.Fatalf("history = %+v, error = %v", history, err)
	}
	history[0].Text = "mutated copy"
	stored, err := gateway.History(ctx, model.AssistantQA, session.StoredID)
	if err != nil || stored[0].Text != "question" {
		t.Fatalf("stored history was not isolated: %+v, %v", stored, err)
	}

	resumed, err := gateway.ResumeSession(ctx, model.AssistantQA, session.StoredID)
	if err != nil || resumed != session {
		t.Fatalf("resumed session = %+v, error = %v", resumed, err)
	}
	if _, err := gateway.ResumeSession(ctx, model.AssistantQA, "missing"); err == nil {
		t.Fatal("missing fake session was resumed")
	}
	state, err := gateway.SessionStatus(ctx, model.AssistantQA, session.RuntimeID)
	if err != nil || state.Status != "idle" {
		t.Fatalf("session state = %+v, error = %v", state, err)
	}
	if err := gateway.Interrupt(ctx, model.AssistantQA, session.RuntimeID); err != nil {
		t.Fatal(err)
	}
	if err := gateway.RespondClarification(ctx, model.AssistantQA, session.RuntimeID, "request", "answer"); err != nil {
		t.Fatal(err)
	}

	event := Event{Type: "message.complete", SessionID: session.RuntimeID}
	gateway.Emit(model.AssistantQA, event)
	select {
	case received := <-gateway.Events(model.AssistantQA):
		if received.Type != event.Type || received.SessionID != event.SessionID {
			t.Fatalf("received event = %+v", received)
		}
	case <-time.After(time.Second):
		t.Fatal("fake event was not emitted")
	}
	if err := gateway.Close(); err != nil {
		t.Fatal(err)
	}

	gateway.Online = false
	if gateway.Status().Available {
		t.Fatal("offline fake gateway reported available")
	}
	if _, err := gateway.CreateSession(ctx, model.AssistantEdit); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("offline create error = %v", err)
	}
	if _, err := gateway.ResumeSession(ctx, model.AssistantQA, session.StoredID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("offline resume error = %v", err)
	}
}

func TestManagerFailsClosedForUnavailableAndInvalidProfiles(t *testing.T) {
	ctx := context.Background()
	manager, err := NewManager(ctx, ManagerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if manager.Status().Available {
		t.Fatal("manager without profiles reported available")
	}
	if _, err := manager.CreateSession(ctx, model.AssistantQA); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured create error = %v", err)
	}
	if _, err := manager.ResumeSession(ctx, model.AssistantQA, "stored"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured resume error = %v", err)
	}
	if _, err := manager.History(ctx, model.AssistantQA, "runtime"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured history error = %v", err)
	}
	if _, err := manager.SessionStatus(ctx, model.AssistantQA, "runtime"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured status error = %v", err)
	}
	if err := manager.Submit(ctx, model.AssistantQA, "runtime", "question"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured submit error = %v", err)
	}
	if err := manager.Interrupt(ctx, model.AssistantQA, "runtime"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured interrupt error = %v", err)
	}
	if err := manager.RespondClarification(ctx, model.AssistantQA, "runtime", "request", "answer"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured clarification error = %v", err)
	}
	if _, open := <-manager.Events(model.AssistantQA); open {
		t.Fatal("missing profile returned an open event channel")
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	for _, rawURL := range []string{"http://localhost:9119", "%"} {
		t.Run(rawURL, func(t *testing.T) {
			configured, err := NewManager(ctx, ManagerConfig{QA: ProfileConfig{
				URL: rawURL, Configured: true,
			}})
			if err == nil {
				_ = configured.Close()
				t.Fatalf("invalid gateway URL %q was accepted", rawURL)
			}
		})
	}
}

func TestRPCErrorIncludesCodeAndMessage(t *testing.T) {
	err := (&RPCError{Code: -32000, Message: "failure"}).Error()
	if err != "Hermes RPC -32000: failure" {
		t.Fatalf("RPC error = %q", err)
	}
}

func TestManagerValidatesGatewaySessionResponsesAndPropagatesErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newFakeWire()
	rpcClient := newClient(ctx, fakeConnector{wire: w}, clientOptions{reconnectDelay: time.Millisecond})
	defer rpcClient.Close()
	wait, waitCancel := context.WithTimeout(ctx, time.Second)
	defer waitCancel()
	if err := rpcClient.WaitConnected(wait); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{
		clients:    map[model.AssistantMode]*client{model.AssistantQA: rpcClient},
		configured: map[model.AssistantMode]bool{model.AssistantQA: true},
	}

	respond := func(result any, rpcErr *RPCError) {
		t.Helper()
		requestPayload := <-w.writes
		var request rpcRequest
		if err := json.Unmarshal(requestPayload, &request); err != nil {
			t.Fatal(err)
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}
		if rpcErr != nil {
			delete(response, "result")
			response["error"] = rpcErr
		}
		payload, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		w.reads <- payload
	}
	call := func(invoke func() error, result any, rpcErr *RPCError) error {
		t.Helper()
		completed := make(chan error, 1)
		go func() { completed <- invoke() }()
		respond(result, rpcErr)
		return <-completed
	}

	var created Session
	err := call(func() error {
		var err error
		created, err = manager.CreateSession(ctx, model.AssistantQA)
		return err
	}, map[string]any{"session_id": "runtime-1", "session_key": "stored-1"}, nil)
	if err != nil || created.StoredID != "stored-1" {
		t.Fatalf("fallback create session = %+v, %v", created, err)
	}
	if err := call(func() error {
		_, err := manager.CreateSession(ctx, model.AssistantQA)
		return err
	}, map[string]any{}, nil); err == nil {
		t.Fatal("create response without identifiers was accepted")
	}
	if err := call(func() error {
		_, err := manager.CreateSession(ctx, model.AssistantQA)
		return err
	}, nil, &RPCError{Code: -1, Message: "create failed"}); err == nil {
		t.Fatal("create RPC error was ignored")
	}

	var resumed Session
	err = call(func() error {
		var err error
		resumed, err = manager.ResumeSession(ctx, model.AssistantQA, "stored-original")
		return err
	}, map[string]any{"session_id": "runtime-2", "resumed": "stored-rotated"}, nil)
	if err != nil || resumed.StoredID != "stored-rotated" {
		t.Fatalf("resumed fallback session = %+v, %v", resumed, err)
	}
	err = call(func() error {
		var err error
		resumed, err = manager.ResumeSession(ctx, model.AssistantQA, "stored-original")
		return err
	}, map[string]any{"session_id": "runtime-3"}, nil)
	if err != nil || resumed.StoredID != "stored-original" {
		t.Fatalf("original durable session fallback = %+v, %v", resumed, err)
	}
	if err := call(func() error {
		_, err := manager.ResumeSession(ctx, model.AssistantQA, "stored-original")
		return err
	}, map[string]any{}, nil); err == nil {
		t.Fatal("resume response without runtime identifier was accepted")
	}
	if err := call(func() error {
		_, err := manager.ResumeSession(ctx, model.AssistantQA, "stored-original")
		return err
	}, nil, &RPCError{Code: -1, Message: "resume failed"}); err == nil {
		t.Fatal("resume RPC error was ignored")
	}

	for name, invoke := range map[string]func() error{
		"history": func() error {
			_, err := manager.History(ctx, model.AssistantQA, "runtime")
			return err
		},
		"status": func() error {
			_, err := manager.SessionStatus(ctx, model.AssistantQA, "runtime")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(invoke, nil, &RPCError{Code: -1, Message: fmt.Sprintf("%s failed", name)}); err == nil {
				t.Fatalf("%s RPC error was ignored", name)
			}
		})
	}
}

type closeErrorWire struct{}

func (closeErrorWire) Read(context.Context) ([]byte, error) { return nil, errors.New("closed") }
func (closeErrorWire) Write(context.Context, []byte) error  { return nil }
func (closeErrorWire) Close() error                         { return errors.New("close failed") }

func TestManagerCloseReturnsProfileCloseErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rpcClient := &client{
		ctx:        ctx,
		cancel:     cancel,
		connection: closeErrorWire{},
		connected:  true,
		changed:    make(chan struct{}),
		pending:    map[string]chan rpcResponse{},
		events:     make(chan Event),
	}
	manager := &Manager{
		clients: map[model.AssistantMode]*client{model.AssistantQA: rpcClient},
	}
	if err := manager.Close(); err == nil || !errors.Is(err, errors.New("close failed")) && err.Error() != "close failed" {
		t.Fatalf("manager close error = %v", err)
	}
}
