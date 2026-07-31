package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"viki/internal/model"
	"viki/internal/store"
)

const assistantConversationColumns = `
	id::text, organization_id::text, user_id::text,
	qa_session_id, edit_session_id, primary_mode, last_mode,
	qa_handoff_cursor, edit_handoff_cursor,
	created_at, updated_at`

func (r *Repository) ListAssistantConversations(ctx context.Context, organizationID, userID string) ([]model.AssistantConversation, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+assistantConversationColumns+`
		FROM assistant_conversations
		WHERE organization_id = $1 AND user_id = $2
		ORDER BY updated_at DESC`, organizationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversations := []model.AssistantConversation{}
	for rows.Next() {
		conversation, err := scanAssistantConversation(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, rows.Err()
}

func (r *Repository) CreateAssistantConversation(ctx context.Context, organizationID, userID string, primaryMode model.AssistantMode) (model.AssistantConversation, error) {
	if primaryMode != model.AssistantQA && primaryMode != model.AssistantEdit {
		primaryMode = model.AssistantQA
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO assistant_conversations(organization_id, user_id, primary_mode, last_mode)
		VALUES ($1, $2, $3, $3)
		RETURNING `+assistantConversationColumns, organizationID, userID, string(primaryMode))
	return scanAssistantConversation(row)
}

func (r *Repository) AssistantConversation(ctx context.Context, organizationID, userID, conversationID string) (model.AssistantConversation, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+assistantConversationColumns+`
		FROM assistant_conversations
		WHERE id = $1 AND organization_id = $2 AND user_id = $3`, conversationID, organizationID, userID)
	conversation, err := scanAssistantConversation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AssistantConversation{}, store.ErrNotFound
	}
	return conversation, err
}

func (r *Repository) AssistantConversationBySession(ctx context.Context, mode model.AssistantMode, sessionID string) (model.AssistantConversation, error) {
	column := "qa_session_id"
	if mode == model.AssistantEdit {
		column = "edit_session_id"
	} else if mode != model.AssistantQA {
		return model.AssistantConversation{}, store.ErrNotFound
	}
	row := r.pool.QueryRow(ctx, `SELECT `+assistantConversationColumns+`
		FROM assistant_conversations WHERE `+column+` = $1`, sessionID)
	conversation, err := scanAssistantConversation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AssistantConversation{}, store.ErrNotFound
	}
	return conversation, err
}

func (r *Repository) SetAssistantSession(ctx context.Context, organizationID, userID, conversationID string, mode model.AssistantMode, sessionID string) error {
	column := "qa_session_id"
	if mode == model.AssistantEdit {
		column = "edit_session_id"
	} else if mode != model.AssistantQA {
		return store.ErrNotFound
	}
	command, err := r.pool.Exec(ctx, `UPDATE assistant_conversations
		SET `+column+` = $4, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND user_id = $3`, conversationID, organizationID, userID, sessionID)
	return requireUpdated(command.RowsAffected(), err)
}

func (r *Repository) SetAssistantPrimaryMode(ctx context.Context, organizationID, userID, conversationID string, mode model.AssistantMode) error {
	command, err := r.pool.Exec(ctx, `UPDATE assistant_conversations
		SET primary_mode = $4, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND user_id = $3`, conversationID, organizationID, userID, string(mode))
	return requireUpdated(command.RowsAffected(), err)
}

func (r *Repository) UpdateAssistantMode(ctx context.Context, organizationID, userID, conversationID string, mode model.AssistantMode) error {
	command, err := r.pool.Exec(ctx, `UPDATE assistant_conversations
		SET last_mode = $4, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND user_id = $3`, conversationID, organizationID, userID, string(mode))
	return requireUpdated(command.RowsAffected(), err)
}

func (r *Repository) UpdateAssistantHandoffCursor(ctx context.Context, organizationID, userID, conversationID string, sourceMode model.AssistantMode, cursor int) error {
	column := "qa_handoff_cursor"
	if sourceMode == model.AssistantEdit {
		column = "edit_handoff_cursor"
	} else if sourceMode != model.AssistantQA {
		return store.ErrNotFound
	}
	command, err := r.pool.Exec(ctx, `UPDATE assistant_conversations
		SET `+column+` = $4, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND user_id = $3`, conversationID, organizationID, userID, cursor)
	return requireUpdated(command.RowsAffected(), err)
}

type assistantConversationScanner interface {
	Scan(...any) error
}

func scanAssistantConversation(scanner assistantConversationScanner) (model.AssistantConversation, error) {
	var conversation model.AssistantConversation
	var primaryMode string
	var lastMode string
	err := scanner.Scan(
		&conversation.ID,
		&conversation.OrganizationID,
		&conversation.UserID,
		&conversation.QASessionID,
		&conversation.EditSessionID,
		&primaryMode,
		&lastMode,
		&conversation.QAHandoffCursor,
		&conversation.EditHandoffCursor,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
	)
	conversation.PrimaryMode = model.AssistantMode(primaryMode)
	conversation.LastMode = model.AssistantMode(lastMode)
	return conversation, err
}

func requireUpdated(rowsAffected int64, err error) error {
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *Repository) AssistantDraftReceipts(ctx context.Context, organizationID, conversationID string) (map[string][]model.AssistantDraftReceipt, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT a.metadata->>'turnId', r.id::text, p.id::text, r.title
		FROM audit_events a
		CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(a.metadata->'revisionIds', '[]'::jsonb)) AS receipt(revision_id)
		JOIN revisions r ON r.id = receipt.revision_id::uuid
		JOIN pages p ON p.id = r.page_id AND p.organization_id = a.organization_id
		WHERE a.organization_id = $1
		  AND a.action IN ('assistant.drafts_created', 'ai.drafts_created')
		  AND a.entity_type = 'assistant_conversation'
		  AND a.entity_id = $2
		ORDER BY a.created_at, r.number
	`, organizationID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]model.AssistantDraftReceipt{}
	for rows.Next() {
		var turnID string
		var receipt model.AssistantDraftReceipt
		if err := rows.Scan(&turnID, &receipt.RevisionID, &receipt.PageID, &receipt.PageTitle); err != nil {
			return nil, err
		}
		if turnID != "" {
			result[turnID] = append(result[turnID], receipt)
		}
	}
	return result, rows.Err()
}
