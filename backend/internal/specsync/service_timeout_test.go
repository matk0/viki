package specsync

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type deadlineWorker struct {
	recorded    bool
	recordError error
	code        string
}

func (w *deadlineWorker) Poll(ctx context.Context) (Receipt, error) {
	<-ctx.Done()
	return Receipt{}, ctx.Err()
}
func (w *deadlineWorker) Record(ctx context.Context, code string) error {
	if ctx.Err() != nil {
		return errors.New("used expired attempt context")
	}
	w.recorded = true
	w.code = code
	return w.recordError
}
func TestExpiredPollStillRecordsFailureThroughFreshBoundedContext(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := &deadlineWorker{}
	pollOnce(context.Background(), worker, time.Millisecond, logger)
	if !worker.recorded || worker.code != "github_unavailable" {
		t.Fatal("timed-out poll did not expose actual failure", worker)
	}
	worker.recorded = false
	worker.recordError = io.EOF
	pollOnce(context.Background(), worker, time.Millisecond, logger)
	if !worker.recorded {
		t.Fatal("status update was not attempted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.recorded = false
	pollOnce(ctx, worker, time.Millisecond, logger)
	if worker.recorded {
		t.Fatal("intentional shutdown overwrote sync state")
	}
}
