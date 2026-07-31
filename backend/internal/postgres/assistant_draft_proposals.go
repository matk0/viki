package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"viki/internal/governance"
	"viki/internal/model"
	"viki/internal/store"
)

func (r *Repository) StageAssistantDraftProposal(ctx context.Context, organizationID, userID string, mutation model.AssistantMutationContext, changeSet model.AIChangeSet) (model.AssistantDraftProposal, error) {
	if strings.TrimSpace(changeSet.Clarification) != "" || len(changeSet.Operations) == 0 {
		return model.AssistantDraftProposal{}, fmt.Errorf("assistant proposal requires operations and no clarification")
	}
	if err := validateAssistantChangeSetShape(changeSet); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	payload, err := json.Marshal(changeSet)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `
		INSERT INTO assistant_draft_proposals(
			id, organization_id, user_id, conversation_id, turn_id, summary, changeset
		)
		SELECT $4, $1, $2, c.id, $4, $5, $6
		FROM assistant_conversations c
		WHERE c.id = $3 AND c.organization_id = $1 AND c.user_id = $2
		ON CONFLICT (id) DO NOTHING
	`, organizationID, userID, mutation.ConversationID, mutation.TurnID, strings.TrimSpace(changeSet.Summary), payload)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if command.RowsAffected() == 1 {
		if err := audit(ctx, tx, organizationID, userID, "assistant.proposal_created", "assistant_draft_proposal", mutation.TurnID, map[string]any{
			"conversationId":  mutation.ConversationID,
			"turnId":          mutation.TurnID,
			"hermesProfile":   mutation.HermesProfile,
			"hermesSessionId": mutation.HermesSessionID,
		}); err != nil {
			return model.AssistantDraftProposal{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	return r.AssistantDraftProposal(ctx, organizationID, userID, mutation.TurnID)
}

func validateAssistantChangeSetShape(changeSet model.AIChangeSet) error {
	createdKinds := make(map[string]model.PageKind, len(changeSet.Operations))
	for index, operation := range changeSet.Operations {
		switch operation.Operation {
		case "create":
			if operation.PageID != nil || operation.BaseRevisionID != nil {
				return fmt.Errorf("assistant operation %d create cannot contain pageId or baseRevisionId", index+1)
			}
		case "revise":
			if operation.PageID == nil || operation.BaseRevisionID == nil {
				return fmt.Errorf("assistant operation %d revise requires pageId and baseRevisionId", index+1)
			}
		default:
			return fmt.Errorf("assistant operation %d has unsupported operation %q", index+1, operation.Operation)
		}

		parentID := operation.ParentID
		if operation.ParentClientKey != "" {
			if createdKinds[operation.ParentClientKey] != model.PageScenario {
				return fmt.Errorf("assistant operation %d references an unknown scenario parent client key", index+1)
			}
			resolvedParent := operation.ParentClientKey
			parentID = &resolvedParent
		}
		if err := validatePageInput(operation.Kind, operation.PrimitiveKind, parentID, operation.Content); err != nil {
			return fmt.Errorf("assistant operation %d: %w", index+1, err)
		}
		if operation.Kind != model.PagePrimitive && len(operation.Content.References) == 0 {
			return fmt.Errorf("assistant operation %d scenario requires primitive references", index+1)
		}
		for _, reference := range operation.Content.References {
			hasPageID := strings.TrimSpace(reference.TargetPageID) != ""
			hasClientKey := strings.TrimSpace(reference.TargetClientKey) != ""
			if hasPageID == hasClientKey || strings.TrimSpace(reference.Relation) == "" || strings.TrimSpace(reference.TargetTitle) == "" {
				return fmt.Errorf("assistant operation %d has an invalid page reference", index+1)
			}
			if hasClientKey && createdKinds[reference.TargetClientKey] != model.PagePrimitive {
				return fmt.Errorf("assistant operation %d references an unknown primitive client key", index+1)
			}
		}
		if operation.ClientKey != "" {
			if _, duplicate := createdKinds[operation.ClientKey]; duplicate {
				return fmt.Errorf("assistant operation %d repeats client key %q", index+1, operation.ClientKey)
			}
			createdKinds[operation.ClientKey] = operation.Kind
		}
	}
	return nil
}

func (r *Repository) ListAssistantDraftProposals(ctx context.Context, organizationID, userID string) ([]model.AssistantDraftProposal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, conversation_id::text, turn_id::text, summary, changeset, status,
			COALESCE(rejection_reason, ''), published_revision_ids::text[], created_at, updated_at, published_at
		FROM assistant_draft_proposals
		WHERE organization_id = $1 AND user_id = $2
		ORDER BY CASE WHEN status = 'awaiting_approval' THEN 0 ELSE 1 END, created_at DESC, id DESC
	`, organizationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	proposals := make([]model.AssistantDraftProposal, 0)
	for rows.Next() {
		var proposal model.AssistantDraftProposal
		var status string
		var payload []byte
		var publishedIDs []string
		if err := rows.Scan(
			&proposal.ID, &proposal.ConversationID, &proposal.TurnID, &proposal.Summary, &payload, &status,
			&proposal.RejectionReason, &publishedIDs, &proposal.CreatedAt, &proposal.UpdatedAt, &proposal.PublishedAt,
		); err != nil {
			return nil, err
		}
		var changeSet model.AIChangeSet
		if err := json.Unmarshal(payload, &changeSet); err != nil {
			return nil, fmt.Errorf("decode assistant proposal: %w", err)
		}
		proposal.Operations = changeSet.Operations
		proposal.Status = model.AssistantDraftProposalStatus(status)
		proposal.PublishedRevisions, err = r.revisionsByID(ctx, publishedIDs)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, rows.Err()
}

func (r *Repository) AssistantDraftProposal(ctx context.Context, organizationID, userID, proposalID string) (model.AssistantDraftProposal, error) {
	var proposal model.AssistantDraftProposal
	var status string
	var payload []byte
	var publishedIDs []string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, conversation_id::text, turn_id::text, summary, changeset, status,
			COALESCE(rejection_reason, ''), published_revision_ids::text[], created_at, updated_at, published_at
		FROM assistant_draft_proposals
		WHERE id = $1 AND organization_id = $2 AND user_id = $3
	`, proposalID, organizationID, userID).Scan(
		&proposal.ID, &proposal.ConversationID, &proposal.TurnID, &proposal.Summary, &payload, &status,
		&proposal.RejectionReason, &publishedIDs, &proposal.CreatedAt, &proposal.UpdatedAt, &proposal.PublishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AssistantDraftProposal{}, store.ErrNotFound
	}
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	var changeSet model.AIChangeSet
	if err := json.Unmarshal(payload, &changeSet); err != nil {
		return model.AssistantDraftProposal{}, fmt.Errorf("decode assistant proposal: %w", err)
	}
	proposal.Operations = changeSet.Operations
	proposal.Status = model.AssistantDraftProposalStatus(status)
	proposal.PublishedRevisions, err = r.revisionsByID(ctx, publishedIDs)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	return proposal, nil
}

func (r *Repository) PublishAssistantDraftProposal(ctx context.Context, organizationID, userID, proposalID string) (model.AssistantDraftProposal, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var payload []byte
	var status string
	var conversationID, turnID string
	err = tx.QueryRow(ctx, `
		SELECT changeset, status, conversation_id::text, turn_id::text
		FROM assistant_draft_proposals
		WHERE id = $1 AND organization_id = $2 AND user_id = $3
		FOR UPDATE
	`, proposalID, organizationID, userID).Scan(&payload, &status, &conversationID, &turnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AssistantDraftProposal{}, store.ErrNotFound
	}
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if model.AssistantDraftProposalStatus(status) == model.AssistantProposalPublished {
		_ = tx.Rollback(ctx)
		return r.AssistantDraftProposal(ctx, organizationID, userID, proposalID)
	}
	if model.AssistantDraftProposalStatus(status) != model.AssistantProposalAwaitingApproval {
		return model.AssistantDraftProposal{}, store.ErrConflict
	}
	var changeSet model.AIChangeSet
	if err := json.Unmarshal(payload, &changeSet); err != nil {
		return model.AssistantDraftProposal{}, fmt.Errorf("decode assistant proposal: %w", err)
	}
	createdIDs, err := r.applyAIChangeSetTx(ctx, tx, organizationID, userID, changeSet, model.RevisionAccepted)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE assistant_draft_proposals
		SET status = 'published', published_revision_ids = $2, published_at = now(), updated_at = now()
		WHERE id = $1
	`, proposalID, createdIDs); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if err := audit(ctx, tx, organizationID, userID, "assistant.proposal_published", "assistant_draft_proposal", proposalID, map[string]any{
		"conversationId": conversationID,
		"turnId":         turnID,
		"revisionIds":    createdIDs,
	}); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	return r.AssistantDraftProposal(ctx, organizationID, userID, proposalID)
}

func (r *Repository) DiscardAssistantDraftProposal(ctx context.Context, organizationID, userID, proposalID, reason string) (model.AssistantDraftProposal, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 2000 {
		return model.AssistantDraftProposal{}, governance.ErrRejectionReasonRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `
		UPDATE assistant_draft_proposals
		SET status = 'discarded', rejection_reason = $4, updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND user_id = $3 AND status = 'awaiting_approval'
	`, proposalID, organizationID, userID, reason)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if command.RowsAffected() == 0 {
		return model.AssistantDraftProposal{}, store.ErrConflict
	}
	if err := audit(ctx, tx, organizationID, userID, "assistant.proposal_discarded", "assistant_draft_proposal", proposalID, map[string]any{"reason": reason}); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	return r.AssistantDraftProposal(ctx, organizationID, userID, proposalID)
}
