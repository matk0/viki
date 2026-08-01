package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"viki/internal/app"
	"viki/internal/config"
	"viki/internal/hermes"
	"viki/internal/postgres"
	"viki/internal/store"
)

const databaseOpenAttempts = 30

type runtimeDatabase struct {
	repository        store.Repository
	migrate           func(context.Context) error
	ensureInitialUser func(context.Context, string) error
	close             func()
}

type runtimeGateway struct {
	gateway hermes.Gateway
	close   func() error
}

type runtimeApplication struct {
	publicHandler   http.Handler
	internalHandler http.Handler
	close           func()
}

type startupDependencies struct {
	openDatabase   func(context.Context, string) (runtimeDatabase, error)
	newGateway     func(context.Context, hermes.ManagerConfig) (runtimeGateway, error)
	newApplication func(runtimeDatabase, runtimeGateway, config.Config, *slog.Logger) runtimeApplication
	serve          func(*http.Server) error
	shutdown       func(*http.Server, context.Context) error
}

var (
	exitProcess        = os.Exit
	loadConfiguration  = config.Load
	runCommand         = command
	runApplication     = runConfiguredApplication
	openPostgres       = postgres.Open
	retryAfter         = time.After
	activeDependencies = productionDependencies()
)

func main() {
	exitProcess(runCommand())
}

func command() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := loadConfiguration()
	if err != nil {
		logger.Error("configuration error", "error", err)
		return 1
	}

	rootContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := runApplication(rootContext, cfg, logger); err != nil {
		logger.Error("application stopped", "error", err)
		return 1
	}
	return 0
}

func productionDependencies() startupDependencies {
	return startupDependencies{
		openDatabase:   newRuntimeDatabase,
		newGateway:     newRuntimeGateway,
		newApplication: newRuntimeApplication,
		serve:          (*http.Server).ListenAndServe,
		shutdown:       (*http.Server).Shutdown,
	}
}

func runConfiguredApplication(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	return run(ctx, cfg, logger, activeDependencies)
}

func newRuntimeDatabase(ctx context.Context, databaseURL string) (runtimeDatabase, error) {
	repository, err := openDatabase(ctx, databaseURL)
	if err != nil {
		return runtimeDatabase{}, err
	}
	return runtimeDatabase{
		repository:        repository,
		migrate:           repository.Migrate,
		ensureInitialUser: repository.EnsureInitialUser,
		close:             repository.Close,
	}, nil
}

func newRuntimeGateway(ctx context.Context, managerConfig hermes.ManagerConfig) (runtimeGateway, error) {
	gateway, err := hermes.NewManager(ctx, managerConfig)
	if err != nil {
		return runtimeGateway{}, err
	}
	return runtimeGateway{gateway: gateway, close: gateway.Close}, nil
}

func newRuntimeApplication(database runtimeDatabase, gateway runtimeGateway, cfg config.Config, logger *slog.Logger) runtimeApplication {
	application := app.NewApplication(database.repository, gateway.gateway, app.Options{
		CookieSecure:      cfg.CookieSecure,
		SessionTTL:        cfg.SessionTTL,
		FrontendDir:       cfg.FrontendDir,
		HermesToolToken:   cfg.HermesToolToken,
		HandoffSigningKey: cfg.HermesToolToken,
	}, logger)
	return runtimeApplication{
		publicHandler:   application.PublicHandler(),
		internalHandler: application.InternalHandler(),
		close:           application.Close,
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger, dependencies startupDependencies) error {
	database, err := dependencies.openDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database startup failed: %w", err)
	}
	defer database.close()
	if err := database.migrate(ctx); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}
	if err := database.ensureInitialUser(ctx, cfg.InitialPassword); err != nil {
		return fmt.Errorf("initial user setup failed: %w", err)
	}

	gateway, err := dependencies.newGateway(ctx, hermes.ManagerConfig{
		QA: hermes.ProfileConfig{
			URL: cfg.HermesQAWSURL, Token: cfg.HermesQAToken,
			Configured: cfg.HermesQAConfigured && cfg.HermesToolToken != "",
		},
		Edit: hermes.ProfileConfig{
			URL: cfg.HermesEditWSURL, Token: cfg.HermesEditToken,
			Configured: cfg.HermesEditConfigured && cfg.HermesToolToken != "",
		},
	})
	if err != nil {
		return fmt.Errorf("Hermes gateway configuration failed: %w", err)
	}
	defer func() { _ = gateway.close() }()

	application := dependencies.newApplication(database, gateway, cfg, logger)
	defer application.close()
	publicServer := &http.Server{
		Addr:              cfg.Address,
		Handler:           application.publicHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       2 * time.Minute,
	}
	internalServer := &http.Server{
		Addr:              cfg.InternalAddress,
		Handler:           application.internalHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       3 * time.Minute,
		WriteTimeout:      3 * time.Minute,
		IdleTimeout:       time.Minute,
	}
	return serveUntilStopped(ctx, logger, dependencies, publicServer, internalServer)
}

func serveUntilStopped(
	ctx context.Context,
	logger *slog.Logger,
	dependencies startupDependencies,
	publicServer, internalServer *http.Server,
) error {
	type serverSpec struct {
		name   string
		server *http.Server
	}
	servers := []serverSpec{
		{name: "public", server: publicServer},
		{name: "internal Hermes tools", server: internalServer},
	}
	unexpected := make(chan error, len(servers))
	var wait sync.WaitGroup
	for _, spec := range servers {
		spec := spec
		wait.Add(1)
		go func() {
			defer wait.Done()
			logger.Info("viki listening", "server", spec.name, "address", spec.server.Addr)
			if err := dependencies.serve(spec.server); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("HTTP server failed", "server", spec.name, "error", err)
				unexpected <- fmt.Errorf("%s HTTP server failed: %w", spec.name, err)
			}
		}()
	}

	var runtimeError error
	select {
	case <-ctx.Done():
	case runtimeError = <-unexpected:
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, spec := range servers {
		if err := dependencies.shutdown(spec.server, shutdownContext); err != nil {
			logger.Error("graceful shutdown failed", "server", spec.name, "error", err)
			runtimeError = errors.Join(runtimeError, fmt.Errorf("shut down %s server: %w", spec.name, err))
		}
	}
	wait.Wait()
	return runtimeError
}

func openDatabase(ctx context.Context, databaseURL string) (*postgres.Repository, error) {
	var lastError error
	for attempt := 0; attempt < databaseOpenAttempts; attempt++ {
		repository, err := openPostgres(ctx, databaseURL)
		if err == nil {
			return repository, nil
		}
		lastError = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-retryAfter(time.Second):
		}
	}
	return nil, lastError
}
