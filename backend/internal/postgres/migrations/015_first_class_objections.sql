CREATE TABLE objections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    page_id UUID NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    revision_id UUID NOT NULL REFERENCES revisions(id) ON DELETE CASCADE,
    body TEXT NOT NULL CHECK (length(btrim(body)) > 0),
    author_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    resolved_by UUID REFERENCES users(id)
);

INSERT INTO objections(id, page_id, revision_id, body, author_id, created_at, resolved_at, resolved_by)
SELECT id, page_id, revision_id, body, author_id, created_at, resolved_at, resolved_by
FROM comments
WHERE blocking;

DROP TABLE votes;

UPDATE comments child
SET parent_comment_id = NULL
FROM comments parent
WHERE child.parent_comment_id = parent.id
  AND parent.blocking;

DELETE FROM comments WHERE blocking;

DROP INDEX IF EXISTS comments_page_blocking_idx;
ALTER TABLE comments
    DROP COLUMN blocking,
    DROP COLUMN resolved_at,
    DROP COLUMN resolved_by;

CREATE INDEX objections_page_open_idx ON objections(page_id) WHERE resolved_at IS NULL;
CREATE INDEX objections_revision_idx ON objections(revision_id, created_at);
