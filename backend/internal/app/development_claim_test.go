package app

import (
	"errors"
	"testing"
)

func TestDevelopmentClaimStoreBindsOneOpaqueLeaseToOneTurn(t *testing.T) {
	lease, err := newDevelopmentLease()
	if err != nil || lease == "" {
		t.Fatalf("lease=%q err=%v", lease, err)
	}
	var claims developmentClaimStore
	if claims.bind("session-1", "task-1", lease, "revision-1") {
		t.Fatal("missing claim was bound")
	}
	if !claims.reserve("session-1", "task-1", lease) {
		t.Fatal("claim was not reserved")
	}
	if claims.reserve("session-1", "task-2", "another-lease") {
		t.Fatal("second claim was reserved for the same session")
	}
	if claims.bind("session-1", "other-task", lease, "revision-1") {
		t.Fatal("claim accepted a different task")
	}
	if !claims.bind("session-1", "task-1", lease, "revision-1") {
		t.Fatal("reserved claim was not bound")
	}
	if _, active := claims.begin("session-1", "task-1", "wrong-lease"); active {
		t.Fatal("claim accepted a different lease")
	}
	revisionID, active := claims.begin("session-1", "task-1", lease)
	if !active || revisionID != "revision-1" {
		t.Fatalf("claim revision=%q active=%t", revisionID, active)
	}
	if _, active := claims.begin("session-1", "task-1", lease); active {
		t.Fatal("finishing claim was reused concurrently")
	}
	claims.retry("session-1", "other-task", lease)
	if _, active := claims.begin("session-1", "task-1", lease); active {
		t.Fatal("mismatched retry changed the claim")
	}
	claims.retry("session-1", "task-1", lease)
	if _, active := claims.begin("session-1", "task-1", lease); !active {
		t.Fatal("valid retry did not restore the claim")
	}
	claims.release("session-1", "task-1", "wrong-lease")
	claims.release("session-1", "task-1", lease)
	if _, active := claims.begin("session-1", "task-1", lease); active {
		t.Fatal("released claim remained usable")
	}
	if sameDevelopmentLease("", lease) || sameDevelopmentLease(lease, "") || sameDevelopmentLease(lease, "wrong-lease") || !sameDevelopmentLease(lease, lease) {
		t.Fatal("lease comparison did not fail closed")
	}
}

func TestNewDevelopmentLeaseHandlesRandomFailure(t *testing.T) {
	original := readDevelopmentLeaseRandom
	readDevelopmentLeaseRandom = func([]byte) (int, error) { return 0, errors.New("entropy unavailable") }
	t.Cleanup(func() { readDevelopmentLeaseRandom = original })

	if _, err := newDevelopmentLease(); err == nil {
		t.Fatal("lease generation accepted a random source failure")
	}
}
