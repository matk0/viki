package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"viki/internal/model"
	"viki/internal/store"
)

func (r *Repository) ApplyAIChangeSet(ctx context.Context, organizationID, userID string, mutation model.AssistantMutationContext, changeSet model.AIChangeSet) ([]model.Revision, error) {
	if strings.TrimSpace(changeSet.Clarification) != "" {
		return []model.Revision{}, nil
	}
	if len(changeSet.Operations) == 0 {
		return nil, fmt.Errorf("AI change set requires at least one operation")
	}
	if err := validateAssistantChangeSetShape(changeSet); err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	createdIDs, err := r.applyAIChangeSetTx(ctx, tx, organizationID, userID, changeSet)
	if err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, organizationID, userID, "assistant.drafts_created", "assistant_conversation", mutation.ConversationID, map[string]any{
		"revisionIds":     createdIDs,
		"conversationId":  mutation.ConversationID,
		"turnId":          mutation.TurnID,
		"hermesProfile":   mutation.HermesProfile,
		"hermesSessionId": mutation.HermesSessionID,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.revisionsByID(ctx, createdIDs)
}

func (r *Repository) applyAIChangeSetTx(ctx context.Context, tx pgx.Tx, organizationID, userID string, changeSet model.AIChangeSet) ([]string, error) {
	createdIDs := make([]string, 0, len(changeSet.Operations))
	createdPages := map[string]string{}
	for _, operation := range changeSet.Operations {
		if operation.ParentID == nil && operation.ParentClientKey != "" {
			resolved := createdPages[operation.ParentClientKey]
			if resolved == "" {
				return nil, fmt.Errorf("unknown parent client key %q", operation.ParentClientKey)
			}
			operation.ParentID = &resolved
		}
		for index := range operation.Content.References {
			reference := &operation.Content.References[index]
			if reference.TargetPageID == "" && reference.TargetClientKey != "" {
				reference.TargetPageID = createdPages[reference.TargetClientKey]
				if reference.TargetPageID == "" {
					return nil, fmt.Errorf("unknown reference client key %q", reference.TargetClientKey)
				}
			}
		}
		switch operation.Operation {
		case "create":
			if !slugPattern.MatchString(operation.Slug) {
				return nil, fmt.Errorf("invalid generated slug %q", operation.Slug)
			}
			if err := validatePageInput(operation.Kind, operation.ConceptKind, operation.ParentID, operation.Content); err != nil {
				return nil, err
			}
			if err := validateParent(ctx, tx, organizationID, operation.Kind, operation.ParentID); err != nil {
				return nil, err
			}
			var conceptKind any
			if operation.ConceptKind != nil {
				conceptKind = string(*operation.ConceptKind)
			}
			var pageID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO pages(organization_id, kind, concept_kind, parent_id, slug, created_by)
				VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text
			`, organizationID, string(operation.Kind), conceptKind, operation.ParentID, operation.Slug, userID).Scan(&pageID); err != nil {
				if strings.Contains(err.Error(), "pages_organization_id_slug_key") {
					return nil, store.ErrDuplicateSlug
				}
				return nil, err
			}
			revisionID, err := r.insertRevision(ctx, tx, organizationID, pageID, userID, 1, nil, operation.Content)
			if err != nil {
				return nil, err
			}
			if err := updateAssistantDraftPointer(ctx, tx, pageID, revisionID); err != nil {
				return nil, err
			}
			createdIDs = append(createdIDs, revisionID)
			if operation.ClientKey != "" {
				createdPages[operation.ClientKey] = pageID
			}
		case "revise":
			if operation.PageID == nil || operation.BaseRevisionID == nil {
				return nil, fmt.Errorf("revise operation requires pageId and baseRevisionId")
			}
			var kind, slug string
			var conceptKind *string
			var parentID, approvedID, draftID *string
			err := tx.QueryRow(ctx, `
				SELECT kind, concept_kind, parent_id::text, approved_revision_id::text, latest_draft_revision_id::text, slug
				FROM pages WHERE id = $1 AND organization_id = $2 FOR UPDATE
			`, *operation.PageID, organizationID).Scan(&kind, &conceptKind, &parentID, &approvedID, &draftID, &slug)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, store.ErrNotFound
			}
			if err != nil {
				return nil, err
			}
			if string(operation.Kind) != kind || operation.Slug != slug || !conceptKindMatches(operation.ConceptKind, conceptKind) || !optionalStringMatches(operation.ParentID, parentID) {
				return nil, fmt.Errorf("revise operation immutable page metadata does not match the existing page")
			}
			currentID := approvedID
			if draftID != nil {
				currentID = draftID
			}
			if currentID == nil || *currentID != *operation.BaseRevisionID {
				return nil, store.ErrConflict
			}
			var concept *model.ConceptKind
			if conceptKind != nil {
				value := model.ConceptKind(*conceptKind)
				concept = &value
			}
			if err := validatePageInput(model.PageKind(kind), concept, parentID, operation.Content); err != nil {
				return nil, err
			}
			var number int
			if err := tx.QueryRow(ctx, `SELECT max(number) + 1 FROM revisions WHERE page_id = $1`, *operation.PageID).Scan(&number); err != nil {
				return nil, err
			}
			if draftID != nil {
				if _, err := tx.Exec(ctx, `UPDATE revisions SET status = 'superseded' WHERE id = $1 AND status = 'draft'`, *draftID); err != nil {
					return nil, err
				}
			}
			revisionID, err := r.insertRevision(ctx, tx, organizationID, *operation.PageID, userID, number, operation.BaseRevisionID, operation.Content)
			if err != nil {
				return nil, err
			}
			if err := updateAssistantDraftPointer(ctx, tx, *operation.PageID, revisionID); err != nil {
				return nil, err
			}
			createdIDs = append(createdIDs, revisionID)
		default:
			return nil, fmt.Errorf("unsupported AI operation %q", operation.Operation)
		}
	}
	return createdIDs, nil
}

func validateAssistantChangeSetShape(changeSet model.AIChangeSet) error {
	createdKinds := make(map[string]model.PageKind, len(changeSet.Operations))
	createdFeatures := make(map[string]int)
	featuresWithScenarios := make(map[string]bool)
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
		if operation.Operation == "create" && operation.Kind == model.PageFeature {
			if operation.ClientKey == "" {
				return fmt.Errorf("assistant operation %d feature requires at least one scenario", index+1)
			}
			createdFeatures[operation.ClientKey] = index
		}
		if operation.Operation == "create" && operation.Kind == model.PageScenario && operation.ParentClientKey != "" {
			featuresWithScenarios[operation.ParentClientKey] = true
		}
	}
	for clientKey, index := range createdFeatures {
		if !featuresWithScenarios[clientKey] {
			return fmt.Errorf("assistant operation %d feature requires at least one scenario", index+1)
		}
	}
	return nil
}

func updateAssistantDraftPointer(ctx context.Context, tx pgx.Tx, pageID, revisionID string) error {
	_, err := tx.Exec(ctx, `UPDATE pages SET latest_draft_revision_id = $2, updated_at = now() WHERE id = $1`, pageID, revisionID)
	return err
}

func (r *Repository) revisionsByID(ctx context.Context, ids []string) ([]model.Revision, error) {
	revisions := make([]model.Revision, 0, len(ids))
	for _, id := range ids {
		revision, err := r.loadRevision(ctx, id)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, nil
}

func conceptKindMatches(expected *model.ConceptKind, actual *string) bool {
	if expected == nil || actual == nil {
		return expected == nil && actual == nil
	}
	return string(*expected) == *actual
}

func optionalStringMatches(expected, actual *string) bool {
	if expected == nil || actual == nil {
		return expected == nil && actual == nil
	}
	return *expected == *actual
}
