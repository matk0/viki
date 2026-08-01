package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"viki/internal/hermes"
	"viki/internal/model"
	"viki/internal/store"
)

var assistantKeepaliveInterval = 15 * time.Second

func (s *Server) assistantStatus(w http.ResponseWriter, _ *http.Request, _ authState) {
	status := s.assistant.status()
	profile := func(mode model.AssistantMode) map[string]any {
		value := status.Profiles[string(mode)]
		result := map[string]any{
			"mode": mode, "connected": value.Connected, "configured": value.Configured,
			"ready": value.Connected && value.Configured,
		}
		if !value.Configured {
			result["message"] = "Profil asistenta nie je nakonfigurovaný."
		} else if !value.Connected {
			result["message"] = "Hermes profil momentálne nie je dostupný."
		}
		return result
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": status.Available,
		"qa":        profile(model.AssistantQA),
		"edit":      profile(model.AssistantEdit),
	})
}

func (s *Server) listAssistantConversations(w http.ResponseWriter, request *http.Request, auth authState) {
	conversations, err := s.repository.ListAssistantConversations(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	var wait sync.WaitGroup
	semaphore := make(chan struct{}, 4)
	for index := range conversations {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			if !acquireAssistantHistorySlot(request.Context(), semaphore) {
				return
			}
			defer func() { <-semaphore }()
			ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
			defer cancel()
			if err := s.loadAssistantHistory(ctx, &conversations[index]); err != nil && !errors.Is(err, hermes.ErrUnavailable) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				s.logger.Debug("loading Hermes assistant summary failed", "conversationId", conversations[index].ID, "error", err)
			}
			conversations[index].Messages = nil
			s.enrichAssistantSummary(&conversations[index])
		}(index)
	}
	wait.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"conversations": conversations})
}

func acquireAssistantHistorySlot(ctx context.Context, semaphore chan<- struct{}) bool {
	select {
	case semaphore <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Server) createAssistantConversation(w http.ResponseWriter, request *http.Request, auth authState) {
	input := struct {
		PrimaryMode model.AssistantMode `json:"primaryMode"`
	}{PrimaryMode: model.AssistantQA}
	if request.ContentLength != 0 && !decodeJSON(w, request, &input) {
		return
	}
	if input.PrimaryMode != model.AssistantQA && input.PrimaryMode != model.AssistantEdit {
		writeError(w, http.StatusUnprocessableEntity, "invalid_mode", "Vyberte režim otázok alebo úprav.")
		return
	}
	conversation, err := s.repository.CreateAssistantConversation(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, input.PrimaryMode)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.enrichAssistantSummary(&conversation)
	conversation.Messages = []model.AssistantMessage{}
	writeJSON(w, http.StatusCreated, conversation)
}

func (s *Server) assistantConversationDetail(w http.ResponseWriter, request *http.Request, auth authState) {
	conversation, ok := s.authorizedAssistantConversation(w, request, auth)
	if !ok {
		return
	}
	if err := s.loadAssistantHistory(request.Context(), &conversation); err != nil {
		if !errors.Is(err, hermes.ErrUnavailable) {
			s.logger.Warn("loading Hermes assistant history failed", "conversationId", conversation.ID, "error", err)
		}
	}
	s.enrichAssistantSummary(&conversation)
	conversation.Clarification = s.assistant.clarification(conversation.ID)
	if conversation.Messages == nil {
		conversation.Messages = []model.AssistantMessage{}
	}
	writeJSON(w, http.StatusOK, conversation)
}

func (s *Server) updateAssistantConversation(w http.ResponseWriter, request *http.Request, auth authState) {
	conversation, ok := s.authorizedAssistantConversation(w, request, auth)
	if !ok {
		return
	}
	var input struct {
		PrimaryMode *model.AssistantMode `json:"primaryMode"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	if input.PrimaryMode == nil {
		writeError(w, http.StatusBadRequest, "invalid_settings", "Zadajte nastavenie, ktoré chcete zmeniť.")
		return
	}
	if input.PrimaryMode != nil && *input.PrimaryMode != model.AssistantQA && *input.PrimaryMode != model.AssistantEdit {
		writeError(w, http.StatusUnprocessableEntity, "invalid_mode", "Vyberte režim otázok alebo úprav.")
		return
	}
	if s.assistant.state(conversation) == model.AssistantStateRunning || s.assistant.state(conversation) == model.AssistantStateAwaitingClarification {
		writeError(w, http.StatusConflict, "assistant_busy", "Počkajte na dokončenie aktuálnej správy alebo ju zastavte.")
		return
	}
	if err := s.repository.SetAssistantPrimaryMode(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, conversation.ID, *input.PrimaryMode); err != nil {
		s.handleError(w, err)
		return
	}
	conversation.PrimaryMode = *input.PrimaryMode
	conversation, err := s.repository.AssistantConversation(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, conversation.ID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.enrichAssistantSummary(&conversation)
	writeJSON(w, http.StatusOK, conversation)
}

func (s *Server) submitAssistantMessage(w http.ResponseWriter, request *http.Request, auth authState) {
	conversation, ok := s.authorizedAssistantConversation(w, request, auth)
	if !ok {
		return
	}
	var input struct {
		Content string              `json:"content"`
		Mode    model.AssistantMode `json:"mode"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" || len([]rune(input.Content)) > 12000 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_message", "Správa musí obsahovať 1 až 12 000 znakov.")
		return
	}
	if input.Mode != model.AssistantQA && input.Mode != model.AssistantEdit {
		writeError(w, http.StatusUnprocessableEntity, "invalid_mode", "Vyberte režim otázok alebo úprav.")
		return
	}
	turn, err := s.assistant.submit(request.Context(), conversation, input.Mode, input.Content)
	if err != nil {
		s.handleAssistantError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"turnId": turn.ID, "mode": turn.Mode})
}

func (s *Server) streamAssistantEvents(w http.ResponseWriter, request *http.Request, auth authState) {
	conversation, ok := s.authorizedAssistantConversation(w, request, auth)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unavailable", "Streamovanie nie je dostupné.")
		return
	}
	rawLastID := strings.TrimSpace(request.Header.Get("Last-Event-ID"))
	lastID, parseErr := strconv.ParseUint(rawLastID, 10, 64)
	replay, events, unsubscribe := s.assistant.stream(conversation.ID).subscribe(lastID, rawLastID != "" && parseErr == nil)
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	emit := func(event assistantPublicEvent) error {
		encoded, err := json.Marshal(event.Data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, encoded); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	for _, event := range replay {
		if err := emit(event); err != nil {
			return
		}
	}
	flusher.Flush()
	keepalive := time.NewTicker(assistantKeepaliveInterval)
	defer keepalive.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case event, open := <-events:
			if !open || emit(event) != nil {
				return
			}
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) stopAssistantTurn(w http.ResponseWriter, request *http.Request, auth authState) {
	conversation, ok := s.authorizedAssistantConversation(w, request, auth)
	if !ok {
		return
	}
	if _, err := s.assistant.stop(request.Context(), conversation.ID); err != nil {
		s.handleAssistantError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) respondAssistantClarification(w http.ResponseWriter, request *http.Request, auth authState) {
	conversation, ok := s.authorizedAssistantConversation(w, request, auth)
	if !ok {
		return
	}
	requestID, ok := requirePathID(w, request, "requestID")
	if !ok {
		return
	}
	var input struct {
		Answer string `json:"answer"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	input.Answer = strings.TrimSpace(input.Answer)
	if input.Answer == "" || len([]rune(input.Answer)) > 12000 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_answer", "Odpoveď musí obsahovať 1 až 12 000 znakov.")
		return
	}
	if _, err := s.assistant.respondClarification(request.Context(), conversation.ID, requestID, input.Answer); err != nil {
		s.handleAssistantError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) authorizedAssistantConversation(w http.ResponseWriter, request *http.Request, auth authState) (model.AssistantConversation, bool) {
	conversationID, ok := requirePathID(w, request, "conversationID")
	if !ok {
		return model.AssistantConversation{}, false
	}
	conversation, err := s.repository.AssistantConversation(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, conversationID)
	if err != nil {
		s.handleError(w, err)
		return model.AssistantConversation{}, false
	}
	return conversation, true
}

func (s *Server) enrichAssistantSummary(conversation *model.AssistantConversation) {
	conversation.State = s.assistant.state(*conversation)
	if strings.TrimSpace(conversation.Title) == "" {
		conversation.Title = "Nový rozhovor"
	}
}

func (s *Server) loadAssistantHistory(ctx context.Context, conversation *model.AssistantConversation) error {
	if s.gateway == nil {
		return hermes.ErrUnavailable
	}
	var qaMessages, editMessages []model.AssistantMessage
	var errs []error
	for _, item := range []struct {
		mode        model.AssistantMode
		storedID    *string
		destination *[]model.AssistantMessage
	}{
		{model.AssistantQA, conversation.QASessionID, &qaMessages},
		{model.AssistantEdit, conversation.EditSessionID, &editMessages},
	} {
		if item.storedID == nil || *item.storedID == "" {
			continue
		}
		session, updated, err := s.assistant.ensureSession(ctx, *conversation, item.mode)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		*conversation = updated
		history, err := s.gateway.History(ctx, item.mode, session.RuntimeID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		*item.destination = visibleHistory(s.assistant, item.mode, history, conversation.CreatedAt)
	}
	conversation.Messages = mergeAssistantHistory(qaMessages, editMessages)
	for _, message := range conversation.Messages {
		if message.Role == "user" {
			conversation.Title = truncateRunes(strings.TrimSpace(message.Content), 80)
			break
		}
	}
	if receipts, err := s.repository.AssistantDraftReceipts(ctx, conversation.OrganizationID, conversation.ID); err == nil {
		conversation.Messages = attachDraftReceipts(conversation.Messages, receipts)
	} else {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func attachDraftReceipts(messages []model.AssistantMessage, receipts map[string][]model.AssistantDraftReceipt) []model.AssistantMessage {
	assistantIndices := map[string][]int{}
	existing := map[string]map[string]struct{}{}
	for index := range messages {
		if messages[index].Role != "assistant" {
			continue
		}
		turnID := assistantMessageTurnID(messages[index].ID)
		if turnID == "" {
			continue
		}
		assistantIndices[turnID] = append(assistantIndices[turnID], index)
		if existing[turnID] == nil {
			existing[turnID] = map[string]struct{}{}
		}
		for _, draft := range messages[index].Drafts {
			if draft.RevisionID != "" {
				existing[turnID][draft.RevisionID] = struct{}{}
			}
		}
	}
	for turnID, values := range receipts {
		if len(values) == 0 {
			continue
		}
		indices := assistantIndices[turnID]
		if len(indices) > 0 {
			target := indices[len(indices)-1]
			for _, receipt := range values {
				if receipt.RevisionID == "" {
					continue
				}
				if _, duplicate := existing[turnID][receipt.RevisionID]; duplicate {
					continue
				}
				messages[target].Drafts = append(messages[target].Drafts, receipt)
				existing[turnID][receipt.RevisionID] = struct{}{}
			}
			sort.Slice(messages[target].Drafts, func(i, j int) bool {
				return messages[target].Drafts[i].RevisionID < messages[target].Drafts[j].RevisionID
			})
			continue
		}
		for _, message := range messages {
			if message.Role == "user" && strings.HasPrefix(message.ID, turnID+"-") {
				messages = append(messages, model.AssistantMessage{
					ID: turnID + "-assistant-receipt", Role: "assistant", Mode: message.Mode,
					Content: "Vytvoril som drafty vo viki.", Citations: []model.Citation{},
					Drafts: append([]model.AssistantDraftReceipt(nil), values...), CreatedAt: message.CreatedAt.Add(time.Nanosecond),
				})
				break
			}
		}
	}
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
	return messages
}

func assistantMessageTurnID(messageID string) string {
	index := strings.Index(messageID, "-assistant")
	if index <= 0 {
		return ""
	}
	return messageID[:index]
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func (s *Server) handleAssistantError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAssistantTurnActive):
		writeError(w, http.StatusConflict, "assistant_busy", "Počkajte na dokončenie aktuálnej správy alebo ju zastavte.")
	case errors.Is(err, errAssistantTurnNotActive):
		writeError(w, http.StatusConflict, "assistant_idle", "Nie je spustená žiadna správa.")
	case errors.Is(err, errAssistantClarification):
		writeError(w, http.StatusConflict, "clarification_mismatch", "Táto otázka na spresnenie už nie je aktívna.")
	case errors.Is(err, errAssistantCommandForbidden):
		writeError(w, http.StatusUnprocessableEntity, "management_command_forbidden", "Príkazy na správu Hermes nie sú vo viki povolené.")
	case errors.Is(err, hermes.ErrUnavailable), errors.Is(err, hermes.ErrDisconnected):
		writeError(w, http.StatusServiceUnavailable, "assistant_unavailable", "viki asistent momentálne nie je dostupný.")
	case errors.Is(err, store.ErrNotFound):
		s.handleError(w, err)
	default:
		s.logger.Error("assistant request failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "assistant_unavailable", "viki asistent momentálne nie je dostupný.")
	}
}
