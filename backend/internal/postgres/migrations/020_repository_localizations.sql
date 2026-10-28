-- Translations are derived presentations, never business revisions or approvals.
CREATE TABLE repository_localizations (
 revision_id UUID NOT NULL REFERENCES repository_revisions(id),
 locale TEXT NOT NULL CHECK(locale='sk'),
 version INTEGER NOT NULL CHECK(version>0),
 source_hash TEXT NOT NULL,
 content JSONB NOT NULL,
 translator TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(revision_id,locale,version)
);
CREATE TABLE repository_localization_jobs (
 revision_id UUID PRIMARY KEY REFERENCES repository_revisions(id),
 attempts INTEGER NOT NULL DEFAULT 0,
 lease UUID,
 available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 completed BOOLEAN NOT NULL DEFAULT false,
 last_error TEXT NOT NULL DEFAULT ''
);
