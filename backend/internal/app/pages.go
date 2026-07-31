package app

import (
	"net/http"
	"strconv"
	"strings"

	"viki/internal/governance"
	"viki/internal/model"
)

func (s *Server) listPages(w http.ResponseWriter, request *http.Request, auth authState) {
	var kind *model.PageKind
	if raw := request.URL.Query().Get("kind"); raw != "" {
		value := model.PageKind(raw)
		kind = &value
	}
	query := strings.TrimSpace(request.URL.Query().Get("q"))
	if query != "" {
		includeDrafts, _ := strconv.ParseBool(request.URL.Query().Get("includeDrafts"))
		results, err := s.repository.SearchPages(request.Context(), auth.Session.OrganizationID, model.SearchOptions{
			Query: query, Kind: kind, IncludeDrafts: includeDrafts, Limit: 50,
		})
		if err != nil {
			s.handleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": results})
		return
	}
	pages, err := s.repository.ListPages(request.Context(), auth.Session.OrganizationID, kind)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": pages})
}

func (s *Server) createPage(w http.ResponseWriter, request *http.Request, auth authState) {
	var input model.CreatePageInput
	if !decodeJSON(w, request, &input) {
		return
	}
	detail, err := s.repository.CreatePage(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, input, model.RevisionDraft)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (s *Server) pageDetail(w http.ResponseWriter, request *http.Request, auth authState) {
	pageID, ok := requirePathID(w, request, "pageID")
	if !ok {
		return
	}
	detail, err := s.repository.PageDetail(request.Context(), auth.Session.OrganizationID, pageID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) revisionDetail(w http.ResponseWriter, request *http.Request, auth authState) {
	revisionID, ok := requirePathID(w, request, "revisionID")
	if !ok {
		return
	}
	revision, err := s.repository.Revision(request.Context(), auth.Session.OrganizationID, revisionID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, revision)
}

func (s *Server) saveRevision(w http.ResponseWriter, request *http.Request, auth authState) {
	pageID, ok := requirePathID(w, request, "pageID")
	if !ok {
		return
	}
	var input model.SaveRevisionInput
	if !decodeJSON(w, request, &input) {
		return
	}
	revision, err := s.repository.SaveRevision(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, pageID, input)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, revision)
}

func (s *Server) publishRevision(w http.ResponseWriter, request *http.Request, auth authState) {
	revisionID, ok := requirePathID(w, request, "revisionID")
	if !ok {
		return
	}
	detail, err := s.repository.PublishRevision(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, revisionID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) setVote(w http.ResponseWriter, request *http.Request, auth authState) {
	revisionID, ok := requirePathID(w, request, "revisionID")
	if !ok {
		return
	}
	var input struct {
		Value  governance.VoteValue `json:"value"`
		Reason string               `json:"reason"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	if err := governance.ValidateVote(input.Value, input.Reason); err != nil {
		s.handleError(w, err)
		return
	}
	vote, err := s.repository.SetVote(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, revisionID, input.Value, input.Reason)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vote)
}

func (s *Server) addComment(w http.ResponseWriter, request *http.Request, auth authState) {
	var input struct {
		PageID          string  `json:"pageId"`
		RevisionID      string  `json:"revisionId"`
		ParentCommentID *string `json:"parentCommentId"`
		AnchorKind      *string `json:"anchorKind"`
		AnchorID        *string `json:"anchorId"`
		Body            string  `json:"body"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	comment, err := s.repository.AddComment(
		request.Context(), auth.Session.OrganizationID, auth.Session.User.ID,
		input.PageID, input.RevisionID, input.ParentCommentID, input.AnchorKind, input.AnchorID, input.Body, false,
	)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, comment)
}

func (s *Server) resolveComment(w http.ResponseWriter, request *http.Request, auth authState) {
	commentID, ok := requirePathID(w, request, "commentID")
	if !ok {
		return
	}
	comment, err := s.repository.ResolveComment(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, commentID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, comment)
}

func (s *Server) listAudit(w http.ResponseWriter, request *http.Request, auth authState) {
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	events, err := s.repository.ListAudit(request.Context(), auth.Session.OrganizationID, limit)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
