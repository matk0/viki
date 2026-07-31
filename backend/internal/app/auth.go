package app

import (
	"net"
	"net/http"
	"strings"
	"time"

	"viki/internal/security"
)

func (s *Server) login(w http.ResponseWriter, request *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	key := loginKey(request, input.Email)
	if !s.limiter.allow(key) {
		writeError(w, http.StatusTooManyRequests, "login_rate_limited", "Priveľa neúspešných pokusov. Skúste to o päť minút.")
		return
	}
	credential, err := s.repository.CredentialByEmail(request.Context(), input.Email)
	if err != nil || !credential.Active || !security.VerifyPassword(credential.PasswordHash, input.Password) {
		s.limiter.fail(key)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "E-mail alebo heslo nie je správne.")
		return
	}
	sessionToken, sessionHash, err := security.NewOpaqueToken()
	if err != nil {
		s.handleError(w, err)
		return
	}
	csrfToken, csrfHash, err := security.NewOpaqueToken()
	if err != nil {
		s.handleError(w, err)
		return
	}
	expires := time.Now().Add(s.options.SessionTTL)
	if err := s.repository.CreateSession(request.Context(), credential.ID, sessionHash, csrfHash, expires); err != nil {
		s.handleError(w, err)
		return
	}
	s.limiter.success(key)
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: sessionToken, Path: "/", HttpOnly: true, Secure: s.options.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires})
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: csrfToken, Path: "/", HttpOnly: false, Secure: s.options.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires})
	writeJSON(w, http.StatusOK, map[string]any{"user": credential.User, "csrfToken": csrfToken})
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request, auth authState) {
	writeJSON(w, http.StatusOK, map[string]any{"user": auth.Session.User})
}

func (s *Server) logout(w http.ResponseWriter, request *http.Request, auth authState) {
	if err := s.repository.DeleteSession(request.Context(), auth.TokenHash); err != nil {
		s.handleError(w, err)
		return
	}
	clearAuthCookies(w, s.options.CookieSecure)
	w.WriteHeader(http.StatusNoContent)
}

func clearAuthCookies(w http.ResponseWriter, secure bool) {
	for _, name := range []string{sessionCookieName, csrfCookieName} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: name == sessionCookieName, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	}
}

func loginKey(request *http.Request, email string) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	return host + "|" + email
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-5 * time.Minute)
	recent := l.attempts[key][:0]
	for _, attempt := range l.attempts[key] {
		if attempt.After(cutoff) {
			recent = append(recent, attempt)
		}
	}
	l.attempts[key] = recent
	return len(recent) < 5
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[key] = append(l.attempts[key], time.Now())
}

func (l *loginLimiter) success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
