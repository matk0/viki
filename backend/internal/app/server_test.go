package app_test

import (
	"context"
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

type testRepository struct {
	store.Repository
	session model.Session
}

func (r *testRepository) SessionByHash(context.Context, []byte) (model.Session, error) {
	return r.session, nil
}

func TestLegacyVoteEndpointIsUnavailable(t *testing.T) {
	t.Parallel()

	csrf := "csrf-test-token"
	repository := &testRepository{session: model.Session{
		User:           model.User{ID: "00000000-0000-4000-8000-000000000011", Email: "matej@matejlukasik.com"},
		OrganizationID: "00000000-0000-4000-8000-000000000001",
		CSRFHash:       security.HashToken(csrf),
		Expires:        time.Now().Add(time.Hour),
	}}
	handler := app.New(repository, hermes.NewFakeGateway(), app.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodPut, "/api/v1/revisions/00000000-0000-4000-8000-000000000099/vote", nil)
	request.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
	request.AddCookie(&http.Cookie{Name: "viki_csrf", Value: csrf})
	request.Header.Set("X-CSRF-Token", csrf)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestSecurityHeadersAllowSameOriginVoiceInput(t *testing.T) {
	t.Parallel()

	handler := app.New(&testRepository{}, hermes.NewFakeGateway(), app.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestLegacyDraftProposalRoutesAreUnavailableToAuthenticatedUsers(t *testing.T) {
	t.Parallel()

	csrf := "legacy-proposal-csrf"
	proposalID := "00000000-0000-4000-8000-000000000088"
	repository := &testRepository{session: model.Session{
		User:           model.User{ID: "00000000-0000-4000-8000-000000000011", Email: "matej@matejlukasik.com"},
		OrganizationID: "00000000-0000-4000-8000-000000000001", CSRFHash: security.HashToken(csrf), Expires: time.Now().Add(time.Hour),
	}}
	handler := app.New(repository, hermes.NewFakeGateway(), app.Options{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/draft-proposals"},
		{http.MethodGet, "/api/v1/draft-proposals/" + proposalID},
		{http.MethodPost, "/api/v1/draft-proposals/" + proposalID + "/operations/concept/review"},
		{http.MethodPost, "/api/v1/draft-proposals/" + proposalID + "/approve"},
		{http.MethodPost, "/api/v1/draft-proposals/" + proposalID + "/discard"},
	}
	for _, route := range paths {
		request := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
		request.AddCookie(&http.Cookie{Name: "viki_session", Value: "opaque-session"})
		if route.method == http.MethodPost {
			request.AddCookie(&http.Cookie{Name: "viki_csrf", Value: csrf})
			request.Header.Set("X-CSRF-Token", csrf)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s %s status = %d, want 404; body=%s", route.method, route.path, recorder.Code, recorder.Body.String())
		}
	}
}
