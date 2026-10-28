package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"viki/internal/localize"
	"viki/internal/model"
	"viki/internal/specsync"
	"viki/internal/store"
)

func (r *Repository) repositoryLocalizations(ctx context.Context, org, pageID, revisionID string) (map[string]model.Localizations, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT ON(t.revision_id,t.locale) t.revision_id::text,t.locale,t.version,t.source_hash,t.content
 FROM repository_localizations t JOIN repository_revisions r ON r.id=t.revision_id AND r.business_hash=t.source_hash
 JOIN repository_pages p ON p.id=r.page_id WHERE p.organization_id=$1 AND ($2='' OR p.id::text=$2) AND ($3='' OR r.id::text=$3) ORDER BY t.revision_id,t.locale,t.version DESC`, org, pageID, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]model.Localizations{}
	for rows.Next() {
		var id, locale, hash string
		var version int
		var data []byte
		if err = rows.Scan(&id, &locale, &version, &hash, &data); err != nil {
			return nil, err
		}
		var content model.LocalizedContent
		if err = json.Unmarshal(data, &content); err != nil {
			return nil, err
		}
		content.RevisionID = id
		content.SourceHash = hash
		content.Version = version
		if result[id] == nil {
			result[id] = model.Localizations{}
		}
		result[id][locale] = content
	}
	return result, rows.Err()
}

func (r *Repository) ClaimLocalization(ctx context.Context) (*localize.Job, error) {
	_, err := r.pool.Exec(ctx, `INSERT INTO repository_localization_jobs(revision_id)
 SELECT r.id FROM repository_revisions r JOIN repository_pages p ON p.id=r.page_id
 JOIN workspace_repository_sync w ON w.organization_id=p.organization_id
 ON CONFLICT DO NOTHING`)
	if err != nil {
		return nil, err
	}
	job := &localize.Job{}
	var data []byte
	err = r.pool.QueryRow(ctx, `WITH candidate AS (
 SELECT j.revision_id FROM repository_localization_jobs j JOIN repository_revisions r ON r.id=j.revision_id
 JOIN repository_pages p ON p.id=r.page_id WHERE NOT j.completed AND j.attempts<3 AND j.available_at<=now()
 ORDER BY (p.current_revision_id=r.id) DESC,r.created_at,r.id FOR UPDATE OF j SKIP LOCKED LIMIT 1
 ), claimed AS (
 UPDATE repository_localization_jobs SET lease=gen_random_uuid(),attempts=attempts+1,available_at=now()+interval '10 minutes'
 WHERE revision_id=(SELECT revision_id FROM candidate) RETURNING revision_id,lease
 ) SELECT r.id::text,r.business_hash,c.lease::text,r.entry FROM claimed c JOIN repository_revisions r ON r.id=c.revision_id`).Scan(&job.RevisionID, &job.SourceHash, &job.Lease, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entry specsync.Entry
	if err = json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	job.Source = localize.Content{Title: entry.Title, BodyMD: localize.ReadingBody(entry.Title, entry.Body), StepTexts: []string{}}
	for _, step := range entry.Steps {
		job.Source.StepTexts = append(job.Source.StepTexts, step.Text)
	}
	return job, nil
}

func (r *Repository) CompleteLocalization(ctx context.Context, job localize.Job, content localize.Content, translator string) error {
	if err := localize.Validate(job.Source, content); err != nil {
		return err
	}
	data, _ := json.Marshal(content)
	var added int
	err := r.pool.QueryRow(ctx, `WITH owned AS (
 UPDATE repository_localization_jobs j SET completed=true,lease=NULL,last_error=''
 FROM repository_revisions r WHERE j.revision_id=$1 AND r.id=j.revision_id AND r.business_hash=$3
 AND j.lease=$2 AND NOT j.completed AND j.available_at>now() RETURNING j.revision_id
 ), added AS (
 INSERT INTO repository_localizations(revision_id,locale,version,source_hash,content,translator)
 SELECT o.revision_id,'sk',COALESCE((SELECT max(t.version) FROM repository_localizations t WHERE t.revision_id=o.revision_id AND t.locale='sk'),0)+1,$3,$4,$5
 FROM owned o RETURNING 1
 ) SELECT count(*) FROM added`, job.RevisionID, job.Lease, job.SourceHash, data, translator).Scan(&added)
	if err != nil {
		return err
	}
	if added != 1 {
		return store.ErrConflict
	}
	return nil
}
func (r *Repository) FailLocalization(ctx context.Context, job localize.Job) error {
	_, err := r.pool.Exec(ctx, `UPDATE repository_localization_jobs SET lease=NULL,available_at=now()+interval '5 minutes',last_error='translation_failed'
 WHERE revision_id=$1 AND lease=$2 AND NOT completed`, job.RevisionID, job.Lease)
	return err
}
