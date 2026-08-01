package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"viki/internal/model"
	"viki/internal/security"
	"viki/internal/store"
)

type authRepository struct {
	store.Repository
	credential       model.Credential
	credentialErr    error
	createSessionErr error
	deleteSessionErr error
	lookedUpEmail    string
	createdUserID    string
	deletedHash      []byte
}

func (r *authRepository) CredentialByEmail(_ context.Context, email string) (model.Credential, error) {
	r.lookedUpEmail = email
	return r.credential, r.credentialErr
}

func (r *authRepository) CreateSession(_ context.Context, userID string, _, _ []byte, _ time.Time) error {
	r.createdUserID = userID
	return r.createSessionErr
}

func (r *authRepository) DeleteSession(_ context.Context, tokenHash []byte) error {
	r.deletedHash = append([]byte(nil), tokenHash...)
	return r.deleteSessionErr
}

func newAuthTestServer(repository *authRepository) *Server {
	return &Server{
		repository: repository,
		options:    Options{SessionTTL: time.Hour, CookieSecure: true},
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		limiter:    &loginLimiter{attempts: map[string][]time.Time{}},
	}
}

func TestLoginNormalizesIdentityCreatesSessionAndClearsRateLimit(t *testing.T) {
	passwordHash, err := security.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	repository := &authRepository{credential: model.Credential{
		User:         model.User{ID: "user-1", Email: "matej@example.com"},
		PasswordHash: passwordHash,
		Active:       true,
	}}
	server := newAuthTestServer(repository)
	key := "192.0.2.1|matej@example.com"
	server.limiter.attempts[key] = []time.Time{time.Now()}

	original := newOpaqueToken
	tokens := []string{"session-token", "csrf-token"}
	newOpaqueToken = func() (string, []byte, error) {
		token := tokens[0]
		tokens = tokens[1:]
		return token, security.HashToken(token), nil
	}
	t.Cleanup(func() { newOpaqueToken = original })

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"  MATEJ@EXAMPLE.COM ","password":"correct-password"}`))
	recorder := httptest.NewRecorder()
	server.login(recorder, request)

	if recorder.Code != http.StatusOK || repository.lookedUpEmail != "matej@example.com" || repository.createdUserID != "user-1" {
		t.Fatalf("login status=%d email=%q user=%q body=%s", recorder.Code, repository.lookedUpEmail, repository.createdUserID, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 2 || cookies[0].Value != "session-token" || cookies[1].Value != "csrf-token" || !cookies[0].Secure || !cookies[1].Secure {
		t.Fatalf("login cookies = %+v", cookies)
	}
	if _, exists := server.limiter.attempts[key]; exists {
		t.Fatal("successful login retained failed-attempt history")
	}
	if !strings.Contains(recorder.Body.String(), `"csrfToken":"csrf-token"`) {
		t.Fatalf("login response = %s", recorder.Body.String())
	}
}

func TestLoginRejectsInvalidRateLimitedAndIncorrectCredentials(t *testing.T) {
	passwordHash, err := security.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		body       string
		credential model.Credential
		repoErr    error
		prepare    func(*Server)
		status     int
	}{
		{name: "invalid JSON", body: `{`, status: http.StatusBadRequest},
		{name: "repository failure", body: `{"email":"matej@example.com","password":"x"}`, repoErr: store.ErrNotFound, status: http.StatusUnauthorized},
		{name: "inactive", body: `{"email":"matej@example.com","password":"correct-password"}`, credential: model.Credential{PasswordHash: passwordHash}, status: http.StatusUnauthorized},
		{name: "wrong password", body: `{"email":"matej@example.com","password":"wrong"}`, credential: model.Credential{PasswordHash: passwordHash, Active: true}, status: http.StatusUnauthorized},
		{name: "rate limited", body: `{"email":"matej@example.com","password":"correct-password"}`, credential: model.Credential{PasswordHash: passwordHash, Active: true}, prepare: func(server *Server) {
			server.limiter.attempts["192.0.2.1|matej@example.com"] = []time.Time{
				time.Now(), time.Now(), time.Now(), time.Now(), time.Now(),
			}
		}, status: http.StatusTooManyRequests},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &authRepository{credential: test.credential, credentialErr: test.repoErr}
			server := newAuthTestServer(repository)
			if test.prepare != nil {
				test.prepare(server)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(test.body))
			recorder := httptest.NewRecorder()
			server.login(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			if repository.createdUserID != "" {
				t.Fatalf("rejected login created session for %q", repository.createdUserID)
			}
		})
	}
}

func TestLoginReportsTokenAndSessionCreationFailures(t *testing.T) {
	passwordHash, err := security.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	credential := model.Credential{User: model.User{ID: "user-1"}, PasswordHash: passwordHash, Active: true}
	original := newOpaqueToken
	t.Cleanup(func() { newOpaqueToken = original })

	tests := []struct {
		name       string
		tokenCalls []struct {
			token string
			err   error
		}
		repositoryError error
	}{
		{name: "session token", tokenCalls: []struct {
			token string
			err   error
		}{{err: errors.New("session entropy failed")}}},
		{name: "CSRF token", tokenCalls: []struct {
			token string
			err   error
		}{{token: "session-token"}, {err: errors.New("csrf entropy failed")}}},
		{name: "repository", tokenCalls: []struct {
			token string
			err   error
		}{{token: "session-token"}, {token: "csrf-token"}}, repositoryError: errors.New("create session failed")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := append([]struct {
				token string
				err   error
			}(nil), test.tokenCalls...)
			newOpaqueToken = func() (string, []byte, error) {
				call := calls[0]
				calls = calls[1:]
				return call.token, security.HashToken(call.token), call.err
			}
			repository := &authRepository{credential: credential, createSessionErr: test.repositoryError}
			server := newAuthTestServer(repository)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"matej@example.com","password":"correct-password"}`))
			recorder := httptest.NewRecorder()
			server.login(recorder, request)
			if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "request_failed") {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestMeAndLogoutProjectUserAndDestroySession(t *testing.T) {
	repository := &authRepository{}
	server := newAuthTestServer(repository)
	auth := authState{Session: model.Session{User: model.User{ID: "user-1", Email: "matej@example.com"}}, TokenHash: []byte("hash")}

	meRecorder := httptest.NewRecorder()
	server.me(meRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), auth)
	if meRecorder.Code != http.StatusOK || !strings.Contains(meRecorder.Body.String(), `"email":"matej@example.com"`) {
		t.Fatalf("me status=%d body=%s", meRecorder.Code, meRecorder.Body.String())
	}

	logoutRecorder := httptest.NewRecorder()
	server.logout(logoutRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil), auth)
	if logoutRecorder.Code != http.StatusNoContent || !bytes.Equal(repository.deletedHash, auth.TokenHash) || len(logoutRecorder.Result().Cookies()) != 2 {
		t.Fatalf("logout status=%d hash=%q cookies=%+v", logoutRecorder.Code, repository.deletedHash, logoutRecorder.Result().Cookies())
	}

	repository.deleteSessionErr = errors.New("delete failed")
	errorRecorder := httptest.NewRecorder()
	server.logout(errorRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil), auth)
	if errorRecorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failed logout status=%d body=%s", errorRecorder.Code, errorRecorder.Body.String())
	}
}

func TestLoginLimiterExpiresFailuresAndLoginKeyHandlesBareAddresses(t *testing.T) {
	limiter := &loginLimiter{attempts: map[string][]time.Time{
		"key": {time.Now().Add(-6 * time.Minute), time.Now()},
	}}
	if !limiter.allow("key") || len(limiter.attempts["key"]) != 1 {
		t.Fatalf("expired attempts were not removed: %+v", limiter.attempts["key"])
	}
	limiter.fail("key")
	if len(limiter.attempts["key"]) != 2 {
		t.Fatalf("failure was not recorded: %+v", limiter.attempts["key"])
	}
	limiter.success("key")
	if _, exists := limiter.attempts["key"]; exists {
		t.Fatal("success did not clear attempts")
	}

	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.RemoteAddr = "bare-address"
	if got := loginKey(request, "matej@example.com"); got != "bare-address|matej@example.com" {
		t.Fatalf("login key = %q", got)
	}
}
