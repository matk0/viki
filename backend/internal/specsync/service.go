package specsync

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"
)

type Worker interface {
	Poll(context.Context) (Receipt, error)
	Record(context.Context, string) error
}

func WorkerFromEnvironment(env func(string) string) (*Poller, error) {
	githubToken := env("VIKI_GITHUB_READ_TOKEN")
	importToken := env("VIKI_SNAPSHOT_IMPORT_TOKEN")
	if strings.TrimSpace(githubToken) == "" || len(importToken) < 32 {
		return nil, fmt.Errorf("repository-scoped GitHub read token and separate snapshot import token are required")
	}
	target := env("VIKI_SYNC_INTERNAL_URL")
	if target == "" {
		target = "http://viki:8090"
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid internal sync URL")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Port() == "" || (u.Hostname() != "viki" && (ip == nil || !ip.IsLoopback())) {
		return nil, fmt.Errorf("sync URL must be an explicit private Viki service or loopback HTTP listener")
	}
	return &Poller{Client: NewHTTPClient(), GitHubURL: "https://api.github.com", ImportURL: target, GitHubToken: githubToken, ImportToken: importToken, Now: time.Now, RetryDelay: time.Second}, nil
}
func pollOnce(ctx context.Context, worker Worker, timeout time.Duration, logger *slog.Logger) {
	attempt, cancel := context.WithTimeout(ctx, timeout)
	receipt, err := worker.Poll(attempt)
	cancel()
	if err != nil {
		code := ErrorCode(err)
		recorded := false
		// A failed/expired attempt must still report its error through a usable context.
		// Service shutdown is different: do not overwrite status during intentional stop.
		if ctx.Err() == nil {
			report, stop := context.WithTimeout(ctx, 10*time.Second)
			recorded = worker.Record(report, code) == nil
			stop()
		}
		logger.Warn("repository sync poll failed", "code", code, "statusRecorded", recorded)
	} else {
		logger.Info("repository sync poll", "receipt", receipt)
	}
}
func Run(ctx context.Context, worker Worker, interval time.Duration, logger *slog.Logger) {
	for {
		if ctx.Err() != nil {
			return
		}
		pollOnce(ctx, worker, 90*time.Second, logger)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
