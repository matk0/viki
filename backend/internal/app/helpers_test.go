package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"viki/internal/governance"
	"viki/internal/model"
	"viki/internal/security"
	"viki/internal/store"
)

type helperRepository struct {
	store.Repository
	session model.Session
	err     error
}

func (r *helperRepository) SessionByHash(context.Context, []byte) (model.Session, error) {
	return r.session, r.err
}

func TestDecodeJSONAcceptsOneKnownObjectAndRejectsInvalidBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		ok   bool
	}{
		{name: "valid", body: `{"name":"viki"}`, ok: true},
		{name: "malformed", body: `{`, ok: false},
		{name: "unknown field", body: `{"other":true}`, ok: false},
		{name: "multiple objects", body: `{"name":"first"} {"name":"second"}`, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			recorder := httptest.NewRecorder()
			var target struct {
				Name string `json:"name"`
			}
			if got := decodeJSON(recorder, request, &target); got != test.ok {
				t.Fatalf("decodeJSON() = %t, want %t; body=%s", got, test.ok, recorder.Body.String())
			}
			if test.ok && target.Name != "viki" {
				t.Fatalf("decoded name = %q", target.Name)
			}
		})
	}

	oversized := bytes.Repeat([]byte("x"), 2*1024*1024+1)
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(oversized))
	recorder := httptest.NewRecorder()
	if decodeJSON(recorder, request, &struct{}{}) {
		t.Fatal("oversized JSON body was accepted")
	}
}

func TestHandleErrorMapsEveryDomainFailureToSafeHTTPResponse(t *testing.T) {
	t.Parallel()
	server := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{store.ErrNotFound, http.StatusNotFound, "not_found"},
		{store.ErrUnauthorized, http.StatusUnauthorized, "unauthorized"},
		{store.ErrConflict, http.StatusConflict, "revision_conflict"},
		{store.ErrDuplicateSlug, http.StatusConflict, "duplicate_slug"},
		{store.ErrInvalidHierarchy, http.StatusUnprocessableEntity, "invalid_page"},
		{store.ErrInvalidReference, http.StatusUnprocessableEntity, "invalid_page"},
		{governance.ErrRejectionReasonRequired, http.StatusUnprocessableEntity, "rejection_reason_required"},
		{governance.ErrInvalidVote, http.StatusUnprocessableEntity, "invalid_vote"},
		{governance.ErrUnresolvedRejection, http.StatusConflict, "unresolved_rejection"},
		{governance.ErrRejectedProposalDependency, http.StatusConflict, "rejected_proposal_dependency"},
		{errors.New("field is required"), http.StatusUnprocessableEntity, "request_failed"},
		{errors.New("database exploded"), http.StatusUnprocessableEntity, "request_failed"},
	}

	for _, test := range tests {
		recorder := httptest.NewRecorder()
		server.handleError(recorder, test.err)
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
			t.Fatalf("error %v produced status=%d body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestUserSafeErrorAllowsValidationLanguageOnly(t *testing.T) {
	t.Parallel()
	for _, message := range []string{"required value", "invalid value", "must exist", "cannot publish"} {
		if got := userSafeError(errors.New(message)); got != message {
			t.Fatalf("safe validation error = %q, want %q", got, message)
		}
	}
	if got := userSafeError(errors.New("database password leaked")); got != "Požiadavku sa nepodarilo spracovať." {
		t.Fatalf("unsafe error projection = %q", got)
	}
}

func TestAuthenticationMiddlewareFailsClosedAndAcceptsValidCSRF(t *testing.T) {
	csrf := "csrf-token"
	session := model.Session{
		User:           model.User{ID: "user-1"},
		OrganizationID: "organization-1",
		CSRFHash:       security.HashToken(csrf),
		Expires:        time.Now().Add(time.Hour),
	}

	invoke := func(repository *helperRepository, method string, cookies []*http.Cookie, header string) (*httptest.ResponseRecorder, bool) {
		t.Helper()
		server := &Server{repository: repository, options: Options{CookieSecure: true}}
		called := false
		handler := server.requireAuth(func(w http.ResponseWriter, _ *http.Request, auth authState) {
			called = true
			if auth.Session.User.ID != session.User.ID {
				t.Fatalf("authenticated user = %q", auth.Session.User.ID)
			}
			w.WriteHeader(http.StatusNoContent)
		})
		request := httptest.NewRequest(method, "/", nil)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		if header != "" {
			request.Header.Set("X-CSRF-Token", header)
		}
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		return recorder, called
	}

	sessionCookie := &http.Cookie{Name: sessionCookieName, Value: "session-token"}
	csrfCookie := &http.Cookie{Name: csrfCookieName, Value: csrf}
	if recorder, called := invoke(&helperRepository{session: session}, http.MethodGet, nil, ""); recorder.Code != http.StatusUnauthorized || called {
		t.Fatalf("missing cookie status=%d called=%t", recorder.Code, called)
	}
	if recorder, called := invoke(&helperRepository{err: store.ErrNotFound}, http.MethodGet, []*http.Cookie{sessionCookie}, ""); recorder.Code != http.StatusUnauthorized || called || len(recorder.Result().Cookies()) != 2 {
		t.Fatalf("expired session status=%d called=%t cookies=%d", recorder.Code, called, len(recorder.Result().Cookies()))
	}
	if recorder, called := invoke(&helperRepository{session: session}, http.MethodGet, []*http.Cookie{sessionCookie}, ""); recorder.Code != http.StatusNoContent || !called {
		t.Fatalf("authenticated GET status=%d called=%t", recorder.Code, called)
	}
	if recorder, called := invoke(&helperRepository{session: session}, http.MethodPost, []*http.Cookie{sessionCookie}, ""); recorder.Code != http.StatusForbidden || called {
		t.Fatalf("missing CSRF status=%d called=%t", recorder.Code, called)
	}
	if recorder, called := invoke(&helperRepository{session: session}, http.MethodPost, []*http.Cookie{sessionCookie, csrfCookie}, "wrong"); recorder.Code != http.StatusForbidden || called {
		t.Fatalf("mismatched CSRF status=%d called=%t", recorder.Code, called)
	}
	if recorder, called := invoke(&helperRepository{session: session}, http.MethodPost, []*http.Cookie{sessionCookie, csrfCookie}, csrf); recorder.Code != http.StatusNoContent || !called {
		t.Fatalf("valid CSRF status=%d called=%t", recorder.Code, called)
	}
}

func TestRecoveryTimeoutAndPathHelpers(t *testing.T) {
	t.Parallel()
	server := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	panicHandler := server.recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	recorder := httptest.NewRecorder()
	panicHandler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "internal_error") {
		t.Fatalf("panic status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, cancel := contextWithTimeout(request, time.Millisecond)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("request timeout did not create a deadline")
	}

	recorder = httptest.NewRecorder()
	if value, ok := requirePathID(recorder, request, "pageID"); ok || value != "" || recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing path ID = %q, %t, status=%d", value, ok, recorder.Code)
	}
	request.SetPathValue("pageID", " page-1 ")
	if value, ok := requirePathID(httptest.NewRecorder(), request, "pageID"); !ok || value != "page-1" {
		t.Fatalf("path ID = %q, %t", value, ok)
	}

	value := pointer("viki")
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) != `"viki"` {
		t.Fatalf("pointer helper encoded %s, %v", encoded, err)
	}
}
