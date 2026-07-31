package hermes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"viki/internal/model"
)

var ErrUnavailable = errors.New("Hermes gateway unavailable")

type ProfileStatus struct {
	Connected  bool `json:"connected"`
	Configured bool `json:"configured"`
}

type GatewayStatus struct {
	Available bool                     `json:"available"`
	Profiles  map[string]ProfileStatus `json:"profiles"`
}

type Session struct {
	RuntimeID string
	StoredID  string
}

type SessionState struct {
	Running bool
	Status  string
}

type HistoryMessage struct {
	ID        string          `json:"id,omitempty"`
	Role      string          `json:"role"`
	Text      string          `json:"text,omitempty"`
	Name      string          `json:"name,omitempty"`
	Context   json.RawMessage `json:"context,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	CreatedAt time.Time       `json:"created_at,omitempty"`
}

type Gateway interface {
	Status() GatewayStatus
	CreateSession(context.Context, model.AssistantMode) (Session, error)
	ResumeSession(context.Context, model.AssistantMode, string) (Session, error)
	History(context.Context, model.AssistantMode, string) ([]HistoryMessage, error)
	SessionStatus(context.Context, model.AssistantMode, string) (SessionState, error)
	Submit(context.Context, model.AssistantMode, string, string) error
	Interrupt(context.Context, model.AssistantMode, string) error
	RespondClarification(context.Context, model.AssistantMode, string, string, string) error
	Events(model.AssistantMode) <-chan Event
	Close() error
}

type ProfileConfig struct {
	URL        string
	Token      string
	Configured bool
}

type ManagerConfig struct {
	QA   ProfileConfig
	Edit ProfileConfig
}

type Manager struct {
	clients    map[model.AssistantMode]*client
	configured map[model.AssistantMode]bool
}

func NewManager(ctx context.Context, config ManagerConfig) (*Manager, error) {
	manager := &Manager{clients: map[model.AssistantMode]*client{}, configured: map[model.AssistantMode]bool{}}
	for mode, profile := range map[model.AssistantMode]ProfileConfig{
		model.AssistantQA:   config.QA,
		model.AssistantEdit: config.Edit,
	} {
		if profile.URL == "" {
			continue
		}
		manager.configured[mode] = profile.Configured
		connector, err := newWebSocketConnector(profile.URL, profile.Token)
		if err != nil {
			_ = manager.Close()
			return nil, fmt.Errorf("configure Hermes %s profile: %w", mode, err)
		}
		manager.clients[mode] = newClient(ctx, connector, clientOptions{})
	}
	return manager, nil
}

func (m *Manager) Status() GatewayStatus {
	profiles := map[string]ProfileStatus{}
	available := true
	for _, mode := range []model.AssistantMode{model.AssistantQA, model.AssistantEdit} {
		profile := ProfileStatus{}
		if client := m.clients[mode]; client != nil {
			profile.Configured = m.configured[mode]
			profile.Connected = client.Connected()
		}
		profiles[string(mode)] = profile
		available = available && profile.Configured && profile.Connected
	}
	return GatewayStatus{Available: available, Profiles: profiles}
}

func (m *Manager) CreateSession(ctx context.Context, mode model.AssistantMode) (Session, error) {
	client, err := m.profile(mode)
	if err != nil {
		return Session{}, err
	}
	var response struct {
		SessionID       string `json:"session_id"`
		StoredSessionID string `json:"stored_session_id"`
		SessionKey      string `json:"session_key"`
	}
	if err := client.Call(ctx, "session.create", map[string]any{
		"source":              "viki",
		"close_on_disconnect": false,
		"title":               "viki asistent",
	}, &response); err != nil {
		return Session{}, err
	}
	storedID := response.StoredSessionID
	if storedID == "" {
		storedID = response.SessionKey
	}
	if response.SessionID == "" || storedID == "" {
		return Session{}, errors.New("Hermes session.create omitted a session identifier")
	}
	return Session{RuntimeID: response.SessionID, StoredID: storedID}, nil
}

func (m *Manager) ResumeSession(ctx context.Context, mode model.AssistantMode, storedID string) (Session, error) {
	client, err := m.profile(mode)
	if err != nil {
		return Session{}, err
	}
	var response struct {
		SessionID  string `json:"session_id"`
		SessionKey string `json:"session_key"`
		Resumed    string `json:"resumed"`
	}
	if err := client.Call(ctx, "session.resume", map[string]any{
		"session_id":          storedID,
		"source":              "viki",
		"close_on_disconnect": false,
	}, &response); err != nil {
		return Session{}, err
	}
	durableID := response.SessionKey
	if durableID == "" {
		durableID = response.Resumed
	}
	if durableID == "" {
		durableID = storedID
	}
	if response.SessionID == "" {
		return Session{}, errors.New("Hermes session.resume omitted its runtime session identifier")
	}
	return Session{RuntimeID: response.SessionID, StoredID: durableID}, nil
}

func (m *Manager) History(ctx context.Context, mode model.AssistantMode, runtimeID string) ([]HistoryMessage, error) {
	client, err := m.profile(mode)
	if err != nil {
		return nil, err
	}
	var response struct {
		Messages []HistoryMessage `json:"messages"`
	}
	if err := client.Call(ctx, "session.history", map[string]any{"session_id": runtimeID}, &response); err != nil {
		return nil, err
	}
	return response.Messages, nil
}

func (m *Manager) SessionStatus(ctx context.Context, mode model.AssistantMode, runtimeID string) (SessionState, error) {
	client, err := m.profile(mode)
	if err != nil {
		return SessionState{}, err
	}
	var response struct {
		Output string `json:"output"`
	}
	if err := client.Call(ctx, "session.status", map[string]any{"session_id": runtimeID}, &response); err != nil {
		return SessionState{}, err
	}
	running := strings.Contains(strings.ToLower(response.Output), "agent running: yes")
	status := "idle"
	if running {
		status = "streaming"
	}
	return SessionState{Running: running, Status: status}, nil
}

func (m *Manager) Submit(ctx context.Context, mode model.AssistantMode, runtimeID, text string) error {
	client, err := m.profile(mode)
	if err != nil {
		return err
	}
	var response struct {
		Status string `json:"status"`
	}
	return client.Call(ctx, "prompt.submit", map[string]any{"session_id": runtimeID, "text": text}, &response)
}

func (m *Manager) Interrupt(ctx context.Context, mode model.AssistantMode, runtimeID string) error {
	client, err := m.profile(mode)
	if err != nil {
		return err
	}
	return client.Call(ctx, "session.interrupt", map[string]any{"session_id": runtimeID}, &struct{}{})
}

func (m *Manager) RespondClarification(ctx context.Context, mode model.AssistantMode, runtimeID, requestID, answer string) error {
	client, err := m.profile(mode)
	if err != nil {
		return err
	}
	return client.Call(ctx, "clarify.respond", map[string]any{
		"session_id": runtimeID,
		"request_id": requestID,
		"answer":     answer,
	}, &struct{}{})
}

func (m *Manager) Events(mode model.AssistantMode) <-chan Event {
	if client := m.clients[mode]; client != nil {
		return client.Events()
	}
	channel := make(chan Event)
	close(channel)
	return channel
}

func (m *Manager) Close() error {
	var errs []error
	for _, client := range m.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) profile(mode model.AssistantMode) (*client, error) {
	client := m.clients[mode]
	if client == nil || !m.configured[mode] || !client.Connected() {
		return nil, ErrUnavailable
	}
	return client, nil
}

type FakeGateway struct {
	mu       sync.Mutex
	Sessions map[model.AssistantMode]map[string][]HistoryMessage
	EventsBy map[model.AssistantMode]chan Event
	Next     int
	Online   bool
}

func NewFakeGateway() *FakeGateway {
	return &FakeGateway{
		Sessions: map[model.AssistantMode]map[string][]HistoryMessage{
			model.AssistantQA:   {},
			model.AssistantEdit: {},
		},
		EventsBy: map[model.AssistantMode]chan Event{
			model.AssistantQA:   make(chan Event, 64),
			model.AssistantEdit: make(chan Event, 64),
		},
		Online: true,
	}
}

func (g *FakeGateway) Status() GatewayStatus {
	profiles := map[string]ProfileStatus{
		"qa":   {Connected: g.Online, Configured: true},
		"edit": {Connected: g.Online, Configured: true},
	}
	return GatewayStatus{Available: g.Online, Profiles: profiles}
}

func (g *FakeGateway) CreateSession(_ context.Context, mode model.AssistantMode) (Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.Online {
		return Session{}, ErrUnavailable
	}
	g.Next++
	storedID := fmt.Sprintf("%s-stored-%d", mode, g.Next)
	g.Sessions[mode][storedID] = []HistoryMessage{}
	return Session{RuntimeID: "runtime-" + storedID, StoredID: storedID}, nil
}

func (g *FakeGateway) ResumeSession(_ context.Context, mode model.AssistantMode, storedID string) (Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.Online {
		return Session{}, ErrUnavailable
	}
	if _, exists := g.Sessions[mode][storedID]; !exists {
		return Session{}, errors.New("fake Hermes session not found")
	}
	return Session{RuntimeID: "runtime-" + storedID, StoredID: storedID}, nil
}

func (g *FakeGateway) History(_ context.Context, mode model.AssistantMode, runtimeID string) ([]HistoryMessage, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	storedID := trimFakeRuntimeID(runtimeID)
	return append([]HistoryMessage(nil), g.Sessions[mode][storedID]...), nil
}

func (g *FakeGateway) SessionStatus(context.Context, model.AssistantMode, string) (SessionState, error) {
	return SessionState{Status: "idle"}, nil
}

func (g *FakeGateway) Submit(_ context.Context, mode model.AssistantMode, runtimeID, text string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	storedID := trimFakeRuntimeID(runtimeID)
	g.Sessions[mode][storedID] = append(g.Sessions[mode][storedID], HistoryMessage{Role: "user", Text: text})
	return nil
}

func (g *FakeGateway) Interrupt(context.Context, model.AssistantMode, string) error { return nil }

func (g *FakeGateway) RespondClarification(context.Context, model.AssistantMode, string, string, string) error {
	return nil
}

func (g *FakeGateway) Events(mode model.AssistantMode) <-chan Event { return g.EventsBy[mode] }
func (g *FakeGateway) Close() error                                 { return nil }

func (g *FakeGateway) Emit(mode model.AssistantMode, event Event) {
	g.EventsBy[mode] <- event
}

func trimFakeRuntimeID(runtimeID string) string {
	const prefix = "runtime-"
	if len(runtimeID) > len(prefix) && runtimeID[:len(prefix)] == prefix {
		return runtimeID[len(prefix):]
	}
	return runtimeID
}
