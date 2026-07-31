CREATE TABLE assistant_draft_proposals (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id),
    user_id UUID NOT NULL REFERENCES users(id),
    conversation_id UUID NOT NULL REFERENCES assistant_conversations(id),
    turn_id UUID NOT NULL UNIQUE,
    summary TEXT NOT NULL,
    changeset JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'awaiting_approval'
        CHECK (status IN ('awaiting_approval', 'published', 'discarded')),
    published_revision_ids UUID[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX assistant_draft_proposals_owner_idx
    ON assistant_draft_proposals(organization_id, user_id, created_at DESC);
