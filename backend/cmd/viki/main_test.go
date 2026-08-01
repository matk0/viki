package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"viki/internal/config"
	"viki/internal/hermes"
	"viki/internal/postgres"
)

var errStartupTest = errors.New("startup test failure")

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func readyTimer(time.Duration) <-chan time.Time {
	ready := make(chan time.Time, 1)
	ready <- time.Now()
	return ready
}

func TestMainExitsWithCommandStatus(t *testing.T) {
	originalRun := runCommand
	originalExit := exitProcess
	t.Cleanup(func() {
		runCommand = originalRun
		exitProcess = originalExit
	})

	runCommand = func() int { return 7 }
	got := -1
	exitProcess = func(code int) { got = code }
	main()
	if got != 7 {
		t.Fatalf("expected exit status 7, got %d", got)
	}
}

func TestCommandReportsConfigurationAndRuntimeOutcomes(t *testing.T) {
	originalLoad := loadConfiguration
	originalRun := runApplication
	t.Cleanup(func() {
		loadConfiguration = originalLoad
		runApplication = originalRun
	})

	t.Run("configuration error", func(t *testing.T) {
		loadConfiguration = func() (config.Config, error) { return config.Config{}, errStartupTest }
		if got := command(); got != 1 {
			t.Fatalf("expected failure status, got %d", got)
		}
	})

	loadConfiguration = func() (config.Config, error) { return config.Config{}, nil }
	t.Run("runtime error", func(t *testing.T) {
		runApplication = func(context.Context, config.Config, *slog.Logger) error { return errStartupTest }
		if got := command(); got != 1 {
			t.Fatalf("expected failure status, got %d", got)
		}
	})

	t.Run("success", func(t *testing.T) {
		runApplication = func(context.Context, config.Config, *slog.Logger) error { return nil }
		if got := command(); got != 0 {
			t.Fatalf("expected success status, got %d", got)
		}
	})
}

func TestRunConfiguredApplicationUsesActiveDependencies(t *testing.T) {
	original := activeDependencies
	t.Cleanup(func() { activeDependencies = original })
	counters := &runtimeCounters{serveError: http.ErrServerClosed}
	activeDependencies = baseDependencies(counters)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runConfiguredApplication(ctx, config.Config{}, testLogger()); err != nil {
		t.Fatal(err)
	}
	if counters.applicationCreated != 1 {
		t.Fatalf("expected the active dependencies to run, got %#v", counters)
	}
}

func TestOpenDatabaseRetriesStopsAndExhausts(t *testing.T) {
	originalOpen := openPostgres
	originalRetry := retryAfter
	t.Cleanup(func() {
		openPostgres = originalOpen
		retryAfter = originalRetry
	})

	expected := &postgres.Repository{}
	t.Run("immediate success", func(t *testing.T) {
		openPostgres = func(context.Context, string) (*postgres.Repository, error) { return expected, nil }
		got, err := openDatabase(context.Background(), "database")
		if err != nil || got != expected {
			t.Fatalf("unexpected result: repository=%p error=%v", got, err)
		}
	})

	t.Run("retry then success", func(t *testing.T) {
		calls := 0
		openPostgres = func(context.Context, string) (*postgres.Repository, error) {
			calls++
			if calls == 1 {
				return nil, errStartupTest
			}
			return expected, nil
		}
		retryAfter = readyTimer
		got, err := openDatabase(context.Background(), "database")
		if err != nil || got != expected || calls != 2 {
			t.Fatalf("unexpected retry result: repository=%p calls=%d error=%v", got, calls, err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		openPostgres = func(context.Context, string) (*postgres.Repository, error) { return nil, errStartupTest }
		retryAfter = func(time.Duration) <-chan time.Time { return make(chan time.Time) }
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := openDatabase(ctx, "database")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	})

	t.Run("attempts exhausted", func(t *testing.T) {
		calls := 0
		openPostgres = func(context.Context, string) (*postgres.Repository, error) {
			calls++
			return nil, errStartupTest
		}
		retryAfter = readyTimer
		_, err := openDatabase(context.Background(), "database")
		if !errors.Is(err, errStartupTest) || calls != databaseOpenAttempts {
			t.Fatalf("expected final error after %d attempts, calls=%d error=%v", databaseOpenAttempts, calls, err)
		}
	})
}

func TestProductionRuntimeConstructors(t *testing.T) {
	originalOpen := openPostgres
	originalRetry := retryAfter
	t.Cleanup(func() {
		openPostgres = originalOpen
		retryAfter = originalRetry
	})

	t.Run("database error", func(t *testing.T) {
		openPostgres = func(context.Context, string) (*postgres.Repository, error) { return nil, errStartupTest }
		retryAfter = readyTimer
		_, err := newRuntimeDatabase(context.Background(), "database")
		if !errors.Is(err, errStartupTest) {
			t.Fatalf("expected database error, got %v", err)
		}
	})

	t.Run("database success", func(t *testing.T) {
		openPostgres = func(context.Context, string) (*postgres.Repository, error) { return &postgres.Repository{}, nil }
		database, err := newRuntimeDatabase(context.Background(), "database")
		if err != nil || database.repository == nil || database.migrate == nil || database.ensureInitialUser == nil || database.close == nil {
			t.Fatalf("unexpected runtime database: %#v, %v", database, err)
		}
	})

	t.Run("gateway success", func(t *testing.T) {
		gateway, err := newRuntimeGateway(context.Background(), hermes.ManagerConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if gateway.gateway == nil || gateway.close == nil {
			t.Fatalf("unexpected runtime gateway: %#v", gateway)
		}
		if err := gateway.close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("gateway error", func(t *testing.T) {
		_, err := newRuntimeGateway(context.Background(), hermes.ManagerConfig{
			QA: hermes.ProfileConfig{URL: "://invalid"},
		})
		if err == nil {
			t.Fatal("expected invalid gateway URL to fail")
		}
	})

	t.Run("application", func(t *testing.T) {
		application := newRuntimeApplication(runtimeDatabase{}, runtimeGateway{}, config.Config{}, testLogger())
		if application.publicHandler == nil || application.internalHandler == nil || application.close == nil {
			t.Fatalf("unexpected runtime application: %#v", application)
		}
		application.close()
	})
}

func baseDependencies(counters *runtimeCounters) startupDependencies {
	return startupDependencies{
		openDatabase: func(context.Context, string) (runtimeDatabase, error) {
			return runtimeDatabase{
				migrate: func(context.Context) error {
					counters.migrated++
					return counters.migrationError
				},
				ensureInitialUser: func(context.Context, string) error {
					counters.seeded++
					return counters.seedError
				},
				close: func() { counters.databaseClosed++ },
			}, counters.databaseError
		},
		newGateway: func(context.Context, hermes.ManagerConfig) (runtimeGateway, error) {
			return runtimeGateway{close: func() error {
				counters.gatewayClosed++
				return nil
			}}, counters.gatewayError
		},
		newApplication: func(runtimeDatabase, runtimeGateway, config.Config, *slog.Logger) runtimeApplication {
			counters.applicationCreated++
			return runtimeApplication{
				publicHandler:   http.NotFoundHandler(),
				internalHandler: http.NotFoundHandler(),
				close:           func() { counters.applicationClosed++ },
			}
		},
		serve: func(*http.Server) error {
			counters.mu.Lock()
			defer counters.mu.Unlock()
			counters.served++
			return counters.serveError
		},
		shutdown: func(*http.Server, context.Context) error {
			counters.shutdown++
			return counters.shutdownError
		},
	}
}

type runtimeCounters struct {
	mu                                                     sync.Mutex
	databaseError, migrationError, seedError, gatewayError error
	serveError, shutdownError                              error
	migrated, seeded, databaseClosed                       int
	gatewayClosed, applicationCreated, applicationClosed   int
	served, shutdown                                       int
}

func TestRunHandlesStartupFailures(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*runtimeCounters)
		want      error
	}{
		{name: "database", configure: func(c *runtimeCounters) { c.databaseError = errStartupTest }, want: errStartupTest},
		{name: "migration", configure: func(c *runtimeCounters) { c.migrationError = errStartupTest }, want: errStartupTest},
		{name: "seed", configure: func(c *runtimeCounters) { c.seedError = errStartupTest }, want: errStartupTest},
		{name: "gateway", configure: func(c *runtimeCounters) { c.gatewayError = errStartupTest }, want: errStartupTest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			counters := &runtimeCounters{serveError: http.ErrServerClosed}
			test.configure(counters)
			err := run(context.Background(), config.Config{}, testLogger(), baseDependencies(counters))
			if !errors.Is(err, test.want) {
				t.Fatalf("expected startup error, got %v", err)
			}
		})
	}
}

func TestRunClosesResourcesAndServers(t *testing.T) {
	t.Run("graceful context cancellation", func(t *testing.T) {
		counters := &runtimeCounters{serveError: http.ErrServerClosed}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := run(ctx, config.Config{Address: ":8080", InternalAddress: "127.0.0.1:8090"}, testLogger(), baseDependencies(counters))
		if err != nil {
			t.Fatal(err)
		}
		if counters.migrated != 1 || counters.seeded != 1 || counters.served != 2 || counters.shutdown != 2 || counters.applicationCreated != 1 || counters.applicationClosed != 1 || counters.gatewayClosed != 1 || counters.databaseClosed != 1 {
			t.Fatalf("unexpected lifecycle counters: %#v", counters)
		}
	})

	t.Run("serve error", func(t *testing.T) {
		counters := &runtimeCounters{serveError: errStartupTest}
		err := run(context.Background(), config.Config{}, testLogger(), baseDependencies(counters))
		if !errors.Is(err, errStartupTest) || counters.shutdown != 2 {
			t.Fatalf("expected serve error and two shutdowns, counters=%#v error=%v", counters, err)
		}
	})

	t.Run("shutdown error", func(t *testing.T) {
		counters := &runtimeCounters{serveError: http.ErrServerClosed, shutdownError: errStartupTest}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := run(ctx, config.Config{}, testLogger(), baseDependencies(counters))
		if !errors.Is(err, errStartupTest) || counters.shutdown != 2 {
			t.Fatalf("expected shutdown error, counters=%#v error=%v", counters, err)
		}
	})
}
