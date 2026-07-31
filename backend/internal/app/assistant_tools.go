package app

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"viki/internal/model"
	"viki/internal/security"
	"viki/internal/store"
)

func (s *Server) internalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/hermes/tools/{tool}", s.handleHermesTool)
	return s.recover(s.securityHeaders(mux))
}

func (s *Server) handleHermesTool(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.authorizeHermesToolRequest(request) {
		writeError(w, http.StatusUnauthorized, "invalid_service_credential", "Service credential is not valid.")
		return
	}
	mode := normalizeHermesProfile(request.Header.Get("X-Hermes-Profile"))
	sessionID := strings.TrimSpace(request.Header.Get("X-Hermes-Session-ID"))
	if (mode != model.AssistantQA && mode != model.AssistantEdit) || sessionID == "" {
		writeError(w, http.StatusForbidden, "invalid_hermes_identity", "Hermes identity is not active.")
		return
	}
	turn, active := s.assistant.activeGrant(mode, sessionID)
	if !active {
		writeError(w, http.StatusForbidden, "inactive_hermes_turn", "Hermes session has no active Viki turn.")
		return
	}
	conversation, err := s.repository.AssistantConversation(request.Context(), turn.OrganizationID, turn.UserID, turn.ConversationID)
	if err != nil || !assistantBindingMatches(conversation, turn, sessionID) {
		writeError(w, http.StatusForbidden, "invalid_hermes_binding", "Hermes session is not bound to this Viki conversation.")
		return
	}
	tool := strings.TrimSpace(request.PathValue("tool"))
	if !toolAllowed(mode, tool) {
		writeError(w, http.StatusForbidden, "tool_not_allowed", "Tool is not allowed for this Hermes profile.")
		return
	}

	switch tool {
	case "search_viki":
		s.handleHermesSearch(w, request, conversation)
	case "get_viki_page":
		s.handleHermesGetPage(w, request, conversation)
	case "get_viki_revision":
		s.handleHermesGetRevision(w, request, conversation)
	case "propose_viki_changeset":
		s.handleHermesProposeChanges(w, request, conversation, turn)
	default:
		writeError(w, http.StatusForbidden, "tool_not_allowed", "Tool is not allowed for this Hermes profile.")
	}
}

func (s *Server) authorizeHermesToolRequest(request *http.Request) bool {
	expected := strings.TrimSpace(s.options.HermesToolToken)
	if expected == "" {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
	if provided == "" {
		return false
	}
	expectedHash := security.HashToken(expected)
	providedHash := security.HashToken(provided)
	return subtle.ConstantTimeCompare(expectedHash, providedHash) == 1
}

func (s *Server) handleHermesSearch(w http.ResponseWriter, request *http.Request, conversation model.AssistantConversation) {
	var input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_query", "query is required")
		return
	}
	if input.Limit <= 0 || input.Limit > 20 {
		input.Limit = 10
	}
	documents, err := s.repository.Retrieve(request.Context(), conversation.OrganizationID, input.Query, true, input.Limit)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": map[string]any{"documents": documents}})
}

func (s *Server) handleHermesGetPage(w http.ResponseWriter, request *http.Request, conversation model.AssistantConversation) {
	var input struct {
		PageID string `json:"pageId"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	detail, err := s.repository.PageDetail(request.Context(), conversation.OrganizationID, strings.TrimSpace(input.PageID))
	if err != nil {
		s.handleError(w, err)
		return
	}
	result := map[string]any{"page": detail.Page}
	if detail.AcceptedRevision != nil {
		result["acceptedRevision"] = detail.AcceptedRevision
	}
	if detail.DraftRevision != nil {
		result["draftRevision"] = detail.DraftRevision
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) handleHermesGetRevision(w http.ResponseWriter, request *http.Request, conversation model.AssistantConversation) {
	var input struct {
		RevisionID string `json:"revisionId"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	revision, err := s.repository.Revision(request.Context(), conversation.OrganizationID, strings.TrimSpace(input.RevisionID))
	if err != nil {
		s.handleError(w, err)
		return
	}
	detail, err := s.repository.PageDetail(request.Context(), conversation.OrganizationID, revision.PageID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	accepted := detail.Page.AcceptedRevisionID != nil && *detail.Page.AcceptedRevisionID == revision.ID
	currentDraft := detail.Page.LatestDraftRevisionID != nil && *detail.Page.LatestDraftRevisionID == revision.ID
	if !accepted && !currentDraft {
		s.handleError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": revision})
}

func (s *Server) handleHermesProposeChanges(w http.ResponseWriter, request *http.Request, conversation model.AssistantConversation, turn *assistantTurn) {
	var changeSet model.AIChangeSet
	if !decodeJSON(w, request, &changeSet) {
		return
	}
	if strings.TrimSpace(changeSet.Clarification) != "" || len(changeSet.Operations) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_change_set", "A draft change set must contain at least one operation and no clarification.")
		return
	}
	proposal, err := s.repository.StageAssistantDraftProposal(request.Context(), conversation.OrganizationID, conversation.UserID, model.AssistantMutationContext{
		ConversationID:  conversation.ID,
		TurnID:          turn.ID,
		HermesProfile:   "viki-edit",
		HermesSessionID: turn.StoredID,
	}, changeSet)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": map[string]any{"proposal": proposal}})
}

func assistantBindingMatches(conversation model.AssistantConversation, turn *assistantTurn, presentedSessionID string) bool {
	storedID := conversationSessionID(conversation, turn.Mode)
	if storedID == nil || *storedID != turn.StoredID {
		return false
	}
	return presentedSessionID == turn.StoredID
}

func normalizeHermesProfile(value string) model.AssistantMode {
	switch strings.TrimSpace(value) {
	case "qa", "viki-qa":
		return model.AssistantQA
	case "edit", "viki-edit":
		return model.AssistantEdit
	default:
		return ""
	}
}
