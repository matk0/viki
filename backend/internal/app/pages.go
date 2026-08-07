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

func (s *Server) listStepDefinitions(w http.ResponseWriter, request *http.Request, auth authState) {
	var role *model.StepRole
	if raw := strings.TrimSpace(request.URL.Query().Get("role")); raw != "" {
		value := model.StepRole(raw)
		if value != model.StepContext && value != model.StepAction && value != model.StepOutcome {
			writeError(w, http.StatusUnprocessableEntity, "invalid_step_role", "Rola kroku nie je platná.")
			return
		}
		role = &value
	}
	definitions, err := s.repository.ListStepDefinitions(request.Context(), auth.Session.OrganizationID, request.URL.Query().Get("q"), role)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"definitions": definitions})
}

func (s *Server) createPage(w http.ResponseWriter, request *http.Request, auth authState) {
	var input model.CreatePageInput
	if !decodeJSON(w, request, &input) {
		return
	}
	if input.Kind == model.PageFeature && input.InitialScenario == nil {
		writeError(w, http.StatusUnprocessableEntity, "feature_requires_scenario", "Funkcia musí pri vytvorení obsahovať aspoň jeden scenár.")
		return
	}
	if input.Kind != model.PageFeature && input.InitialScenario != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_initial_scenario", "Úvodný scenár možno vytvoriť iba spolu s funkciou.")
		return
	}
	detail, err := s.repository.CreatePage(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, input)
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

func (s *Server) approveRevision(w http.ResponseWriter, request *http.Request, auth authState) {
	revisionID, ok := requirePathID(w, request, "revisionID")
	if !ok {
		return
	}
	detail, err := s.repository.ApproveRevision(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, revisionID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) raiseObjection(w http.ResponseWriter, request *http.Request, auth authState) {
	revisionID, ok := requirePathID(w, request, "revisionID")
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	if err := governance.ValidateObjectionReason(input.Reason); err != nil {
		s.handleError(w, err)
		return
	}
	objection, err := s.repository.AddObjection(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, revisionID, input.Reason)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, objection)
}

func (s *Server) addComment(w http.ResponseWriter, request *http.Request, auth authState) {
	var input struct {
		PageID          string  `json:"pageId"`
		RevisionID      string  `json:"revisionId"`
		ParentCommentID *string `json:"parentCommentId"`
		Body            string  `json:"body"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	comment, err := s.repository.AddComment(
		request.Context(), auth.Session.OrganizationID, auth.Session.User.ID,
		input.PageID, input.RevisionID, input.ParentCommentID, input.Body,
	)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, comment)
}

func (s *Server) resolveObjection(w http.ResponseWriter, request *http.Request, auth authState) {
	objectionID, ok := requirePathID(w, request, "objectionID")
	if !ok {
		return
	}
	objection, err := s.repository.ResolveObjection(request.Context(), auth.Session.OrganizationID, auth.Session.User.ID, objectionID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objection)
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
