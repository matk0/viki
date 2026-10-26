package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"viki/internal/specsync"
)

var exitProcess = os.Exit
var serve = specsync.Run

func main() { exitProcess(command()) }
func command() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	worker, err := specsync.WorkerFromEnvironment(os.Getenv)
	if err != nil {
		logger.Error("sync configuration error", "error", err)
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	serve(ctx, worker, 5*time.Minute, logger)
	return 0
}
