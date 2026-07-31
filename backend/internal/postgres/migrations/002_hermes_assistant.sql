DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM chat_messages LIMIT 1) THEN
        RAISE EXCEPTION 'cannot migrate legacy assistant data: chat_messages is not empty';
    END IF;
    IF EXISTS (SELECT 1 FROM ai_runs LIMIT 1) THEN
        RAISE EXCEPTION 'cannot migrate legacy assistant data: ai_runs is not empty';
    END IF;
END $$;

DROP TABLE message_citations;
DROP TABLE chat_messages;
DROP TABLE ai_runs;

ALTER TABLE chats RENAME TO assistant_conversations;
ALTER TABLE assistant_conversations
    DROP COLUMN title,
    ADD COLUMN qa_session_id TEXT,
    ADD COLUMN edit_session_id TEXT,
    ADD COLUMN primary_mode TEXT NOT NULL DEFAULT 'qa' CHECK (primary_mode IN ('qa', 'edit')),
    ADD COLUMN last_mode TEXT NOT NULL DEFAULT 'qa' CHECK (last_mode IN ('qa', 'edit')),
    ADD COLUMN qa_handoff_cursor INTEGER NOT NULL DEFAULT 0 CHECK (qa_handoff_cursor >= 0),
    ADD COLUMN edit_handoff_cursor INTEGER NOT NULL DEFAULT 0 CHECK (edit_handoff_cursor >= 0);

CREATE UNIQUE INDEX assistant_conversations_qa_session_idx
    ON assistant_conversations(qa_session_id) WHERE qa_session_id IS NOT NULL;
CREATE UNIQUE INDEX assistant_conversations_edit_session_idx
    ON assistant_conversations(edit_session_id) WHERE edit_session_id IS NOT NULL;
CREATE INDEX assistant_conversations_owner_updated_idx
    ON assistant_conversations(organization_id, user_id, updated_at DESC);
