package main

import (
	"context"
	"log/slog"
	"testing"
	"time"
	"viki/internal/specsync"
)

func TestWorkerCommandAndSignalLifecycle(t *testing.T) {
	t.Setenv("VIKI_GITHUB_READ_TOKEN", "")
	t.Setenv("VIKI_SNAPSHOT_IMPORT_TOKEN", "")
	if command() != 1 {
		t.Fatal("missing credentials accepted")
	}
	t.Setenv("VIKI_GITHUB_READ_TOKEN", "repository-scoped-test-token")
	t.Setenv("VIKI_SNAPSHOT_IMPORT_TOKEN", "01234567890123456789012345678901")
	oldServe, oldExit := serve, exitProcess
	t.Cleanup(func() { serve, exitProcess = oldServe, oldExit })
	serve = func(_ context.Context, _ specsync.Worker, interval time.Duration, _ *slog.Logger) {
		if interval != 5*time.Minute {
			t.Fatal(interval)
		}
	}
	exitProcess = func(code int) {
		if code != 0 {
			t.Fatal(code)
		}
	}
	main()
}
