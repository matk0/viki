ALTER TABLE assistant_draft_proposals
ADD COLUMN operation_reviews JSONB NOT NULL DEFAULT '[]'::jsonb;
