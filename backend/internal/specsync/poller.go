package specsync

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"viki/internal/model"
)

const repositoryAPI = "/repos/matk0/vozokany"
const maxArchiveBytes = 16 * 1024 * 1024
const maxExpandedBytes = 32 * 1024 * 1024

type Poller struct {
	Client      *http.Client
	GitHubURL   string
	ImportURL   string
	GitHubToken string
	ImportToken string
	Now         func() time.Time
	RetryDelay  time.Duration
}
type Receipt struct {
	State        string `json:"state"`
	SourceCommit string `json:"sourceCommit"`
	RunID        string `json:"runId"`
	RunAttempt   int    `json:"runAttempt"`
	ContentHash  string `json:"contentHash"`
}
type pollError struct{ code string }

func (e pollError) Error() string { return e.code }
func ErrorCode(err error) string {
	var e pollError
	if errors.As(err, &e) {
		return e.code
	}
	return "github_unavailable"
}
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" {
			return fmt.Errorf("unsupported redirect")
		}
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			req.Header.Del("Authorization")
		}
		return nil
	}}
}
func (p *Poller) request(ctx context.Context, method, url, token string, body []byte, limit int64) ([]byte, error) {
	var data []byte
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		response, err := p.Client.Do(req)
		if err != nil {
			return nil, err
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, limit+1))
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > limit {
			return nil, pollError{"candidate_rejected"}
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return data, nil
		}
		if response.StatusCode == 401 || response.StatusCode == 403 {
			return nil, pollError{"github_authentication_failed"}
		}
		if (response.StatusCode != 429 && response.StatusCode < 500) || attempt == 2 {
			return nil, pollError{"github_unavailable"}
		}
		delay := p.RetryDelay
		if delay <= 0 {
			delay = time.Second
		}
		timer := time.NewTimer(delay * time.Duration(attempt+1))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func (p *Poller) get(ctx context.Context, path string, target any) error {
	data, err := p.request(ctx, http.MethodGet, p.GitHubURL+path, p.GitHubToken, nil, 2*1024*1024)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, target); err != nil {
		return pollError{"candidate_rejected"}
	}
	return nil
}
func (p *Poller) Record(ctx context.Context, code string) error {
	data, _ := json.Marshal(map[string]string{"error": code})
	_, err := p.request(ctx, http.MethodPost, p.ImportURL+"/internal/v1/repository-sync/status", p.ImportToken, data, 1024*1024)
	return err
}

type githubRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}
type githubRun struct {
	ID             int64            `json:"id"`
	Attempt        int              `json:"run_attempt"`
	HeadSHA        string           `json:"head_sha"`
	HeadBranch     string           `json:"head_branch"`
	Event          string           `json:"event"`
	Path           string           `json:"path"`
	Status         string           `json:"status"`
	Conclusion     string           `json:"conclusion"`
	WorkflowID     int64            `json:"workflow_id"`
	Repository     githubRepository `json:"repository"`
	HeadRepository githubRepository `json:"head_repository"`
}
type githubArtifact struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Expired   bool      `json:"expired"`
	ExpiresAt time.Time `json:"expires_at"`
	Size      int64     `json:"size_in_bytes"`
	Digest    string    `json:"digest"`
	Run       struct {
		ID               int64  `json:"id"`
		RepositoryID     int64  `json:"repository_id"`
		HeadRepositoryID int64  `json:"head_repository_id"`
		HeadSHA          string `json:"head_sha"`
		HeadBranch       string `json:"head_branch"`
	} `json:"workflow_run"`
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (p *Poller) Poll(ctx context.Context) (Receipt, error) {
	receipt := Receipt{}
	data, err := p.request(ctx, http.MethodGet, p.ImportURL+"/internal/v1/repository-sync", p.ImportToken, nil, 1024*1024)
	if err != nil {
		return receipt, pollError{"import_unavailable"}
	}
	var current model.WorkspaceSyncStatus
	if json.Unmarshal(data, &current) != nil || current.Mode != "git_managed" || current.Repository != Repository {
		return receipt, pollError{"import_unavailable"}
	}
	var repository githubRepository
	if err = p.get(ctx, repositoryAPI, &repository); err != nil {
		return receipt, err
	}
	if repository.ID <= 0 || repository.FullName != "matk0/vozokany" {
		return receipt, pollError{"candidate_rejected"}
	}
	var branch struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err = p.get(ctx, repositoryAPI+"/branches/main", &branch); err != nil {
		return receipt, err
	}
	var workflow struct {
		ID   int64  `json:"id"`
		Path string `json:"path"`
	}
	if err = p.get(ctx, repositoryAPI+"/actions/workflows/verify.yml", &workflow); err != nil {
		return receipt, err
	}
	if workflow.ID <= 0 || workflow.Path != Workflow {
		return receipt, pollError{"candidate_rejected"}
	}
	var runs struct {
		Runs []githubRun `json:"workflow_runs"`
	}
	if err = p.get(ctx, repositoryAPI+"/actions/workflows/verify.yml/runs?branch=main&event=push&per_page=10", &runs); err != nil {
		return receipt, err
	}
	if len(runs.Runs) == 0 {
		return receipt, pollError{"no_eligible_main_run"}
	}
	run := runs.Runs[0]
	for _, candidate := range runs.Runs {
		if candidate.ID > run.ID {
			run = candidate
		}
	}
	var verified githubRun
	if run.ID <= 0 || run.Attempt < 1 {
		return receipt, pollError{"candidate_rejected"}
	}
	if err = p.get(ctx, repositoryAPI+fmt.Sprintf("/actions/runs/%d/attempts/%d", run.ID, run.Attempt), &verified); err != nil {
		return receipt, err
	}
	if !reflect.DeepEqual(run, verified) {
		return receipt, pollError{"candidate_rejected"}
	}
	if run.Status != "completed" || run.Conclusion != "success" {
		return receipt, pollError{"no_eligible_main_run"}
	}
	if run.Path != Workflow || run.WorkflowID != workflow.ID || run.Event != "push" || run.HeadBranch != "main" || run.Repository.ID != repository.ID || run.HeadRepository.ID != repository.ID || run.Repository.FullName != repository.FullName || run.HeadRepository.FullName != repository.FullName || !commitPattern.MatchString(run.HeadSHA) || run.HeadSHA != branch.Commit.SHA {
		return receipt, pollError{"candidate_rejected"}
	}
	receipt = Receipt{State: "unchanged", SourceCommit: run.HeadSHA, RunID: strconv.FormatInt(run.ID, 10), RunAttempt: run.Attempt, ContentHash: current.ContentHash}
	if current.RunID != "" {
		last, err := strconv.ParseInt(current.RunID, 10, 64)
		if err != nil || run.ID < last || (run.ID == last && run.Attempt < current.RunAttempt) {
			return receipt, pollError{"candidate_rejected"}
		}
		if run.ID == last && run.Attempt == current.RunAttempt {
			if current.Commit != run.HeadSHA {
				return receipt, pollError{"candidate_rejected"}
			}
			return receipt, p.Record(ctx, "")
		}
	}
	if current.Commit != "" && current.Commit != run.HeadSHA {
		if !commitPattern.MatchString(current.Commit) {
			return receipt, pollError{"candidate_rejected"}
		}
		var comparison struct {
			Status     string `json:"status"`
			BaseCommit struct {
				SHA string `json:"sha"`
			} `json:"base_commit"`
			MergeBaseCommit struct {
				SHA string `json:"sha"`
			} `json:"merge_base_commit"`
		}
		if err = p.get(ctx, repositoryAPI+"/compare/"+current.Commit+"..."+run.HeadSHA, &comparison); err != nil {
			return receipt, err
		}
		if comparison.Status != "ahead" || comparison.BaseCommit.SHA != current.Commit || comparison.MergeBaseCommit.SHA != current.Commit {
			return receipt, pollError{"candidate_rejected"}
		}
	}
	var artifacts struct {
		Total     int              `json:"total_count"`
		Artifacts []githubArtifact `json:"artifacts"`
	}
	if err = p.get(ctx, repositoryAPI+fmt.Sprintf("/actions/runs/%d/artifacts?per_page=100", run.ID), &artifacts); err != nil {
		return receipt, err
	}
	if artifacts.Total != len(artifacts.Artifacts) {
		return receipt, pollError{"artifact_unavailable"}
	}
	expected := fmt.Sprintf("vozokany-business-specs-v1-%s-%d-%d", run.HeadSHA, run.ID, run.Attempt)
	matches := []githubArtifact{}
	for _, a := range artifacts.Artifacts {
		if a.Name == expected {
			matches = append(matches, a)
		}
	}
	if len(matches) != 1 {
		return receipt, pollError{"artifact_unavailable"}
	}
	a := matches[0]
	if a.ID <= 0 || a.Expired || !a.ExpiresAt.After(p.Now()) || a.Size <= 0 || a.Size > maxArchiveBytes || a.Run.ID != run.ID || a.Run.RepositoryID != repository.ID || a.Run.HeadRepositoryID != repository.ID || a.Run.HeadBranch != "main" || a.Run.HeadSHA != run.HeadSHA {
		return receipt, pollError{"artifact_unavailable"}
	}
	archive, err := p.request(ctx, http.MethodGet, p.GitHubURL+repositoryAPI+fmt.Sprintf("/actions/artifacts/%d/zip", a.ID), p.GitHubToken, nil, maxArchiveBytes)
	if err != nil {
		return receipt, err
	}
	if Hash(archive) != a.Digest {
		return receipt, pollError{"candidate_rejected"}
	}
	files, err := ReadArtifact(archive)
	if err != nil {
		return receipt, pollError{"candidate_rejected"}
	}
	snapshot, err := Decode(files["snapshot.json"])
	if err != nil {
		return receipt, pollError{"candidate_rejected"}
	}
	if err = snapshot.Eligible(); err != nil {
		return receipt, pollError{"candidate_rejected"}
	}
	if _, err = snapshot.Entries(); err != nil {
		return receipt, pollError{"candidate_rejected"}
	}
	if snapshot.Source.Commit != run.HeadSHA || snapshot.Publication.RunID != receipt.RunID || snapshot.Publication.RunAttempt != run.Attempt || Hash(files["cucumber-messages.ndjson"]) != snapshot.Verification.MessagesHash {
		return receipt, pollError{"candidate_rejected"}
	}
	var execution struct {
		Source struct {
			Commit      string `json:"commit"`
			Dirty       bool   `json:"dirty"`
			Fingerprint string `json:"fingerprint"`
		} `json:"source"`
		Publication  Publication  `json:"publication"`
		Verification Verification `json:"verification"`
	}
	decoder := json.NewDecoder(bytes.NewReader(files["execution.json"]))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&execution) != nil || decoder.Decode(new(any)) != io.EOF || !reflect.DeepEqual(execution.Verification, snapshot.Verification) || !reflect.DeepEqual(execution.Publication, snapshot.Publication) || execution.Source.Commit != snapshot.Source.Commit || execution.Source.Dirty || execution.Source.Fingerprint != Hash([]byte{0}) {
		return receipt, pollError{"candidate_rejected"}
	}
	if err = ValidateMessages(files["cucumber-messages.ndjson"], snapshot); err != nil {
		return receipt, pollError{"candidate_rejected"}
	}
	// Recheck the moving branch/run after downloading: an in-flight older run cannot overwrite a newer main candidate.
	var latest struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err = p.get(ctx, repositoryAPI+"/branches/main", &latest); err != nil {
		return receipt, err
	}
	if latest.Commit.SHA != run.HeadSHA {
		return receipt, pollError{"candidate_rejected"}
	}
	var finalRun githubRun
	if err = p.get(ctx, repositoryAPI+fmt.Sprintf("/actions/runs/%d/attempts/%d", run.ID, run.Attempt), &finalRun); err != nil {
		return receipt, err
	}
	if !reflect.DeepEqual(finalRun, run) {
		return receipt, pollError{"candidate_rejected"}
	}
	_, err = p.request(ctx, http.MethodPost, p.ImportURL+"/internal/v1/repository-snapshots", p.ImportToken, files["snapshot.json"], 1024*1024)
	if err != nil {
		return receipt, pollError{"import_unavailable"}
	}
	receipt.State = "imported"
	receipt.ContentHash = snapshot.Source.ContentHash
	return receipt, nil
}
func ReadArtifact(data []byte) (map[string][]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(archive.File) != 3 {
		return nil, fmt.Errorf("artifact must contain exactly three files")
	}
	files := map[string][]byte{}
	var size uint64
	for _, file := range archive.File {
		if !file.Mode().IsRegular() || !(file.Name == "snapshot.json" || file.Name == "execution.json" || file.Name == "cucumber-messages.ndjson") || files[file.Name] != nil {
			return nil, fmt.Errorf("unsupported artifact entry")
		}
		size += file.UncompressedSize64
		if size > maxExpandedBytes {
			return nil, fmt.Errorf("expanded artifact too large")
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		contents, err := io.ReadAll(io.LimitReader(reader, maxExpandedBytes+1))
		reader.Close()
		if err != nil {
			return nil, err
		}
		files[file.Name] = contents
	}
	return files, nil
}
