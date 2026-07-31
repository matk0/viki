package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"viki/internal/app"
	"viki/internal/hermes"
	"viki/internal/model"
	"viki/internal/security"
	"viki/internal/store"
)

type voteRepository struct {
	store.Repository
	session model.Session
}

type proposalRepository struct {
	store.Repository
	session         model.Session
	proposal        model.AssistantDraftProposal
	discarded       bool
	rejectionReason string
}

func (r *proposalRepository) SessionByHash(context.Context, []byte) (model.Session, error) {
	return r.session, nil
}

func (r *proposalRepository) AssistantDraftProposal(context.Context, string, string, string) (model.AssistantDraftProposal, error) {
	return r.proposal, nil
}

func (r *proposalRepository) PublishAssistantDraftProposal(context.Context, string, string, string) (model.AssistantDraftProposal, error) {
	r.proposal.Status = model.AssistantProposalPublished
	return r.proposal, nil
}

func (r *proposalRepository) DiscardAssistantDraftProposal(_ context.Context, _, _, _, reason string) (model.AssistantDraftProposal, error) {
	r.discarded = true
	r.rejectionReason = reason
	r.proposal.Status = model.AssistantProposalDiscarded
	r.proposal.RejectionReason = reason
	return r.proposal, nil
}

func (r *voteRepository) SessionByHash(context.Context, []byte) (model.Session, error) {
	return r.session, nil
}

func TestRejectVoteWithoutReasonIsRejectedAtHTTPBoundary(t *testing.T) {
	t.Parallel()

	csrf := "csrf-test-token"
	repository := &voteRepository{session: model.Session{
		User:           model.User{ID: "00000000-0000-4000-8000-000000000011", Email: "matej@matejlukasik.com"},
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		CSRFHash:       security.HashToken(csrf),
		Expires:        time.Now().Add(time.Hour),
	}}
	handler := app.New(repository, hermes.NewFakeGateway(), app.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body, _ := json.Marshal(map[string]string{"value": "reject", "reason": ""})
	request := httptest.NewRequest(http.MethodPut, "/api/v1/revisions/00000000-0000-4000-8000-000000000099/vote", bytes.NewReader(body))
	request.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
	request.AddCookie(&http.Cookie{Name: "viki_csrf", Value: csrf})
	request.Header.Set("X-CSRF-Token", csrf)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
}

func TestSecurityHeadersAllowSameOriginVoiceInput(t *testing.T) {
	t.Parallel()

	handler := app.New(&voteRepository{}, hermes.NewFakeGateway(), app.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Permissions-Policy"); got != "camera=(), geolocation=(), microphone=(self)" {
		t.Fatalf("Permissions-Policy = %q", got)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); strings.Contains(got, "media-src") || strings.Contains(got, "blob:") {
		t.Fatalf("removed voice media remains allowed by CSP: %q", got)
	}
}

func TestDraftProposalCanBeReadAndApprovedThroughAuthenticatedAPI(t *testing.T) {
	t.Parallel()

	csrf := "proposal-csrf"
	proposalID := "00000000-0000-4000-8000-000000000088"
	repository := &proposalRepository{
		session: model.Session{
			User:           model.User{ID: "00000000-0000-4000-8000-000000000011", Email: "matej@matejlukasik.com"},
			OrganizationID: "00000000-0000-4000-8000-000000000001", CSRFHash: security.HashToken(csrf), Expires: time.Now().Add(time.Hour),
		},
		proposal: model.AssistantDraftProposal{
			ID: proposalID, TurnID: proposalID, Summary: "Pridať zákazníka", Status: model.AssistantProposalAwaitingApproval,
			Operations: []model.AIChangeOperation{{Operation: "create", Kind: model.PageScenario, Slug: "novy-scenar", Content: model.RevisionContent{Title: "Nový scenár"}}},
		},
	}
	handler := app.New(repository, hermes.NewFakeGateway(), app.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	read := httptest.NewRequest(http.MethodGet, "/api/v1/draft-proposals/"+proposalID, nil)
	read.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
	readRecorder := httptest.NewRecorder()
	handler.ServeHTTP(readRecorder, read)
	if readRecorder.Code != http.StatusOK || !strings.Contains(readRecorder.Body.String(), "awaiting_approval") {
		t.Fatalf("read status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}

	approve := httptest.NewRequest(http.MethodPost, "/api/v1/draft-proposals/"+proposalID+"/approve", nil)
	approve.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
	approve.AddCookie(&http.Cookie{Name: "viki_csrf", Value: csrf})
	approve.Header.Set("X-CSRF-Token", csrf)
	approveRecorder := httptest.NewRecorder()
	handler.ServeHTTP(approveRecorder, approve)
	if approveRecorder.Code != http.StatusOK || !strings.Contains(approveRecorder.Body.String(), "published") {
		t.Fatalf("approve status=%d body=%s", approveRecorder.Code, approveRecorder.Body.String())
	}
}

func TestDraftProposalRejectionRequiresReasonAtHTTPBoundary(t *testing.T) {
	t.Parallel()

	csrf := "proposal-rejection-csrf"
	proposalID := "00000000-0000-4000-8000-000000000089"
	repository := &proposalRepository{
		session: model.Session{
			User:           model.User{ID: "00000000-0000-4000-8000-000000000011", Email: "matej@matejlukasik.com"},
			OrganizationID: "00000000-0000-4000-8000-000000000001", CSRFHash: security.HashToken(csrf), Expires: time.Now().Add(time.Hour),
		},
		proposal: model.AssistantDraftProposal{ID: proposalID, Status: model.AssistantProposalAwaitingApproval},
	}
	handler := app.New(repository, hermes.NewFakeGateway(), app.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body, _ := json.Marshal(map[string]string{"reason": "   "})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/draft-proposals/"+proposalID+"/discard", bytes.NewReader(body))
	request.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
	request.AddCookie(&http.Cookie{Name: "viki_csrf", Value: csrf})
	request.Header.Set("X-CSRF-Token", csrf)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
	if repository.discarded {
		t.Fatal("repository was called without a rejection reason")
	}

	body, _ = json.Marshal(map[string]string{"reason": "  Chýba presný spôsob výpočtu ceny.  "})
	request = httptest.NewRequest(http.MethodPost, "/api/v1/draft-proposals/"+proposalID+"/discard", bytes.NewReader(body))
	request.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
	request.AddCookie(&http.Cookie{Name: "viki_csrf", Value: csrf})
	request.Header.Set("X-CSRF-Token", csrf)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if repository.rejectionReason != "Chýba presný spôsob výpočtu ceny." {
		t.Fatalf("rejection reason = %q", repository.rejectionReason)
	}
	if !strings.Contains(recorder.Body.String(), `"rejectionReason":"Chýba presný spôsob výpočtu ceny."`) {
		t.Fatalf("response does not include rejection reason: %s", recorder.Body.String())
	}
}
