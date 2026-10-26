package model

import "time"

type RepositorySource struct {
	ExternalID     string    `json:"externalId"`
	Repository     string    `json:"repository"`
	Commit         string    `json:"commit"`
	ContentHash    string    `json:"contentHash"`
	Path           string    `json:"path"`
	Content        string    `json:"content"`
	RunID          string    `json:"runId"`
	RunAttempt     int       `json:"runAttempt"`
	VerifiedCommit string    `json:"verifiedCommit"`
	Result         string    `json:"result"`
	ImportedAt     time.Time `json:"importedAt"`
}
type WorkspaceSyncStatus struct {
	Mode           string     `json:"mode"`
	Repository     string     `json:"repository,omitempty"`
	LastCheckedAt  *time.Time `json:"lastCheckedAt,omitempty"`
	LastImportedAt *time.Time `json:"lastImportedAt,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	LastErrorAt    *time.Time `json:"lastErrorAt,omitempty"`
	Commit         string     `json:"commit,omitempty"`
	RunID          string     `json:"runId,omitempty"`
	RunAttempt     int        `json:"runAttempt,omitempty"`
	ContentHash    string     `json:"contentHash,omitempty"`
	PageCount      int        `json:"pageCount"`
	ScenarioCount  int        `json:"scenarioCount"`
	Stale          bool       `json:"stale"`
}
