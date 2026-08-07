package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"sync"
)

type developmentClaim struct {
	taskID     string
	revisionID string
	lease      string
	finishing  bool
}

type developmentClaimStore struct {
	mu        sync.Mutex
	bySession map[string]developmentClaim
}

var readDevelopmentLeaseRandom = rand.Read

func newDevelopmentLease() (string, error) {
	value := make([]byte, 32)
	if _, err := readDevelopmentLeaseRandom(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (store *developmentClaimStore) reserve(sessionID, taskID, lease string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.bySession == nil {
		store.bySession = map[string]developmentClaim{}
	}
	if _, exists := store.bySession[sessionID]; exists {
		return false
	}
	store.bySession[sessionID] = developmentClaim{taskID: taskID, lease: lease}
	return true
}

func (store *developmentClaimStore) bind(sessionID, taskID, lease, revisionID string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	claim, valid := store.bySession[sessionID]
	if !valid || claim.taskID != taskID || !sameDevelopmentLease(claim.lease, lease) || claim.revisionID != "" {
		return false
	}
	claim.revisionID = revisionID
	store.bySession[sessionID] = claim
	return true
}

func (store *developmentClaimStore) begin(sessionID, taskID, lease string) (string, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	claim, valid := store.bySession[sessionID]
	if !valid || claim.taskID != taskID || claim.revisionID == "" || claim.finishing || !sameDevelopmentLease(claim.lease, lease) {
		return "", false
	}
	claim.finishing = true
	store.bySession[sessionID] = claim
	return claim.revisionID, true
}

func (store *developmentClaimStore) retry(sessionID, taskID, lease string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	claim, valid := store.bySession[sessionID]
	if !valid || claim.taskID != taskID || !sameDevelopmentLease(claim.lease, lease) {
		return
	}
	claim.finishing = false
	store.bySession[sessionID] = claim
}

func (store *developmentClaimStore) release(sessionID, taskID, lease string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	claim, valid := store.bySession[sessionID]
	if valid && claim.taskID == taskID && sameDevelopmentLease(claim.lease, lease) {
		delete(store.bySession, sessionID)
	}
}

func sameDevelopmentLease(expected, provided string) bool {
	if expected == "" || provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}
