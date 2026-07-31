package app

import (
	"net/http"
	"strings"

	"viki/internal/governance"
	"viki/internal/model"
)

type rejectDraftProposalInput struct {
	Reason string `json:"reason"`
}

func (s *Server) listDraftProposals(w http.ResponseWriter, request *http.Request, auth authState) {
	proposals, err := s.repository.ListAssistantDraftProposals(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": proposals})
}

func (s *Server) draftProposal(w http.ResponseWriter, request *http.Request, auth authState) {
	proposalID, ok := requirePathID(w, request, "proposalID")
	if !ok {
		return
	}
	proposal, err := s.repository.AssistantDraftProposal(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, proposalID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}

func (s *Server) approveDraftProposal(w http.ResponseWriter, request *http.Request, auth authState) {
	proposalID, ok := requirePathID(w, request, "proposalID")
	if !ok {
		return
	}
	proposal, err := s.repository.PublishAssistantDraftProposal(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, proposalID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.publishProposalEvent(proposal, "draft_published")
	writeJSON(w, http.StatusOK, proposal)
}

func (s *Server) discardDraftProposal(w http.ResponseWriter, request *http.Request, auth authState) {
	proposalID, ok := requirePathID(w, request, "proposalID")
	if !ok {
		return
	}
	var input rejectDraftProposalInput
	if !decodeJSON(w, request, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || len([]rune(input.Reason)) > 2000 {
		s.handleError(w, governance.ErrRejectionReasonRequired)
		return
	}
	proposal, err := s.repository.DiscardAssistantDraftProposal(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, proposalID, input.Reason)
	if err != nil {
		s.handleError(w, err)
		return
	}
	s.publishProposalEvent(proposal, "draft_discarded")
	writeJSON(w, http.StatusOK, proposal)
}

func (s *Server) publishProposalEvent(proposal model.AssistantDraftProposal, eventType string) {
	if s.assistant == nil || proposal.ConversationID == "" {
		return
	}
	s.assistant.publish(proposal.ConversationID, eventType, map[string]any{
		"turnId":   proposal.TurnID,
		"mode":     model.AssistantEdit,
		"proposal": proposal,
	})
}
