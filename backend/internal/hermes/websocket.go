package hermes

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type webSocketConnector struct {
	url    string
	header http.Header
}

func newWebSocketConnector(rawURL, token string) (*webSocketConnector, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return nil, errors.New("Hermes gateway URL must use ws or wss")
	}
	if token != "" {
		query := parsed.Query()
		query.Set("token", token)
		parsed.RawQuery = query.Encode()
	}
	header := make(http.Header)
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
		header.Set("X-Hermes-Session-Token", token)
	}
	return &webSocketConnector{url: parsed.String(), header: header}, nil
}

func (c *webSocketConnector) Connect(ctx context.Context) (wire, error) {
	connection, _, err := websocket.DefaultDialer.DialContext(ctx, c.url, c.header)
	if err != nil {
		return nil, err
	}
	return &webSocketWire{connection: connection}, nil
}

type webSocketWire struct {
	connection *websocket.Conn
	writeMu    sync.Mutex
}

func (w *webSocketWire) Read(context.Context) ([]byte, error) {
	_, payload, err := w.connection.ReadMessage()
	return payload, err
}

func (w *webSocketWire) Write(ctx context.Context, payload []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	if err := w.connection.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer w.connection.SetWriteDeadline(time.Time{})
	return w.connection.WriteMessage(websocket.TextMessage, payload)
}

func (w *webSocketWire) Close() error { return w.connection.Close() }
