package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrDisconnected     = errors.New("Hermes gateway disconnected")
	ErrMethodNotAllowed = errors.New("Hermes JSON-RPC method is not allowlisted")
)

var allowedMethods = map[string]struct{}{
	"clarify.respond":   {},
	"prompt.submit":     {},
	"session.create":    {},
	"session.history":   {},
	"session.interrupt": {},
	"session.resume":    {},
	"session.status":    {},
}

type wire interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
	Close() error
}

type connector interface {
	Connect(context.Context) (wire, error)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("Hermes RPC %d: %s", e.Code, e.Message)
}

type Event struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	Payload   json.RawMessage `json:"payload"`
}

type clientOptions struct {
	reconnectDelay time.Duration
	callTimeout    time.Duration
}

type client struct {
	ctx       context.Context
	cancel    context.CancelFunc
	connector connector
	options   clientOptions

	mu         sync.Mutex
	connection wire
	connected  bool
	changed    chan struct{}
	pending    map[string]chan rpcResponse
	nextID     uint64
	events     chan Event
	closeOnce  sync.Once
}

func newClient(parent context.Context, connectionFactory connector, options clientOptions) *client {
	ctx, cancel := context.WithCancel(parent)
	if options.reconnectDelay <= 0 {
		options.reconnectDelay = 250 * time.Millisecond
	}
	if options.callTimeout <= 0 {
		options.callTimeout = 30 * time.Second
	}
	c := &client{
		ctx:       ctx,
		cancel:    cancel,
		connector: connectionFactory,
		options:   options,
		changed:   make(chan struct{}),
		pending:   map[string]chan rpcResponse{},
		events:    make(chan Event, 256),
	}
	go c.run()
	return c
}

func (c *client) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		c.cancel()
		c.mu.Lock()
		if c.connection != nil {
			closeErr = c.connection.Close()
		}
		c.connection = nil
		c.setConnectedLocked(false)
		c.failPendingLocked()
		c.mu.Unlock()
	})
	return closeErr
}

func (c *client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

func (c *client) Events() <-chan Event { return c.events }

func (c *client) WaitConnected(ctx context.Context) error {
	for {
		c.mu.Lock()
		if c.connected {
			c.mu.Unlock()
			return nil
		}
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return ErrDisconnected
		case <-changed:
		}
	}
}

func (c *client) Call(ctx context.Context, method string, params any, target any) error {
	if _, allowed := allowedMethods[method]; !allowed {
		return ErrMethodNotAllowed
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.callTimeout)
		defer cancel()
	}
	if err := c.WaitConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	c.nextID++
	id := strconv.FormatUint(c.nextID, 10)
	responseChannel := make(chan rpcResponse, 1)
	c.pending[id] = responseChannel
	connection := c.connection
	c.mu.Unlock()

	request := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	payload, err := json.Marshal(request)
	if err != nil {
		c.removePending(id)
		return err
	}
	if connection == nil {
		c.removePending(id)
		return ErrDisconnected
	}
	if err := connection.Write(ctx, payload); err != nil {
		c.removePending(id)
		return fmt.Errorf("write Hermes RPC: %w", err)
	}

	select {
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.ctx.Done():
		c.removePending(id)
		return ErrDisconnected
	case response := <-responseChannel:
		if response.Error != nil {
			return response.Error
		}
		if target == nil || len(response.Result) == 0 || string(response.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(response.Result, target); err != nil {
			return fmt.Errorf("decode Hermes RPC result: %w", err)
		}
		return nil
	}
}

func (c *client) run() {
	defer close(c.events)
	delay := c.options.reconnectDelay
	for {
		connection, err := c.connector.Connect(c.ctx)
		if err != nil {
			if !waitContext(c.ctx, delay) {
				return
			}
			if delay < 10*time.Second {
				delay *= 2
				if delay > 10*time.Second {
					delay = 10 * time.Second
				}
			}
			continue
		}
		delay = c.options.reconnectDelay
		c.mu.Lock()
		c.connection = connection
		c.setConnectedLocked(true)
		c.mu.Unlock()

		err = c.readLoop(connection)
		_ = connection.Close()
		c.mu.Lock()
		if c.connection == connection {
			c.connection = nil
			c.setConnectedLocked(false)
			c.failPendingLocked()
		}
		c.mu.Unlock()
		if c.ctx.Err() != nil {
			return
		}
		if !waitContext(c.ctx, delay) {
			return
		}
	}
}

func (c *client) readLoop(connection wire) error {
	for {
		payload, err := connection.Read(c.ctx)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(payload), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var envelope struct {
				ID     string `json:"id"`
				Method string `json:"method"`
				Params *Event `json:"params"`
			}
			if err := json.Unmarshal([]byte(line), &envelope); err != nil {
				continue
			}
			if envelope.Method == "event" && envelope.Params != nil {
				select {
				case c.events <- *envelope.Params:
				case <-c.ctx.Done():
					return c.ctx.Err()
				}
				continue
			}
			if envelope.ID == "" {
				continue
			}
			var response rpcResponse
			if err := json.Unmarshal([]byte(line), &response); err != nil {
				continue
			}
			c.mu.Lock()
			responseChannel := c.pending[response.ID]
			delete(c.pending, response.ID)
			c.mu.Unlock()
			if responseChannel != nil {
				responseChannel <- response
			}
		}
	}
}

func (c *client) removePending(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *client) setConnectedLocked(connected bool) {
	if c.connected == connected {
		return
	}
	c.connected = connected
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *client) failPendingLocked() {
	for id, responseChannel := range c.pending {
		delete(c.pending, id)
		responseChannel <- rpcResponse{Error: &RPCError{Code: -1, Message: ErrDisconnected.Error()}}
	}
}

func waitContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
