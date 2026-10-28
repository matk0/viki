package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"viki/internal/localize"
	"viki/internal/model"
	"viki/internal/store"
)

func TestLocalizationPreservesCanonicalHistoryAndIdempotentImports(t *testing.T) {
	r, pool, _, org := syncFixture(t)
	ctx := context.Background()
	base := eligibleFixture(t)
	importOK(t, r, org, base)
	pages, err := r.RepositoryPages(ctx, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, p := range pages {
		ids[p.Slug] = *p.RepositoryRevisionID
	}
	for i := 0; i < 3; i++ {
		job, err := r.ClaimLocalization(ctx)
		if err != nil || job == nil {
			t.Fatal(job, err)
		}
		// Exercise structured fields without changing protected tokens. Real Slovak
		// wording is verified separately in browser/translation acceptance checks.
		content := job.Source
		content.Title = "Slovenský preklad: " + content.Title
		if err = r.CompleteLocalization(ctx, *job, content, "reviewed-test"); err != nil {
			t.Fatal(err)
		}
		if err = r.CompleteLocalization(ctx, *job, content, "reviewed-test"); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("lease replay: %v", err)
		}
	}
	if job, err := r.ClaimLocalization(ctx); err != nil || job != nil {
		t.Fatal(job, err)
	}
	after, err := r.RepositoryPages(ctx, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range after {
		localized := p.Localizations["sk"]
		if localized.RevisionID != ids[p.Slug] || !strings.HasPrefix(localized.Title, "Slovenský") || localized.Version != 1 {
			t.Fatalf("localized page=%+v", p)
		}
		detail, err := r.PageDetail(ctx, org, p.ID)
		if err != nil || detail.RepositoryRevision.ID != ids[p.Slug] || detail.RepositoryRevision.Title != p.Title || detail.RepositoryRevision.Localizations["sk"].RevisionID != ids[p.Slug] {
			t.Fatal(detail, err)
		}
	}
	results, err := r.SearchRepositoryPages(ctx, org, model.SearchOptions{Query: "slovensky"})
	if err != nil || len(results) != 3 {
		t.Fatal(results, err)
	}
	documents, err := r.RetrieveRepositoryPages(ctx, org, "Slovenský", 10)
	if err != nil || len(documents) != 3 || !strings.Contains(documents[0].Content, "Translation (sk)") || documents[0].Localizations["sk"].RevisionID != documents[0].RevisionID {
		t.Fatal(documents, err)
	}
	other, err := r.repositoryLocalizations(ctx, "00000000-0000-4000-8000-000000000099", "", "")
	if err != nil || len(other) != 0 {
		t.Fatal(other, err)
	}
	base.Publication.RunAttempt = 2
	importOK(t, r, org, base)
	if revisionCount(t, pool, org) != 3 {
		t.Fatal("translation or repeated CI created business revisions")
	}
	if job, err := r.ClaimLocalization(ctx); err != nil || job != nil {
		t.Fatal("unchanged import queued work", job, err)
	}
	changed := replacementReadSnapshot(t)
	updateCatalogueHash(&changed)
	importOK(t, r, org, changed)
	newer, err := r.RepositoryPages(ctx, org, nil)
	if err != nil {
		t.Fatal(err)
	}
	missing := 0
	for _, p := range newer {
		if len(p.Localizations) == 0 {
			missing++
		}
	}
	if missing == 0 {
		t.Fatal("old translations attached to changed revisions")
	}
	for _, oldID := range ids {
		revision, err := r.RepositoryRevision(ctx, org, oldID)
		if err != nil || revision.Localizations["sk"].RevisionID != oldID {
			t.Fatal("history lost translation", revision, err)
		}
	}
	// A reviewed correction appends a translation version, preserving the English
	// revision. This is an operator operation, never a public editing capability.
	_, err = pool.Exec(ctx, `UPDATE repository_localization_jobs SET completed=false,attempts=0,available_at=now() WHERE revision_id=$1`, ids[base.Documents[0].ID])
	if err != nil {
		t.Fatal(err)
	}
	for {
		job, err := r.ClaimLocalization(ctx)
		if err != nil || job == nil {
			t.Fatal(job, err)
		}
		content := job.Source
		content.Title = "Opravený preklad: " + content.Title
		if err = r.CompleteLocalization(ctx, *job, content, "reviewed-correction"); err != nil {
			t.Fatal(err)
		}
		if job.RevisionID == ids[base.Documents[0].ID] {
			break
		}
	}
	old, err := r.RepositoryRevision(ctx, org, ids[base.Documents[0].ID])
	if err != nil || old.Localizations["sk"].Version != 2 {
		t.Fatal(old, err)
	}
}

func TestLocalizationLeasesRetriesAndPersistenceFailures(t *testing.T) {
	r, pool, fault, org := syncFixture(t)
	ctx := context.Background()
	importOK(t, r, org, eligibleFixture(t))
	for _, tc := range []struct {
		method, contains string
		run              func() error
	}{
		{"exec", "INSERT INTO repository_localization_jobs", func() error { _, err := r.ClaimLocalization(ctx); return err }},
		{"query_row", "WITH candidate AS", func() error { _, err := r.ClaimLocalization(ctx); return err }},
		{"query", "FROM repository_localizations", func() error { _, err := r.repositoryLocalizations(ctx, org, "", ""); return err }},
	} {
		fault.activate(tc.method, tc.contains, 1)
		requireInjectedFailure(t, tc.run())
		fault.activate("disabled", "", 1)
	}
	for _, read := range []func() error{func() error { _, err := r.repositoryVersions(ctx, org, "", ""); return err }, func() error { _, err := r.RepositoryPages(ctx, org, nil); return err }} {
		fault.activate("query", "FROM repository_localizations", 1)
		requireInjectedFailure(t, read())
		fault.activate("disabled", "", 1)
	}
	job, err := r.ClaimLocalization(ctx)
	if err != nil || job == nil {
		t.Fatal(job, err)
	}
	duplicate, err := r.ClaimLocalization(ctx)
	if err != nil || duplicate == nil || duplicate.RevisionID == job.RevisionID {
		t.Fatal("leased job claimed twice", duplicate, err)
	}
	if err = r.CompleteLocalization(ctx, *job, localize.Content{}, "test"); err == nil {
		t.Fatal("invalid translation accepted")
	}
	fault.activate("query_row", "WITH owned AS", 1)
	requireInjectedFailure(t, r.CompleteLocalization(ctx, *job, job.Source, "test"))
	fault.activate("disabled", "", 1)
	wrong := *job
	wrong.SourceHash = "wrong"
	if err = r.CompleteLocalization(ctx, wrong, job.Source, "test"); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	fault.activate("exec", "UPDATE repository_localization_jobs", 1)
	requireInjectedFailure(t, r.FailLocalization(ctx, *job))
	fault.activate("disabled", "", 1)
	if err = r.FailLocalization(ctx, *job); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE repository_localization_jobs SET completed=true WHERE revision_id<>$1`, job.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if next, err := r.ClaimLocalization(ctx); err != nil || next != nil {
		t.Fatal("backoff ignored", next, err)
	}
	for i := 0; i < 2; i++ {
		_, err = pool.Exec(ctx, `UPDATE repository_localization_jobs SET available_at=now() WHERE revision_id=$1`, job.RevisionID)
		if err != nil {
			t.Fatal(err)
		}
		retry, err := r.ClaimLocalization(ctx)
		if err != nil || retry == nil {
			t.Fatal(retry, err)
		}
		if err = r.FailLocalization(ctx, *retry); err != nil {
			t.Fatal(err)
		}
	}
	_, err = pool.Exec(ctx, `UPDATE repository_localization_jobs SET available_at=now() WHERE revision_id=$1`, job.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if next, err := r.ClaimLocalization(ctx); err != nil || next != nil {
		t.Fatal("retry limit ignored", next, err)
	}
	// Corrupt translation data fails closed instead of replacing English prose.
	_, err = pool.Exec(ctx, `INSERT INTO repository_localizations(revision_id,locale,version,source_hash,content,translator) VALUES($1,'sk',1,$2,'[]','test')`, job.RevisionID, job.SourceHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.repositoryLocalizations(ctx, org, "", ""); err == nil {
		t.Fatal("invalid content decoded")
	}
	_, err = pool.Exec(ctx, `UPDATE repository_localizations SET content='{"title":"A","bodyMd":"","stepTexts":[]}' WHERE revision_id=$1`, job.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"rows_scan", "rows_error"} {
		fault.activate(method, "FROM repository_localizations", 1)
		requireInjectedFailure(t, func() error { _, err := r.repositoryLocalizations(ctx, org, "", ""); return err }())
		fault.activate("disabled", "", 1)
	}
	_, err = pool.Exec(ctx, `DELETE FROM repository_localizations WHERE revision_id=$1`, job.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE repository_localization_jobs SET attempts=0,completed=false,available_at=now() WHERE revision_id=$1`, job.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE repository_revisions SET entry='[]' WHERE id=$1`, job.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.ClaimLocalization(ctx); err == nil {
		t.Fatal("corrupt source accepted")
	}
}
