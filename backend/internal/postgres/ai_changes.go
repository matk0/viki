package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"viki/internal/governance"
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
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	createdIDs, err := r.applyAIChangeSetTx(ctx, tx, organizationID, userID, changeSet, model.RevisionDraft)
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

func (r *Repository) applyAIChangeSetTx(ctx context.Context, tx pgx.Tx, organizationID, userID string, changeSet model.AIChangeSet, status model.RevisionStatus) ([]string, error) {
	if status != model.RevisionDraft && status != model.RevisionAccepted {
		return nil, fmt.Errorf("unsupported assistant revision status %q", status)
	}
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
			if err := validatePageInput(operation.Kind, operation.PrimitiveKind, operation.ParentID, operation.Content); err != nil {
				return nil, err
			}
			if err := validateParent(ctx, tx, organizationID, operation.Kind, operation.ParentID); err != nil {
				return nil, err
			}
			var primitiveKind any
			if operation.PrimitiveKind != nil {
				primitiveKind = string(*operation.PrimitiveKind)
			}
			var pageID string
			if err := tx.QueryRow(ctx, `
				INSERT INTO pages(organization_id, kind, primitive_kind, parent_id, slug, created_by)
				VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text
			`, organizationID, string(operation.Kind), primitiveKind, operation.ParentID, operation.Slug, userID).Scan(&pageID); err != nil {
				if strings.Contains(err.Error(), "pages_organization_id_slug_key") {
					return nil, store.ErrDuplicateSlug
				}
				return nil, err
			}
			revisionID, err := r.insertRevision(ctx, tx, organizationID, pageID, userID, 1, status, nil, operation.Content)
			if err != nil {
				return nil, err
			}
			if err := updateAssistantPagePointers(ctx, tx, pageID, revisionID, status); err != nil {
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
			var primitiveKind *string
			var parentID, acceptedID, draftID *string
			err := tx.QueryRow(ctx, `
				SELECT kind, primitive_kind, parent_id::text, accepted_revision_id::text, latest_draft_revision_id::text, slug
				FROM pages WHERE id = $1 AND organization_id = $2 FOR UPDATE
			`, *operation.PageID, organizationID).Scan(&kind, &primitiveKind, &parentID, &acceptedID, &draftID, &slug)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, store.ErrNotFound
			}
			if err != nil {
				return nil, err
			}
			if string(operation.Kind) != kind || operation.Slug != slug || !primitiveKindMatches(operation.PrimitiveKind, primitiveKind) || !optionalStringMatches(operation.ParentID, parentID) {
				return nil, fmt.Errorf("revise operation immutable page metadata does not match the existing page")
			}
			currentID := acceptedID
			if draftID != nil {
				currentID = draftID
			}
			if currentID == nil || *currentID != *operation.BaseRevisionID {
				return nil, store.ErrConflict
			}
			if status == model.RevisionAccepted {
				if err := ensurePagePublishable(ctx, tx, *operation.PageID); err != nil {
					return nil, err
				}
			}
			var primitive *model.PrimitiveKind
			if primitiveKind != nil {
				value := model.PrimitiveKind(*primitiveKind)
				primitive = &value
			}
			if err := validatePageInput(model.PageKind(kind), primitive, parentID, operation.Content); err != nil {
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
			if status == model.RevisionAccepted && acceptedID != nil {
				if _, err := tx.Exec(ctx, `UPDATE revisions SET status = 'superseded' WHERE id = $1 AND status = 'accepted'`, *acceptedID); err != nil {
					return nil, err
				}
			}
			revisionID, err := r.insertRevision(ctx, tx, organizationID, *operation.PageID, userID, number, status, operation.BaseRevisionID, operation.Content)
			if err != nil {
				return nil, err
			}
			if err := updateAssistantPagePointers(ctx, tx, *operation.PageID, revisionID, status); err != nil {
				return nil, err
			}
			createdIDs = append(createdIDs, revisionID)
		default:
			return nil, fmt.Errorf("unsupported AI operation %q", operation.Operation)
		}
	}
	return createdIDs, nil
}

func updateAssistantPagePointers(ctx context.Context, tx pgx.Tx, pageID, revisionID string, status model.RevisionStatus) error {
	if status == model.RevisionAccepted {
		_, err := tx.Exec(ctx, `UPDATE pages SET accepted_revision_id = $2, latest_draft_revision_id = NULL, updated_at = now() WHERE id = $1`, pageID, revisionID)
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE pages SET latest_draft_revision_id = $2, updated_at = now() WHERE id = $1`, pageID, revisionID)
	return err
}

func ensurePagePublishable(ctx context.Context, tx pgx.Tx, pageID string) error {
	rows, err := tx.Query(ctx, `SELECT id::text FROM comments WHERE page_id = $1 AND blocking AND resolved_at IS NULL`, pageID)
	if err != nil {
		return err
	}
	defer rows.Close()
	threads := []governance.BlockingThread{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		threads = append(threads, governance.BlockingThread{ID: id})
	}
	return governance.CanPublish(threads)
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

func primitiveKindMatches(expected *model.PrimitiveKind, actual *string) bool {
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
