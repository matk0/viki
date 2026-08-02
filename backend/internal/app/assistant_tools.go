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
	mux.HandleFunc("GET /internal/v1/development/pending", s.handleDevelopmentPending)
	return s.recover(s.securityHeaders(mux))
}

func (s *Server) handleHermesTool(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.authorizeHermesToolRequest(request) {
		writeError(w, http.StatusUnauthorized, "invalid_service_credential", "Service credential is not valid.")
		return
	}
	presentedProfile := strings.TrimSpace(request.Header.Get("X-Hermes-Profile"))
	sessionID := strings.TrimSpace(request.Header.Get("X-Hermes-Session-ID"))
	if presentedProfile == "developer" || presentedProfile == "viki-developer" {
		if sessionID == "" {
			writeError(w, http.StatusForbidden, "invalid_hermes_identity", "Hermes identity is not active.")
			return
		}
		tool := strings.TrimSpace(request.PathValue("tool"))
		if !developerToolAllowed(tool) {
			writeError(w, http.StatusForbidden, "tool_not_allowed", "Tool is not allowed for this Hermes profile.")
			return
		}
		s.handleDeveloperTool(w, request, tool)
		return
	}
	mode := normalizeHermesProfile(presentedProfile)
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
		s.handleHermesSearch(w, request, conversation, mode)
	case "get_viki_page":
		s.handleHermesGetPage(w, request, conversation)
	case "get_viki_revision":
		s.handleHermesGetRevision(w, request, conversation)
	case "apply_viki_draft_changeset":
		s.handleHermesApplyDraftChanges(w, request, conversation, turn)
	}
}

func (s *Server) handleDevelopmentPending(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.authorizeHermesToolRequest(request) {
		writeError(w, http.StatusUnauthorized, "invalid_service_credential", "Service credential is not valid.")
		return
	}
	queued, err := s.repository.HasQueuedScenarioDevelopment(request.Context())
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"wakeAgent": queued})
}

func developerToolAllowed(name string) bool {
	switch name {
	case "claim_next_scenario", "complete_scenario_development", "block_scenario_development":
		return true
	default:
		return false
	}
}

func (s *Server) handleDeveloperTool(w http.ResponseWriter, request *http.Request, tool string) {
	switch tool {
	case "claim_next_scenario":
		var input struct{}
		if !decodeJSON(w, request, &input) {
			return
		}
		task, err := s.repository.ClaimScenarioDevelopment(request.Context())
		if err != nil {
			s.handleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": task})
	case "complete_scenario_development":
		var input struct {
			Implementation string `json:"implementation"`
		}
		if !decodeJSON(w, request, &input) {
			return
		}
		input.Implementation = strings.TrimSpace(input.Implementation)
		if input.Implementation == "" {
			writeError(w, http.StatusUnprocessableEntity, "invalid_implementation", "implementation is required")
			return
		}
		receipt, err := s.target.Apply(request.Context(), input.Implementation)
		if err != nil {
			s.handleError(w, err)
			return
		}
		development, err := s.repository.CompleteScenarioDevelopment(request.Context(), receipt)
		if err != nil {
			s.handleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": development})
	case "block_scenario_development":
		var input struct {
			Reason string `json:"reason"`
		}
		if !decodeJSON(w, request, &input) {
			return
		}
		input.Reason = strings.TrimSpace(input.Reason)
		if input.Reason == "" {
			writeError(w, http.StatusUnprocessableEntity, "invalid_reason", "reason is required")
			return
		}
		development, err := s.repository.BlockScenarioDevelopment(request.Context(), input.Reason)
		if err != nil {
			s.handleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": development})
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

func (s *Server) handleHermesSearch(w http.ResponseWriter, request *http.Request, conversation model.AssistantConversation, mode model.AssistantMode) {
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
	result := map[string]any{"documents": documents}
	if mode == model.AssistantEdit {
		definitions, err := s.repository.ListStepDefinitions(request.Context(), conversation.OrganizationID, input.Query, nil)
		if err != nil {
			s.handleError(w, err)
			return
		}
		result["stepDefinitions"] = definitions
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
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
	if detail.ApprovedRevision != nil {
		result["approvedRevision"] = detail.ApprovedRevision
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
	approved := detail.Page.ApprovedRevisionID != nil && *detail.Page.ApprovedRevisionID == revision.ID
	currentDraft := detail.Page.LatestDraftRevisionID != nil && *detail.Page.LatestDraftRevisionID == revision.ID
	if !approved && !currentDraft {
		s.handleError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": revision})
}

func (s *Server) handleHermesApplyDraftChanges(w http.ResponseWriter, request *http.Request, conversation model.AssistantConversation, turn *assistantTurn) {
	var changeSet model.AIChangeSet
	if !decodeJSON(w, request, &changeSet) {
		return
	}
	if strings.TrimSpace(changeSet.Clarification) != "" || len(changeSet.Operations) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_change_set", "A draft change set must contain at least one operation and no clarification.")
		return
	}
	revisions, err := s.repository.ApplyAIChangeSet(request.Context(), conversation.OrganizationID, conversation.UserID, model.AssistantMutationContext{
		ConversationID:  conversation.ID,
		TurnID:          turn.ID,
		HermesProfile:   "viki-edit",
		HermesSessionID: turn.StoredID,
	}, changeSet)
	if err != nil {
		s.handleError(w, err)
		return
	}
	drafts := make([]model.AssistantDraftReceipt, 0, len(revisions))
	for _, revision := range revisions {
		if revision.ID == "" || revision.PageID == "" {
			continue
		}
		drafts = append(drafts, model.AssistantDraftReceipt{
			RevisionID: revision.ID,
			PageID:     revision.PageID,
			PageTitle:  revision.Title,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": map[string]any{"drafts": drafts}})
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
