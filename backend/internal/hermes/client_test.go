package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeWire struct {
	reads  chan []byte
	writes chan []byte
	closed chan struct{}
	once   sync.Once
}

func newFakeWire() *fakeWire {
	return &fakeWire{
		reads:  make(chan []byte, 8),
		writes: make(chan []byte, 8),
		closed: make(chan struct{}),
	}
}

func (w *fakeWire) Read(context.Context) ([]byte, error) {
	select {
	case payload := <-w.reads:
		return payload, nil
	case <-w.closed:
		return nil, errors.New("closed")
	}
}

func (w *fakeWire) Write(_ context.Context, payload []byte) error {
	select {
	case w.writes <- append([]byte(nil), payload...):
		return nil
	case <-w.closed:
		return errors.New("closed")
	}
}

func (w *fakeWire) Close() error {
	w.once.Do(func() { close(w.closed) })
	return nil
}

type fakeConnector struct{ wire *fakeWire }

func (c fakeConnector) Connect(context.Context) (wire, error) { return c.wire, nil }

func TestClientCorrelatesResponsesAndRejectsNonAllowlistedMethods(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newFakeWire()
	client := newClient(ctx, fakeConnector{wire: w}, clientOptions{reconnectDelay: time.Millisecond})
	defer client.Close()

	w.reads <- []byte(`{"jsonrpc":"2.0","method":"event","params":{"type":"gateway.ready","payload":{}}}`)
	connected, waitCancel := context.WithTimeout(ctx, time.Second)
	defer waitCancel()
	if err := client.WaitConnected(connected); err != nil {
		t.Fatal(err)
	}

	type callResult struct {
		value string
		err   error
	}
	results := make(chan callResult, 2)
	go func() {
		var response struct {
			SessionID string `json:"session_id"`
		}
		err := client.Call(ctx, "session.create", map[string]any{}, &response)
		results <- callResult{value: response.SessionID, err: err}
	}()
	go func() {
		var response struct {
			Status string `json:"status"`
		}
		err := client.Call(ctx, "session.status", map[string]any{"session_id": "runtime"}, &response)
		results <- callResult{value: response.Status, err: err}
	}()

	requests := make([]rpcRequest, 0, 2)
	for len(requests) < 2 {
		select {
		case payload := <-w.writes:
			var request rpcRequest
			if err := json.Unmarshal(payload, &request); err != nil {
				t.Fatal(err)
			}
			requests = append(requests, request)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for JSON-RPC requests")
		}
	}

	for index := len(requests) - 1; index >= 0; index-- {
		request := requests[index]
		result := map[string]any{"status": "idle"}
		if request.Method == "session.create" {
			result = map[string]any{"session_id": "runtime"}
		}
		payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		w.reads <- payload
	}

	seen := map[string]bool{}
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		seen[result.value] = true
	}
	if !seen["runtime"] || !seen["idle"] {
		t.Fatalf("unexpected correlated results: %#v", seen)
	}

	if err := client.Call(ctx, "command.dispatch", map[string]any{"command": "/model unsafe"}, &struct{}{}); !errors.Is(err, ErrMethodNotAllowed) {
		t.Fatalf("non-allowlisted method error = %v, want %v", err, ErrMethodNotAllowed)
	}
	select {
	case payload := <-w.writes:
		t.Fatalf("rejected request reached Hermes: %s", payload)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestClientCallTimesOutWhenConnectedGatewayStopsResponding(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newFakeWire()
	client := newClient(ctx, fakeConnector{wire: w}, clientOptions{reconnectDelay: time.Millisecond, callTimeout: 20 * time.Millisecond})
	defer client.Close()
	w.reads <- []byte(`{"jsonrpc":"2.0","method":"event","params":{"type":"gateway.ready","payload":{}}}`)
	connected, waitCancel := context.WithTimeout(ctx, time.Second)
	defer waitCancel()
	if err := client.WaitConnected(connected); err != nil {
		t.Fatal(err)
	}

	err := client.Call(ctx, "session.status", map[string]any{"session_id": "runtime"}, &struct{}{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unresponsive RPC error = %v, want deadline exceeded", err)
	}
	client.mu.Lock()
	pending := len(client.pending)
	client.mu.Unlock()
	if pending != 0 {
		t.Fatalf("timed-out RPC retained %d pending correlations", pending)
	}
}
