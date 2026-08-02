package governance_test

import (
	"errors"
	"testing"

	"viki/internal/governance"
)

func TestObjectionRequiresReasonAndBlocksApprovalUntilResolved(t *testing.T) {
	t.Parallel()

	if err := governance.ValidateObjectionReason(""); !errors.Is(err, governance.ErrObjectionReasonRequired) {
		t.Fatalf("empty objection reason error = %v, want %v", err, governance.ErrObjectionReasonRequired)
	}

	objections := []governance.ObjectionState{{ID: "objection-1", Resolved: false}}
	if err := governance.CanApprove(objections); !errors.Is(err, governance.ErrUnresolvedObjection) {
		t.Fatalf("approval error = %v, want %v", err, governance.ErrUnresolvedObjection)
	}

	objections[0].Resolved = true
	if err := governance.CanApprove(objections); err != nil {
		t.Fatalf("approval after resolution: %v", err)
	}
}

func TestObjectionValidationAcceptsAReasonAndApprovalWithoutBlockers(t *testing.T) {
	t.Parallel()

	if err := governance.ValidateObjectionReason("  dôvod  "); err != nil {
		t.Fatalf("reasoned objection was rejected: %v", err)
	}
	if err := governance.CanApprove(nil); err != nil {
		t.Fatalf("approval without blockers failed: %v", err)
	}
}
