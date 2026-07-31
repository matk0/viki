package governance_test

import (
	"errors"
	"testing"

	"viki/internal/governance"
)

func TestRejectionRequiresReasonAndBlocksPublicationUntilResolved(t *testing.T) {
	t.Parallel()

	if err := governance.ValidateVote(governance.VoteReject, ""); !errors.Is(err, governance.ErrRejectionReasonRequired) {
		t.Fatalf("empty rejection reason error = %v, want %v", err, governance.ErrRejectionReasonRequired)
	}

	threads := []governance.BlockingThread{{ID: "thread-1", Resolved: false}}
	if err := governance.CanPublish(threads); !errors.Is(err, governance.ErrUnresolvedRejection) {
		t.Fatalf("publication error = %v, want %v", err, governance.ErrUnresolvedRejection)
	}

	threads[0].Resolved = true
	if err := governance.CanPublish(threads); err != nil {
		t.Fatalf("publication after resolution: %v", err)
	}
}
