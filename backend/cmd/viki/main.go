package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"viki/internal/app"
	"viki/internal/config"
	"viki/internal/hermes"
	"viki/internal/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	rootContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	repository, err := openDatabase(rootContext, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database startup failed", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	if err := repository.Migrate(rootContext); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	if err := repository.EnsureInitialUser(rootContext, cfg.InitialPassword); err != nil {
		logger.Error("initial user setup failed", "error", err)
		os.Exit(1)
	}

	gateway, err := hermes.NewManager(rootContext, hermes.ManagerConfig{
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
		logger.Error("Hermes gateway configuration failed", "error", err)
		os.Exit(1)
	}
	defer gateway.Close()

	application := app.NewApplication(repository, gateway, app.Options{
		CookieSecure:      cfg.CookieSecure,
		SessionTTL:        cfg.SessionTTL,
		FrontendDir:       cfg.FrontendDir,
		HermesToolToken:   cfg.HermesToolToken,
		HandoffSigningKey: cfg.HermesToolToken,
	}, logger)
	defer application.Close()
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           application.PublicHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       2 * time.Minute,
	}
	internalServer := &http.Server{
		Addr:              cfg.InternalAddress,
		Handler:           application.InternalHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       3 * time.Minute,
		WriteTimeout:      3 * time.Minute,
		IdleTimeout:       time.Minute,
	}
	go func() {
		logger.Info("viki listening", "address", cfg.Address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			stop()
		}
	}()
	go func() {
		logger.Info("viki internal Hermes tools listening", "address", cfg.InternalAddress)
		if err := internalServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("internal HTTP server failed", "error", err)
			stop()
		}
	}()
	<-rootContext.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	if err := internalServer.Shutdown(shutdownContext); err != nil {
		logger.Error("internal graceful shutdown failed", "error", err)
	}
}

func openDatabase(ctx context.Context, databaseURL string) (*postgres.Repository, error) {
	var lastError error
	for attempt := 0; attempt < 30; attempt++ {
		repository, err := postgres.Open(ctx, databaseURL)
		if err == nil {
			return repository, nil
		}
		lastError = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, lastError
}
