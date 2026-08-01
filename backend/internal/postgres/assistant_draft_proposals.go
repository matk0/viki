package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"viki/internal/governance"
	"viki/internal/model"
	"viki/internal/store"
)

var marshalAssistantJSON = json.Marshal

func (r *Repository) StageAssistantDraftProposal(ctx context.Context, organizationID, userID string, mutation model.AssistantMutationContext, changeSet model.AIChangeSet) (model.AssistantDraftProposal, error) {
	if strings.TrimSpace(changeSet.Clarification) != "" || len(changeSet.Operations) == 0 {
		return model.AssistantDraftProposal{}, fmt.Errorf("assistant proposal requires operations and no clarification")
	}
	if err := validateAssistantChangeSetShape(changeSet); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	payload, err := marshalAssistantJSON(changeSet)
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
			if createdKinds[operation.ParentClientKey] != model.PageFeature {
				return fmt.Errorf("assistant operation %d references an unknown feature parent client key", index+1)
			}
			resolvedParent := operation.ParentClientKey
			parentID = &resolvedParent
		}
		if err := validatePageInput(operation.Kind, operation.ConceptKind, parentID, operation.Content); err != nil {
			return fmt.Errorf("assistant operation %d: %w", index+1, err)
		}
		if operation.Kind != model.PageConcept && len(operation.Content.References) == 0 {
			return fmt.Errorf("assistant operation %d feature or scenario requires concept references", index+1)
		}
		for _, reference := range operation.Content.References {
			hasPageID := strings.TrimSpace(reference.TargetPageID) != ""
			hasClientKey := strings.TrimSpace(reference.TargetClientKey) != ""
			if hasPageID == hasClientKey || strings.TrimSpace(reference.Relation) == "" || strings.TrimSpace(reference.TargetTitle) == "" {
				return fmt.Errorf("assistant operation %d has an invalid page reference", index+1)
			}
			if hasClientKey && createdKinds[reference.TargetClientKey] != model.PageConcept {
				return fmt.Errorf("assistant operation %d references an unknown concept client key", index+1)
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
		SELECT id::text, conversation_id::text, turn_id::text, summary, changeset, operation_reviews, status,
			COALESCE(rejection_reason, ''), published_revision_ids::text[], created_at, updated_at, published_at
		FROM assistant_draft_proposals
		WHERE organization_id = $1 AND user_id = $2 AND status = 'awaiting_approval'
		ORDER BY created_at DESC, id DESC
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
		var reviewPayload []byte
		var publishedIDs []string
		if err := rows.Scan(
			&proposal.ID, &proposal.ConversationID, &proposal.TurnID, &proposal.Summary, &payload, &reviewPayload, &status,
			&proposal.RejectionReason, &publishedIDs, &proposal.CreatedAt, &proposal.UpdatedAt, &proposal.PublishedAt,
		); err != nil {
			return nil, err
		}
		var changeSet model.AIChangeSet
		if err := json.Unmarshal(payload, &changeSet); err != nil {
			return nil, fmt.Errorf("decode assistant proposal: %w", err)
		}
		proposal.Operations = changeSet.Operations
		if err := json.Unmarshal(reviewPayload, &proposal.OperationReviews); err != nil {
			return nil, fmt.Errorf("decode assistant operation reviews: %w", err)
		}
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
	var reviewPayload []byte
	var publishedIDs []string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, conversation_id::text, turn_id::text, summary, changeset, operation_reviews, status,
			COALESCE(rejection_reason, ''), published_revision_ids::text[], created_at, updated_at, published_at
		FROM assistant_draft_proposals
		WHERE id = $1 AND organization_id = $2 AND user_id = $3
	`, proposalID, organizationID, userID).Scan(
		&proposal.ID, &proposal.ConversationID, &proposal.TurnID, &proposal.Summary, &payload, &reviewPayload, &status,
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
	if err := json.Unmarshal(reviewPayload, &proposal.OperationReviews); err != nil {
		return model.AssistantDraftProposal{}, fmt.Errorf("decode assistant operation reviews: %w", err)
	}
	proposal.Status = model.AssistantDraftProposalStatus(status)
	proposal.PublishedRevisions, err = r.revisionsByID(ctx, publishedIDs)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	return proposal, nil
}

func (r *Repository) ReviewAssistantDraftProposalOperation(
	ctx context.Context,
	organizationID, userID, proposalID, operationKey string,
	value model.AssistantOperationReviewValue,
	reason string,
	cascadeDescendants bool,
) (model.AssistantDraftProposal, error) {
	operationKey = strings.TrimSpace(operationKey)
	reason = strings.TrimSpace(reason)
	if value != model.AssistantReviewApprove && value != model.AssistantReviewReject {
		return model.AssistantDraftProposal{}, fmt.Errorf("invalid assistant operation review")
	}
	if value == model.AssistantReviewReject && (reason == "" || len([]rune(reason)) > 2000) {
		return model.AssistantDraftProposal{}, governance.ErrRejectionReasonRequired
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var changePayload, reviewPayload []byte
	var status, conversationID, turnID string
	err = tx.QueryRow(ctx, `
		SELECT changeset, operation_reviews, status, conversation_id::text, turn_id::text
		FROM assistant_draft_proposals
		WHERE id = $1 AND organization_id = $2 AND user_id = $3
		FOR UPDATE
	`, proposalID, organizationID, userID).Scan(&changePayload, &reviewPayload, &status, &conversationID, &turnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AssistantDraftProposal{}, store.ErrNotFound
	}
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if model.AssistantDraftProposalStatus(status) != model.AssistantProposalAwaitingApproval {
		return model.AssistantDraftProposal{}, store.ErrConflict
	}

	var changeSet model.AIChangeSet
	if err := json.Unmarshal(changePayload, &changeSet); err != nil {
		return model.AssistantDraftProposal{}, fmt.Errorf("decode assistant proposal: %w", err)
	}
	var reviews []model.AssistantOperationReview
	if err := json.Unmarshal(reviewPayload, &reviews); err != nil {
		return model.AssistantDraftProposal{}, fmt.Errorf("decode assistant operation reviews: %w", err)
	}
	if !assistantProposalHasOperation(changeSet, operationKey) {
		return model.AssistantDraftProposal{}, store.ErrNotFound
	}
	operationKeys := []string{operationKey}
	if cascadeDescendants {
		operationKeys = assistantProposalOperationTreeKeys(changeSet, operationKey)
	}
	reviewedAt := time.Now().UTC()
	for _, reviewedOperationKey := range operationKeys {
		reviews = upsertAssistantOperationReview(changeSet, reviews, model.AssistantOperationReview{
			OperationKey: reviewedOperationKey,
			Value:        value,
			Reason:       reason,
			ReviewedAt:   reviewedAt,
		})
	}
	nextReviewPayload, err := marshalAssistantJSON(reviews)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	for _, reviewedOperationKey := range operationKeys {
		if err := auditAssistantOperationReview(ctx, tx, organizationID, userID, proposalID, conversationID, turnID, reviews, reviewedOperationKey); err != nil {
			return model.AssistantDraftProposal{}, err
		}
	}

	if len(reviews) != len(changeSet.Operations) {
		if _, err := tx.Exec(ctx, `
			UPDATE assistant_draft_proposals
			SET operation_reviews = $2, updated_at = now()
			WHERE id = $1
		`, proposalID, nextReviewPayload); err != nil {
			return model.AssistantDraftProposal{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return model.AssistantDraftProposal{}, err
		}
		return r.AssistantDraftProposal(ctx, organizationID, userID, proposalID)
	}

	approvedChangeSet, approvedKeys, rejectedKeys, err := approvedAssistantChangeSet(changeSet, reviews)
	if err != nil {
		return model.AssistantDraftProposal{}, err
	}
	createdIDs := []string{}
	nextStatus := model.AssistantProposalDiscarded
	if len(approvedChangeSet.Operations) > 0 {
		createdIDs, err = r.applyAIChangeSetTx(ctx, tx, organizationID, userID, approvedChangeSet, model.RevisionAccepted)
		if err != nil {
			return model.AssistantDraftProposal{}, err
		}
		nextStatus = model.AssistantProposalPublished
	}
	if _, err := tx.Exec(ctx, `
		UPDATE assistant_draft_proposals
		SET operation_reviews = $2,
			status = $3,
			published_revision_ids = $4,
			published_at = CASE WHEN $3 = 'published' THEN now() ELSE NULL END,
			updated_at = now()
		WHERE id = $1
	`, proposalID, nextReviewPayload, nextStatus, createdIDs); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	finalAction := "assistant.proposal_discarded"
	if nextStatus == model.AssistantProposalPublished {
		finalAction = "assistant.proposal_published"
	}
	if err := audit(ctx, tx, organizationID, userID, finalAction, "assistant_draft_proposal", proposalID, map[string]any{
		"conversationId":        conversationID,
		"turnId":                turnID,
		"approvedOperationKeys": approvedKeys,
		"rejectedOperationKeys": rejectedKeys,
		"revisionIds":           createdIDs,
	}); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AssistantDraftProposal{}, err
	}
	return r.AssistantDraftProposal(ctx, organizationID, userID, proposalID)
}

func assistantOperationKey(operation model.AIChangeOperation, index int) string {
	if key := strings.TrimSpace(operation.ClientKey); key != "" {
		return key
	}
	if operation.PageID != nil {
		if key := strings.TrimSpace(*operation.PageID); key != "" {
			return key
		}
	}
	return fmt.Sprintf("operation-%d", index+1)
}

func assistantProposalHasOperation(changeSet model.AIChangeSet, operationKey string) bool {
	for index, operation := range changeSet.Operations {
		if assistantOperationKey(operation, index) == operationKey {
			return true
		}
	}
	return false
}

func assistantProposalOperationTreeKeys(changeSet model.AIChangeSet, rootOperationKey string) []string {
	selected := map[string]bool{rootOperationKey: true}
	for changed := true; changed; {
		changed = false
		for index, operation := range changeSet.Operations {
			key := assistantOperationKey(operation, index)
			if operation.ParentClientKey != "" && selected[operation.ParentClientKey] && !selected[key] {
				selected[key] = true
				changed = true
			}
		}
		for index, operation := range changeSet.Operations {
			if !selected[assistantOperationKey(operation, index)] {
				continue
			}
			for _, reference := range operation.Content.References {
				if reference.TargetClientKey != "" && !selected[reference.TargetClientKey] {
					selected[reference.TargetClientKey] = true
					changed = true
				}
			}
		}
	}

	ordered := make([]string, 0, len(selected))
	for index, operation := range changeSet.Operations {
		key := assistantOperationKey(operation, index)
		if selected[key] {
			ordered = append(ordered, key)
		}
	}
	return ordered
}

func upsertAssistantOperationReview(
	changeSet model.AIChangeSet,
	reviews []model.AssistantOperationReview,
	next model.AssistantOperationReview,
) []model.AssistantOperationReview {
	byKey := make(map[string]model.AssistantOperationReview, len(reviews)+1)
	for _, review := range reviews {
		if assistantProposalHasOperation(changeSet, review.OperationKey) {
			byKey[review.OperationKey] = review
		}
	}
	byKey[next.OperationKey] = next

	ordered := make([]model.AssistantOperationReview, 0, len(byKey))
	for index, operation := range changeSet.Operations {
		if review, ok := byKey[assistantOperationKey(operation, index)]; ok {
			ordered = append(ordered, review)
		}
	}
	return ordered
}

func auditAssistantOperationReview(
	ctx context.Context,
	tx pgx.Tx,
	organizationID, userID, proposalID, conversationID, turnID string,
	reviews []model.AssistantOperationReview,
	operationKey string,
) error {
	for _, review := range reviews {
		if review.OperationKey != operationKey {
			continue
		}
		return audit(ctx, tx, organizationID, userID, "assistant.proposal_operation_reviewed", "assistant_draft_proposal", proposalID, map[string]any{
			"conversationId": conversationID,
			"turnId":         turnID,
			"operationKey":   review.OperationKey,
			"value":          review.Value,
			"reason":         review.Reason,
		})
	}
	return store.ErrNotFound
}

func approvedAssistantChangeSet(
	changeSet model.AIChangeSet,
	reviews []model.AssistantOperationReview,
) (model.AIChangeSet, []string, []string, error) {
	reviewsByKey := make(map[string]model.AssistantOperationReviewValue, len(reviews))
	for _, review := range reviews {
		reviewsByKey[review.OperationKey] = review.Value
	}

	approvedKeys := make([]string, 0, len(reviews))
	rejectedKeys := make([]string, 0, len(reviews))
	approvedOperations := make([]model.AIChangeOperation, 0, len(changeSet.Operations))
	for index, operation := range changeSet.Operations {
		key := assistantOperationKey(operation, index)
		switch reviewsByKey[key] {
		case model.AssistantReviewApprove:
			if operation.ParentClientKey != "" && reviewsByKey[operation.ParentClientKey] != model.AssistantReviewApprove {
				return model.AIChangeSet{}, nil, nil, governance.ErrRejectedProposalDependency
			}
			for _, reference := range operation.Content.References {
				if reference.TargetClientKey != "" && reviewsByKey[reference.TargetClientKey] != model.AssistantReviewApprove {
					return model.AIChangeSet{}, nil, nil, governance.ErrRejectedProposalDependency
				}
			}
			approvedKeys = append(approvedKeys, key)
			approvedOperations = append(approvedOperations, operation)
		case model.AssistantReviewReject:
			rejectedKeys = append(rejectedKeys, key)
		default:
			return model.AIChangeSet{}, nil, nil, fmt.Errorf("assistant proposal operation %q has no review", key)
		}
	}

	approved := model.AIChangeSet{Summary: changeSet.Summary, Operations: approvedOperations}
	if len(approved.Operations) > 0 {
		if err := validateAssistantChangeSetShape(approved); err != nil {
			return model.AIChangeSet{}, nil, nil, err
		}
	}
	return approved, approvedKeys, rejectedKeys, nil
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
