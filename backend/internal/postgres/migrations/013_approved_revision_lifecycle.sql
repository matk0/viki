DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM assistant_draft_proposals
        WHERE status = 'awaiting_approval'
    ) THEN
        RAISE EXCEPTION 'cannot retire assistant proposal lifecycle while pending proposals exist';
    END IF;
END
$$;

ALTER TABLE revisions DROP CONSTRAINT revisions_status_check;
UPDATE revisions SET status = 'approved' WHERE status = 'accepted';
ALTER TABLE revisions
    ADD CONSTRAINT revisions_status_check
    CHECK (status IN ('draft', 'approved', 'superseded'));

ALTER TABLE pages RENAME COLUMN accepted_revision_id TO approved_revision_id;
ALTER TABLE pages RENAME CONSTRAINT pages_accepted_revision_fk TO pages_approved_revision_fk;
ALTER TABLE revisions RENAME COLUMN accepted_at TO approved_at;

UPDATE pages AS page
SET approved_revision_id = candidate.revision_id
FROM (
    SELECT page_id, min(id::text)::uuid AS revision_id
    FROM revisions
    WHERE status = 'approved'
    GROUP BY page_id
    HAVING count(*) = 1
) AS candidate
WHERE page.id = candidate.page_id
  AND page.approved_revision_id IS NULL;

CREATE UNIQUE INDEX revisions_one_draft_per_page_idx
    ON revisions(page_id)
    WHERE status = 'draft';

CREATE UNIQUE INDEX revisions_one_approved_per_page_idx
    ON revisions(page_id)
    WHERE status = 'approved';
