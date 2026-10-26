package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"viki/internal/model"
	"viki/internal/specsync"
	"viki/internal/store"
)

func (r *Repository) ConfigureInitialRepositoryWorkspace(ctx context.Context, repository string) error {
	return r.ConfigureRepositoryWorkspace(ctx, defaultOrganizationID, repository)
}

func (r *Repository) ConfigureRepositoryWorkspace(ctx context.Context, organizationID, repository string) error {
	if repository != "" && repository != specsync.Repository {
		return fmt.Errorf("unsupported Git repository")
	}
	if repository == "" {
		return nil
	}
	// Do not silently hide an existing human-governed catalogue when changing modes.
	_, err := r.pool.Exec(ctx, `INSERT INTO workspace_repository_sync(organization_id,repository)
 SELECT $1,$2 WHERE NOT EXISTS(SELECT 1 FROM pages WHERE organization_id=$1)
 ON CONFLICT(organization_id) DO UPDATE SET repository=EXCLUDED.repository`, organizationID, repository)
	if err != nil {
		return err
	}
	status, err := r.WorkspaceSyncStatus(ctx, organizationID)
	if err != nil {
		return err
	}
	if status.Mode != "git_managed" {
		return fmt.Errorf("cannot enable Git mode over existing human-authored pages")
	}
	return nil
}
func (r *Repository) repositoryManaged(ctx context.Context, org string) (bool, error) {
	var managed bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workspace_repository_sync WHERE organization_id=$1)`, org).Scan(&managed)
	return managed, err
}
func (r *Repository) WorkspaceSyncStatus(ctx context.Context, org string) (model.WorkspaceSyncStatus, error) {
	status := model.WorkspaceSyncStatus{Mode: "human_governed"}
	err := r.pool.QueryRow(ctx, `SELECT w.repository,w.last_checked_at,w.last_error,w.last_error_at,s.imported_at,COALESCE(s.source_commit,''),COALESCE(s.run_id::text,''),COALESCE(s.run_attempt,0),COALESCE(s.content_hash,''),
 (SELECT count(*) FROM repository_pages p WHERE p.organization_id=w.organization_id AND NOT p.archived),
 (SELECT count(*) FROM repository_pages p WHERE p.organization_id=w.organization_id AND NOT p.archived AND p.kind='scenario')
 FROM workspace_repository_sync w LEFT JOIN repository_snapshots s ON s.id=w.current_snapshot_id WHERE w.organization_id=$1`, org).Scan(&status.Repository, &status.LastCheckedAt, &status.LastError, &status.LastErrorAt, &status.LastImportedAt, &status.Commit, &status.RunID, &status.RunAttempt, &status.ContentHash, &status.PageCount, &status.ScenarioCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Mode = "git_managed"
	status.Stale = status.LastCheckedAt == nil || time.Since(*status.LastCheckedAt) > 15*time.Minute || status.LastError != ""
	return status, nil
}
func (r *Repository) RecordRepositoryPoll(ctx context.Context, org, errorCode string) error {
	_, err := r.pool.Exec(ctx, `UPDATE workspace_repository_sync SET last_checked_at=now(),last_error=$2,last_error_at=CASE WHEN $2='' THEN NULL ELSE now() END WHERE organization_id=$1`, org, errorCode)
	return err
}
func (r *Repository) ImportRepositorySnapshot(ctx context.Context, org string, s specsync.Snapshot) (model.WorkspaceSyncStatus, error) {
	// Validate the complete wire contract here as well, including direct Go callers.
	raw, _ := json.Marshal(s)
	validated, decodeErr := specsync.Decode(raw)
	if decodeErr != nil {
		return model.WorkspaceSyncStatus{}, decodeErr
	}
	s = validated
	if err := s.Eligible(); err != nil {
		return model.WorkspaceSyncStatus{}, err
	}
	entries, err := s.Entries()
	if err != nil {
		return model.WorkspaceSyncStatus{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.WorkspaceSyncStatus{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var repository string
	var currentID *string
	if err = tx.QueryRow(ctx, `SELECT repository,current_snapshot_id::text FROM workspace_repository_sync WHERE organization_id=$1 FOR UPDATE`, org).Scan(&repository, &currentID); err != nil {
		return model.WorkspaceSyncStatus{}, err
	}
	if repository != s.Source.Repository {
		return model.WorkspaceSyncStatus{}, fmt.Errorf("snapshot repository differs from workspace")
	}
	runID, _ := strconv.ParseInt(s.Publication.RunID, 10, 64)
	if currentID != nil {
		var lastRun int64
		var lastAttempt int
		var lastRaw []byte
		if err = tx.QueryRow(ctx, `SELECT run_id,run_attempt,snapshot FROM repository_snapshots WHERE id=$1`, *currentID).Scan(&lastRun, &lastAttempt, &lastRaw); err != nil {
			return model.WorkspaceSyncStatus{}, err
		}
		if runID < lastRun || (runID == lastRun && s.Publication.RunAttempt < lastAttempt) {
			return model.WorkspaceSyncStatus{}, fmt.Errorf("stale snapshot")
		}
		if runID == lastRun && s.Publication.RunAttempt == lastAttempt {
			var last specsync.Snapshot
			if err = json.Unmarshal(lastRaw, &last); err != nil {
				return model.WorkspaceSyncStatus{}, err
			}
			old, _ := json.Marshal(last)
			if string(old) != string(raw) {
				return model.WorkspaceSyncStatus{}, fmt.Errorf("immutable run identity has conflicting snapshot")
			}
			if _, err = tx.Exec(ctx, `UPDATE workspace_repository_sync SET last_checked_at=now(),last_error='',last_error_at=NULL WHERE organization_id=$1`, org); err != nil {
				return model.WorkspaceSyncStatus{}, err
			}
			if err = tx.Commit(ctx); err != nil {
				return model.WorkspaceSyncStatus{}, err
			}
			return r.WorkspaceSyncStatus(ctx, org)
		}
	}
	var snapshotID string
	if err = tx.QueryRow(ctx, `INSERT INTO repository_snapshots(organization_id,source_commit,content_hash,run_id,run_attempt,snapshot) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, org, s.Source.Commit, s.Source.ContentHash, runID, s.Publication.RunAttempt, raw).Scan(&snapshotID); err != nil {
		return model.WorkspaceSyncStatus{}, err
	}
	// A snapshot is complete: old entries are tombstoned, never removed.
	if _, err = tx.Exec(ctx, `UPDATE repository_pages SET archived=true WHERE organization_id=$1`, org); err != nil {
		return model.WorkspaceSyncStatus{}, err
	}
	for _, entry := range entries {
		var pageID string
		if err = tx.QueryRow(ctx, `INSERT INTO repository_pages(organization_id,external_id,kind) VALUES($1,$2,$3) ON CONFLICT(organization_id,external_id) DO UPDATE SET archived=false WHERE repository_pages.kind=EXCLUDED.kind RETURNING id::text`, org, entry.ExternalID, entry.Kind).Scan(&pageID); err != nil {
			return model.WorkspaceSyncStatus{}, fmt.Errorf("source identity changed kind or page unavailable: %w", err)
		}
		data, _ := json.Marshal(entry)
		var revisionID string
		err = tx.QueryRow(ctx, `SELECT id::text FROM repository_revisions WHERE page_id=$1 AND business_hash=$2`, pageID, entry.BusinessHash).Scan(&revisionID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `INSERT INTO repository_revisions(page_id,number,business_hash,entry) SELECT $1,COALESCE(max(number),0)+1,$2,$3 FROM repository_revisions WHERE page_id=$1 RETURNING id::text`, pageID, entry.BusinessHash, data).Scan(&revisionID)
		}
		if err != nil {
			return model.WorkspaceSyncStatus{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO repository_snapshot_entries(snapshot_id,page_id,revision_id,entry) VALUES($1,$2,$3,$4)`, snapshotID, pageID, revisionID, data); err != nil {
			return model.WorkspaceSyncStatus{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE repository_pages SET current_revision_id=$2,archived=false WHERE id=$1`, pageID, revisionID); err != nil {
			return model.WorkspaceSyncStatus{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE workspace_repository_sync SET current_snapshot_id=$2,last_checked_at=now(),last_error='',last_error_at=NULL WHERE organization_id=$1`, org, snapshotID); err != nil {
		return model.WorkspaceSyncStatus{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.WorkspaceSyncStatus{}, err
	}
	return r.WorkspaceSyncStatus(ctx, org)
}

// repositoryVersions returns only immutable entries and their verified snapshot provenance.
func (r *Repository) repositoryVersions(ctx context.Context, org, pageID, revisionID string) ([]model.Revision, error) {
	rows, err := r.pool.Query(ctx, `SELECT r.id::text,r.page_id::text,r.number,r.created_at,
 COALESCE(e.entry,r.entry),s.source_commit,s.run_id::text,s.run_attempt,s.imported_at,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('targetPageId',target.id::text,'targetTitle',related.entry->>'title','relation','uses') ORDER BY target.external_id)
 FROM repository_snapshot_entries related JOIN repository_pages target ON target.id=related.page_id
 WHERE related.snapshot_id=s.id AND target.external_id IN(SELECT jsonb_array_elements_text(e.entry->'conceptIds'))),'[]'::jsonb)
 FROM repository_revisions r JOIN repository_pages p ON p.id=r.page_id
 JOIN LATERAL (SELECT se.entry,se.snapshot_id FROM repository_snapshot_entries se JOIN repository_snapshots ss ON ss.id=se.snapshot_id WHERE se.revision_id=r.id ORDER BY ss.run_id DESC,ss.run_attempt DESC LIMIT 1) e ON true
 JOIN repository_snapshots s ON s.id=e.snapshot_id WHERE p.organization_id=$1 AND ($2='' OR p.id::text=$2) AND ($3='' OR r.id::text=$3) ORDER BY r.number DESC`, org, pageID, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Revision{}
	for rows.Next() {
		var rev model.Revision
		var entry specsync.Entry
		var data, references []byte
		source := model.RepositorySource{Repository: specsync.Repository, Result: "passed"}
		if err = rows.Scan(&rev.ID, &rev.PageID, &rev.Number, &rev.CreatedAt, &data, &source.Commit, &source.RunID, &source.RunAttempt, &source.ImportedAt, &references); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &entry); err != nil {
			return nil, err
		}
		source.ExternalID = entry.ExternalID
		source.Path = entry.Path
		source.Content = entry.SourceContent
		source.ContentHash = entry.DocumentHash
		source.VerifiedCommit = source.Commit
		rev.RepositorySource = &source
		rev.Status = model.RevisionRepositoryBaseline
		rev.Title = entry.Title
		rev.BodyMD = entry.Body
		rev.CreatedBy = model.User{DisplayName: "Repository baseline"}
		rev.Steps = []model.Step{}
		// References are a typed JSON projection constructed by the database query.
		_ = json.Unmarshal(references, &rev.References)
		for i, step := range entry.Steps {
			rev.Steps = append(rev.Steps, model.Step{Position: i, Keyword: model.BDDKeyword(strings.ToLower(step.Keyword)), Text: step.Text, Arguments: []string{}})
		}
		result = append(result, rev)
	}
	return result, rows.Err()
}
func (r *Repository) RepositoryPages(ctx context.Context, org string, kind *model.PageKind) ([]model.Page, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.id::text,p.kind,p.current_revision_id::text,p.created_at,e.entry,s.source_commit,s.run_id::text,s.run_attempt,s.imported_at
 FROM repository_pages p JOIN workspace_repository_sync w ON w.organization_id=p.organization_id JOIN repository_snapshots s ON s.id=w.current_snapshot_id JOIN repository_snapshot_entries e ON e.snapshot_id=s.id AND e.page_id=p.id
 WHERE p.organization_id=$1 AND NOT p.archived ORDER BY p.external_id`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pages := []model.Page{}
	parents := map[string]string{}
	externalParents := map[string]string{}
	for rows.Next() {
		var page model.Page
		var entry specsync.Entry
		var data []byte
		source := model.RepositorySource{Repository: specsync.Repository, Result: "passed"}
		if err = rows.Scan(&page.ID, &page.Kind, &page.RepositoryRevisionID, &page.CreatedAt, &data, &source.Commit, &source.RunID, &source.RunAttempt, &source.ImportedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &entry); err != nil {
			return nil, err
		}
		page.Title = entry.Title
		page.Slug = entry.ExternalID
		page.UpdatedAt = source.ImportedAt
		source.ExternalID = entry.ExternalID
		source.Path = entry.Path
		source.ContentHash = entry.DocumentHash
		source.VerifiedCommit = source.Commit
		page.RepositorySource = &source
		parents[entry.ExternalID] = page.ID
		externalParents[page.ID] = entry.ParentID
		pages = append(pages, page)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	result := []model.Page{}
	for _, p := range pages {
		if id := parents[externalParents[p.ID]]; id != "" {
			p.ParentID = &id
		}
		if kind == nil || p.Kind == *kind {
			result = append(result, p)
		}
	}
	return result, nil
}
func (r *Repository) repositoryPageDetail(ctx context.Context, org, pageID string) (model.PageDetail, error) {
	var page model.Page
	var entry specsync.Entry
	var data []byte
	err := r.pool.QueryRow(ctx, `SELECT p.id::text,p.kind,p.current_revision_id::text,p.archived,p.created_at,r.entry,parent.id::text FROM repository_pages p JOIN repository_revisions r ON r.id=p.current_revision_id LEFT JOIN repository_pages parent ON parent.organization_id=p.organization_id AND parent.external_id=r.entry->>'parentId' WHERE p.organization_id=$1 AND p.id=$2`, org, pageID).Scan(&page.ID, &page.Kind, &page.RepositoryRevisionID, &page.Archived, &page.CreatedAt, &data, &page.ParentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.PageDetail{}, store.ErrNotFound
	}
	if err != nil {
		return model.PageDetail{}, err
	}
	if err = json.Unmarshal(data, &entry); err != nil {
		return model.PageDetail{}, err
	}
	page.Slug = entry.ExternalID
	page.Title = entry.Title
	versions, err := r.repositoryVersions(ctx, org, pageID, "")
	if err != nil {
		return model.PageDetail{}, err
	}
	detail := model.PageDetail{Page: page, Revisions: []model.RevisionSummary{}, Comments: []model.Comment{}, Objections: []model.Objection{}, Children: []model.Page{}, ReviewStates: []model.RevisionReviewState{}}
	for _, v := range versions {
		detail.Revisions = append(detail.Revisions, model.RevisionSummary{ID: v.ID, Number: v.Number, Status: v.Status, Title: v.Title, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt})
		if v.ID == *page.RepositoryRevisionID {
			copy := v
			detail.RepositoryRevision = &copy
			detail.Page.RepositorySource = v.RepositorySource
			detail.Page.UpdatedAt = v.RepositorySource.ImportedAt
		}
	}
	pages, err := r.RepositoryPages(ctx, org, nil)
	if err != nil {
		return model.PageDetail{}, err
	}
	for _, p := range pages {
		if p.Slug == entry.ParentID {
			id := p.ID
			detail.Page.ParentID = &id
		}
		if p.ParentID != nil && *p.ParentID == pageID {
			detail.Children = append(detail.Children, p)
		}
	}
	return detail, nil
}
func (r *Repository) RepositoryRevision(ctx context.Context, org, revisionID string) (model.Revision, error) {
	versions, err := r.repositoryVersions(ctx, org, "", revisionID)
	if err != nil {
		return model.Revision{}, err
	}
	if len(versions) == 0 {
		return model.Revision{}, store.ErrNotFound
	}
	return versions[0], nil
}

func (r *Repository) searchRepositoryPages(ctx context.Context, org string, options model.SearchOptions) ([]model.SearchResult, error) {
	pages, err := r.RepositoryPages(ctx, org, options.Kind)
	if err != nil {
		return nil, err
	}
	results := []model.SearchResult{}
	query := strings.ToLower(strings.TrimSpace(options.Query))
	for _, p := range pages {
		versions, err := r.repositoryVersions(ctx, org, p.ID, "")
		if err != nil {
			return nil, err
		}
		for _, v := range versions {
			if v.ID != *p.RepositoryRevisionID {
				continue
			}
			text := v.Title + "\n" + v.BodyMD
			for _, st := range v.Steps {
				text += "\n" + st.Text
			}
			if query == "" || strings.Contains(strings.ToLower(text), query) {
				results = append(results, model.SearchResult{Page: p, RevisionID: v.ID, Excerpt: v.BodyMD, Score: 1, Draft: false})
			}
			break
		}
	}
	if options.Limit > 0 && len(results) > options.Limit {
		results = results[:options.Limit]
	}
	return results, nil
}
func (r *Repository) retrieveRepositoryPages(ctx context.Context, org, query string, limit int) ([]model.RetrievedDocument, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	results, err := r.SearchRepositoryPages(ctx, org, model.SearchOptions{Query: query, Limit: limit})
	if err != nil {
		return nil, err
	}
	documents := []model.RetrievedDocument{}
	for _, result := range results {
		v, err := r.RepositoryRevision(ctx, org, result.RevisionID)
		if err != nil {
			return nil, err
		}
		content := v.Title + "\n" + v.BodyMD
		for _, step := range v.Steps {
			content += "\n" + strings.ToUpper(string(step.Keyword)) + " " + step.Text
		}
		content += "\nRepository baseline; source commit " + v.RepositorySource.Commit + "; verified at " + v.RepositorySource.VerifiedCommit + "; not human approval or production deployment."
		documents = append(documents, model.RetrievedDocument{RevisionID: v.ID, PageID: v.PageID, PageTitle: v.Title, Content: content, Score: 1, Draft: false})
	}
	return documents, nil
}
