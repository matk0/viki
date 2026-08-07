CREATE TABLE scenario_developments (
    revision_id UUID PRIMARY KEY REFERENCES revisions(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'developed', 'blocked')),
    detail TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
