package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"viki/internal/hermes"
	"viki/internal/store"
)

type Options struct {
	CookieSecure           bool
	SessionTTL             time.Duration
	FrontendDir            string
	HermesToolToken        string
	HandoffSigningKey      string
	DevelopmentTargetURL   string
	DevelopmentTargetToken string
}

type Server struct {
	repository store.Repository
	gateway    hermes.Gateway
	assistant  *assistantRuntime
	options    Options
	logger     *slog.Logger
	limiter    *loginLimiter
	target     developmentTarget
}

type Application struct {
	public   http.Handler
	internal http.Handler
	cancel   context.CancelFunc
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func New(repository store.Repository, gateway hermes.Gateway, options Options, logger *slog.Logger) http.Handler {
	return NewApplication(repository, gateway, options, logger).PublicHandler()
}

func NewApplication(repository store.Repository, gateway hermes.Gateway, options Options, logger *slog.Logger) *Application {
	if options.SessionTTL == 0 {
		options.SessionTTL = 12 * time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{
		repository: repository,
		gateway:    gateway,
		options:    options,
		logger:     logger,
		limiter:    &loginLimiter{attempts: map[string][]time.Time{}},
		target:     newHTTPDevelopmentTarget(options.DevelopmentTargetURL, options.DevelopmentTargetToken),
	}
	server.assistant = newAssistantRuntime(ctx, repository, gateway, options.HandoffSigningKey, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.HandleFunc("POST /api/v1/auth/login", server.login)
	mux.HandleFunc("GET /api/v1/auth/me", server.requireAuth(server.me))
	mux.HandleFunc("POST /api/v1/auth/logout", server.requireAuth(server.logout))
	mux.HandleFunc("GET /api/v1/pages", server.requireAuth(server.listPages))
	mux.HandleFunc("GET /api/v1/step-definitions", server.requireAuth(server.listStepDefinitions))
	mux.HandleFunc("POST /api/v1/pages", server.requireAuth(server.createPage))
	mux.HandleFunc("GET /api/v1/pages/{pageID}", server.requireAuth(server.pageDetail))
	mux.HandleFunc("POST /api/v1/pages/{pageID}/revisions", server.requireAuth(server.saveRevision))
	mux.HandleFunc("GET /api/v1/revisions/{revisionID}", server.requireAuth(server.revisionDetail))
	mux.HandleFunc("POST /api/v1/revisions/{revisionID}/approve", server.requireAuth(server.approveRevision))
	mux.HandleFunc("POST /api/v1/revisions/{revisionID}/objections", server.requireAuth(server.raiseObjection))
	mux.HandleFunc("POST /api/v1/comments", server.requireAuth(server.addComment))
	mux.HandleFunc("POST /api/v1/objections/{objectionID}/resolve", server.requireAuth(server.resolveObjection))
	mux.HandleFunc("GET /api/v1/audit", server.requireAuth(server.listAudit))
	mux.HandleFunc("GET /api/v1/assistant/status", server.requireAuth(server.assistantStatus))
	mux.HandleFunc("GET /api/v1/assistant/conversations", server.requireAuth(server.listAssistantConversations))
	mux.HandleFunc("POST /api/v1/assistant/conversations", server.requireAuth(server.createAssistantConversation))
	mux.HandleFunc("GET /api/v1/assistant/conversations/{conversationID}", server.requireAuth(server.assistantConversationDetail))
	mux.HandleFunc("PATCH /api/v1/assistant/conversations/{conversationID}", server.requireAuth(server.updateAssistantConversation))
	mux.HandleFunc("POST /api/v1/assistant/conversations/{conversationID}/messages", server.requireAuth(server.submitAssistantMessage))
	mux.HandleFunc("GET /api/v1/assistant/conversations/{conversationID}/events", server.requireAuth(server.streamAssistantEvents))
	mux.HandleFunc("POST /api/v1/assistant/conversations/{conversationID}/stop", server.requireAuth(server.stopAssistantTurn))
	mux.HandleFunc("POST /api/v1/assistant/conversations/{conversationID}/clarifications/{requestID}", server.requireAuth(server.respondAssistantClarification))
	mux.HandleFunc("/internal/", http.NotFound)
	mux.HandleFunc("/", server.serveFrontend)
	publicHandler := server.recover(server.securityHeaders(server.requestLog(mux)))
	return &Application{
		public:   publicHandler,
		internal: server.internalHandler(),
		cancel:   cancel,
	}
}

func (a *Application) PublicHandler() http.Handler   { return a.public }
func (a *Application) InternalHandler() http.Handler { return a.internal }
func (a *Application) Close()                        { a.cancel() }

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, request *http.Request) {
	if err := s.repository.Ping(request.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "Databáza nie je pripravená.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
