package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"viki/internal/hermes"
	"viki/internal/store"
)

type readinessRepository struct {
	store.Repository
	err error
}

func (r *readinessRepository) Ping(context.Context) error { return r.err }

func TestReadyReflectsDatabaseAvailability(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{name: "ready", status: http.StatusOK, body: `"status":"ready"`},
		{name: "unavailable", err: errors.New("offline"), status: http.StatusServiceUnavailable, body: "database_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{repository: &readinessRepository{err: test.err}}
			recorder := httptest.NewRecorder()
			server.ready(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.body) {
				t.Fatalf("ready status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestNewApplicationPreservesExplicitOptionsAndSuppliesDefaultLogger(t *testing.T) {
	application := NewApplication(
		&readinessRepository{},
		hermes.NewFakeGateway(),
		Options{SessionTTL: time.Hour},
		nil,
	)
	application.Close()

	if application.PublicHandler() == nil || application.InternalHandler() == nil {
		t.Fatal("application omitted an HTTP handler")
	}
}
