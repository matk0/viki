package app

import (
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"strings"

	"viki/internal/model"
	"viki/internal/specsync"
	"viki/internal/store"
)

const initialWorkspaceID = "00000000-0000-4000-8000-000000000001"

type repositorySync interface {
	WorkspaceSyncStatus(context.Context, string) (model.WorkspaceSyncStatus, error)
	ImportRepositorySnapshot(context.Context, string, specsync.Snapshot) (model.WorkspaceSyncStatus, error)
	RecordRepositoryPoll(context.Context, string, string) error
}

func (s *Server) workspaceStatus(ctx context.Context, org string) (model.WorkspaceSyncStatus, error) {
	if r, ok := s.repository.(repositorySync); ok {
		return r.WorkspaceSyncStatus(ctx, org)
	}
	return model.WorkspaceSyncStatus{Mode: "human_governed"}, nil
}
func (s *Server) workspaceSyncStatus(w http.ResponseWriter, r *http.Request, auth authState) {
	status, err := s.workspaceStatus(r.Context(), auth.Session.OrganizationID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
func (s *Server) permitAssistantMode(w http.ResponseWriter, r *http.Request, auth authState, mode model.AssistantMode) bool {
	if mode != model.AssistantEdit {
		return true
	}
	status, err := s.workspaceStatus(r.Context(), auth.Session.OrganizationID)
	if err != nil {
		s.handleError(w, err)
		return false
	}
	if status.Mode == "git_managed" {
		s.handleError(w, store.ErrReadOnly)
		return false
	}
	return true
}
func (s *Server) repositoryImportAuth(w http.ResponseWriter, r *http.Request) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	expected := s.options.SnapshotImportToken
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || len(expected) < 32 || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_import_credential", "Snapshot import credential is not valid.")
		return false
	}
	return true
}
func (s *Server) importRepositorySnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.repositoryImportAuth(w, r) {
		return
	}
	repo, ok := s.repository.(repositorySync)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "sync_unavailable", "Repository sync is unavailable.")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8*1024*1024))
	if err != nil {
		_ = repo.RecordRepositoryPoll(r.Context(), initialWorkspaceID, "snapshot_rejected")
		writeError(w, http.StatusBadRequest, "invalid_snapshot", "Snapshot exceeds the request limit.")
		return
	}
	snapshot, err := specsync.Decode(data)
	if err == nil {
		err = snapshot.Eligible()
	}
	if err == nil {
		_, err = snapshot.Entries()
	}
	if err != nil {
		_ = repo.RecordRepositoryPoll(r.Context(), initialWorkspaceID, "snapshot_rejected")
		writeError(w, http.StatusUnprocessableEntity, "invalid_snapshot", err.Error())
		return
	}
	status, err := repo.ImportRepositorySnapshot(r.Context(), initialWorkspaceID, snapshot)
	if err != nil {
		_ = repo.RecordRepositoryPoll(r.Context(), initialWorkspaceID, "snapshot_rejected")
		writeError(w, http.StatusConflict, "snapshot_rejected", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}
func (s *Server) recordRepositoryPoll(w http.ResponseWriter, r *http.Request) {
	if !s.repositoryImportAuth(w, r) {
		return
	}
	repo, ok := s.repository.(repositorySync)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "sync_unavailable", "Repository sync is unavailable.")
		return
	}
	var input struct {
		Error string `json:"error"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	allowed := map[string]bool{"": true, "github_authentication_failed": true, "github_unavailable": true, "artifact_unavailable": true, "candidate_rejected": true, "import_unavailable": true, "no_eligible_main_run": true}
	if !allowed[input.Error] {
		writeError(w, http.StatusBadRequest, "invalid_sync_status", "Unsupported sync error code.")
		return
	}
	if err := repo.RecordRepositoryPoll(r.Context(), initialWorkspaceID, input.Error); err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"recorded": true})
}

func (s *Server) privateRepositoryStatus(w http.ResponseWriter, r *http.Request) {
	if !s.repositoryImportAuth(w, r) {
		return
	}
	status, err := s.workspaceStatus(r.Context(), initialWorkspaceID)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
