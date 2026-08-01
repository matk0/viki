package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

type fakeConnector struct{ wire wire }

func (c fakeConnector) Connect(context.Context) (wire, error) { return c.wire, nil }

type writeErrorWire struct {
	closed chan struct{}
}

func (w *writeErrorWire) Read(context.Context) ([]byte, error) {
	<-w.closed
	return nil, errors.New("closed")
}

func (w *writeErrorWire) Write(context.Context, []byte) error {
	return errors.New("write failed")
}

func (w *writeErrorWire) Close() error {
	select {
	case <-w.closed:
	default:
		close(w.closed)
	}
	return nil
}

type failingConnector struct{}

func (failingConnector) Connect(context.Context) (wire, error) {
	return nil, errors.New("connect failed")
}

type retryConnector struct {
	mu       sync.Mutex
	failures int
	wire     wire
}

type immediateReadErrorWire struct {
	closed chan struct{}
	once   sync.Once
}

type oneReadWire struct {
	payload []byte
	read    bool
}

func (w *oneReadWire) Read(context.Context) ([]byte, error) {
	if w.read {
		return nil, errors.New("closed")
	}
	w.read = true
	return w.payload, nil
}

func (*oneReadWire) Write(context.Context, []byte) error { return nil }
func (*oneReadWire) Close() error                        { return nil }

func (w *immediateReadErrorWire) Read(context.Context) ([]byte, error) {
	return nil, errors.New("read failed")
}

func (w *immediateReadErrorWire) Write(context.Context, []byte) error { return nil }

func (w *immediateReadErrorWire) Close() error {
	w.once.Do(func() { close(w.closed) })
	return nil
}

func (c *retryConnector) Connect(context.Context) (wire, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failures > 0 {
		c.failures--
		return nil, errors.New("connect failed")
	}
	return c.wire, nil
}

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

func TestClientCallCoversSafeFailureAndEmptyResponsePaths(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newFakeWire()
	client := newClient(ctx, fakeConnector{wire: w}, clientOptions{reconnectDelay: time.Millisecond})
	defer client.Close()
	connected, connectedCancel := context.WithTimeout(ctx, time.Second)
	defer connectedCancel()
	if err := client.WaitConnected(connected); err != nil {
		t.Fatal(err)
	}

	if err := client.Call(ctx, "session.status", make(chan int), &struct{}{}); err == nil {
		t.Fatal("unencodable request parameters were accepted")
	}

	tests := []struct {
		name     string
		response string
		target   any
		wantRPC  bool
		wantErr  bool
	}{
		{name: "rpc error", response: `{"jsonrpc":"2.0","id":"%s","error":{"code":-32000,"message":"failed"}}`, target: &struct{}{}, wantRPC: true},
		{name: "nil target", response: `{"jsonrpc":"2.0","id":"%s","result":{"ignored":true}}`, target: nil},
		{name: "empty result", response: `{"jsonrpc":"2.0","id":"%s"}`, target: &struct{}{}},
		{name: "null result", response: `{"jsonrpc":"2.0","id":"%s","result":null}`, target: &struct{}{}},
		{name: "invalid result", response: `{"jsonrpc":"2.0","id":"%s","result":"wrong shape"}`, target: &struct {
			Value string `json:"value"`
		}{}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := make(chan error, 1)
			go func() {
				result <- client.Call(ctx, "session.status", map[string]any{}, test.target)
			}()
			requestPayload := <-w.writes
			var request rpcRequest
			if err := json.Unmarshal(requestPayload, &request); err != nil {
				t.Fatal(err)
			}
			w.reads <- []byte(fmt.Sprintf(test.response, request.ID))
			err := <-result
			var rpcErr *RPCError
			if test.wantRPC && !errors.As(err, &rpcErr) {
				t.Fatalf("error = %v, want RPC error", err)
			}
			if test.wantErr && err == nil {
				t.Fatal("invalid response decoded successfully")
			}
			if !test.wantRPC && !test.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientCallHandlesDisconnectsBeforeAndDuringWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &client{
		ctx:       ctx,
		cancel:    cancel,
		options:   clientOptions{callTimeout: time.Second},
		connected: true,
		changed:   make(chan struct{}),
		pending:   map[string]chan rpcResponse{},
		events:    make(chan Event),
	}
	if err := client.Call(context.Background(), "session.status", nil, nil); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("missing connection error = %v", err)
	}

	writeFailure := &writeErrorWire{closed: make(chan struct{})}
	client.connection = writeFailure
	if err := client.Call(context.Background(), "session.status", nil, nil); err == nil || !strings.Contains(err.Error(), "write Hermes RPC") {
		t.Fatalf("write failure = %v", err)
	}

	w := newFakeWire()
	client.connection = w
	result := make(chan error, 1)
	go func() {
		result <- client.Call(context.Background(), "session.status", nil, nil)
	}()
	<-w.writes
	cancel()
	if err := <-result; !errors.Is(err, ErrDisconnected) {
		t.Fatalf("in-flight cancellation error = %v", err)
	}
}

func TestClientWaitAndReconnectStopWhenContextsEnd(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	client := newClient(parent, failingConnector{}, clientOptions{reconnectDelay: time.Millisecond})

	waitCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	if err := client.WaitConnected(waitCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation error = %v", err)
	}
	if err := client.Call(waitCtx, "session.status", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled call error = %v", err)
	}
	stop()
	if err := client.WaitConnected(context.Background()); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("client cancellation error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if waitContext(canceled, time.Hour) {
		t.Fatal("waitContext ignored cancellation")
	}
}

func TestClientRetriesConnectorFailuresAndCapsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := newFakeWire()
	connector := &retryConnector{failures: 2, wire: w}
	client := newClient(ctx, connector, clientOptions{reconnectDelay: time.Millisecond})
	defer client.Close()

	wait, waitCancel := context.WithTimeout(ctx, time.Second)
	defer waitCancel()
	if err := client.WaitConnected(wait); err != nil {
		t.Fatal(err)
	}
	if got := nextReconnectDelay(6 * time.Second); got != 10*time.Second {
		t.Fatalf("capped reconnect delay = %s", got)
	}
	if got := nextReconnectDelay(10 * time.Second); got != 10*time.Second {
		t.Fatalf("maximum reconnect delay grew to %s", got)
	}
}

func TestReadLoopDropsMalformedAndUncorrelatedResponses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &client{
		ctx:     ctx,
		cancel:  cancel,
		pending: map[string]chan rpcResponse{},
		events:  make(chan Event),
	}
	w := &oneReadWire{payload: []byte("{\"id\":\"unknown\",\"error\":\"bad\"}\n{\"id\":\"unknown\",\"result\":{}}")}
	if err := c.readLoop(w); err == nil {
		t.Fatal("closed wire did not terminate the read loop")
	}

	blockingWire := newFakeWire()
	blockingWire.reads <- []byte(`{"method":"event","params":{"type":"message.delta"}}`)
	cancel()
	if err := c.readLoop(blockingWire); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled event delivery error = %v", err)
	}
}

func TestClientStopsReconnectWaitAfterReadFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &immediateReadErrorWire{closed: make(chan struct{})}
	client := newClient(ctx, fakeConnector{wire: w}, clientOptions{reconnectDelay: time.Hour})

	select {
	case <-w.closed:
	case <-time.After(time.Second):
		t.Fatal("client did not close the failed connection")
	}
	cancel()
	select {
	case <-client.Events():
	case <-time.After(time.Second):
		t.Fatal("client did not stop its reconnect wait")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientInternalStateTransitionsFailPendingCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &client{
		ctx:       ctx,
		cancel:    cancel,
		connected: true,
		changed:   make(chan struct{}),
		pending:   map[string]chan rpcResponse{},
		events:    make(chan Event, 1),
	}
	c.setConnectedLocked(true)

	responseChannel := make(chan rpcResponse, 1)
	c.pending["1"] = responseChannel
	c.failPendingLocked()
	response := <-responseChannel
	if response.Error == nil || response.Error.Message != ErrDisconnected.Error() {
		t.Fatalf("pending response = %+v", response)
	}
	if len(c.pending) != 0 {
		t.Fatalf("pending calls were retained: %+v", c.pending)
	}
}
