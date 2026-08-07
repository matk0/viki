package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"viki/internal/governance"
	"viki/internal/model"
	"viki/internal/security"
	"viki/internal/store"
)

const (
	sessionCookieName = "viki_session"
	csrfCookieName    = "viki_csrf"
)

type authState struct {
	Session   model.Session
	TokenHash []byte
}

type authHandler func(http.ResponseWriter, *http.Request, authState)

func (s *Server) requireAuth(next authHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Prihláste sa, prosím.")
			return
		}
		tokenHash := security.HashToken(cookie.Value)
		session, err := s.repository.SessionByHash(request.Context(), tokenHash)
		if err != nil {
			clearAuthCookies(w, s.options.CookieSecure)
			writeError(w, http.StatusUnauthorized, "unauthorized", "Relácia vypršala. Prihláste sa znova.")
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodOptions {
			csrfCookie, cookieErr := request.Cookie(csrfCookieName)
			csrfHeader := request.Header.Get("X-CSRF-Token")
			if cookieErr != nil || csrfHeader == "" || csrfCookie.Value != csrfHeader || subtle.ConstantTimeCompare(session.CSRFHash, security.HashToken(csrfHeader)) != 1 {
				writeError(w, http.StatusForbidden, "csrf_failed", "Bezpečnostný token nie je platný. Obnovte stránku.")
				return
			}
		}
		next(w, request, authState{Session: session, TokenHash: tokenHash})
	}
}

func decodeJSON(w http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(w, request.Body, 2*1024*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Požiadavka nemá platný formát.")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "Požiadavka obsahuje viac ako jeden JSON objekt.")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (s *Server) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Záznam sa nenašiel.")
	case errors.Is(err, store.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized", "Prihlásenie zlyhalo.")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "revision_conflict", "Stránku medzitým upravil iný používateľ. Obnovte draft.")
	case errors.Is(err, store.ErrDuplicateSlug):
		writeError(w, http.StatusConflict, "duplicate_slug", "Stránka s touto adresou už existuje.")
	case errors.Is(err, store.ErrInvalidHierarchy), errors.Is(err, store.ErrInvalidReference):
		writeError(w, http.StatusUnprocessableEntity, "invalid_page", "Hierarchia alebo referencia stránky nie je platná.")
	case errors.Is(err, governance.ErrObjectionReasonRequired):
		writeError(w, http.StatusUnprocessableEntity, "objection_reason_required", "Pri námietke musíte uviesť dôvod.")
	case errors.Is(err, governance.ErrUnresolvedObjection):
		writeError(w, http.StatusConflict, "unresolved_objection", "Schváleniu bráni nevyriešená námietka.")
	case errors.Is(err, governance.ErrParentFeatureNotApproved):
		writeError(w, http.StatusConflict, "parent_feature_not_approved", "Scenár možno schváliť až po schválení nadradenej funkcie.")
	default:
		s.logger.Error("request failed", "error", err)
		writeError(w, http.StatusUnprocessableEntity, "request_failed", userSafeError(err))
	}
}

func userSafeError(err error) string {
	message := err.Error()
	if strings.Contains(message, "required") || strings.Contains(message, "invalid") || strings.Contains(message, "must") || strings.Contains(message, "cannot") {
		return message
	}
	return "Požiadavku sa nepodarilo spracovať."
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, request)
		s.logger.Info("http request", "method", request.Method, "path", request.URL.Path, "duration", time.Since(started))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(self)")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; font-src 'self'")
		next.ServeHTTP(w, request)
	})
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("panic", "value", recovered, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal_error", "Nastala neočakávaná chyba.")
			}
		}()
		next.ServeHTTP(w, request)
	})
}

func contextWithTimeout(request *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(request.Context(), timeout)
}

func pointer[T any](value T) *T { return &value }

func requirePathID(w http.ResponseWriter, request *http.Request, name string) (string, bool) {
	value := strings.TrimSpace(request.PathValue(name))
	if value == "" {
		writeError(w, http.StatusBadRequest, "missing_id", fmt.Sprintf("Chýba identifikátor %s.", name))
		return "", false
	}
	return value, true
}
