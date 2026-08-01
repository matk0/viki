package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"viki/internal/model"
	"viki/internal/store"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func validatePageInput(kind model.PageKind, conceptKind *model.ConceptKind, parentID *string, content model.RevisionContent) error {
	if strings.TrimSpace(content.Title) == "" {
		return fmt.Errorf("title is required")
	}
	switch kind {
	case model.PageConcept:
		if conceptKind == nil || (*conceptKind != model.ConceptNoun && *conceptKind != model.ConceptVerb) || parentID != nil {
			return store.ErrInvalidHierarchy
		}
		if len(content.Steps) != 0 {
			return fmt.Errorf("concept cannot contain BDD steps")
		}
	case model.PageFeature:
		if conceptKind != nil || parentID != nil || len(content.Steps) != 0 {
			return store.ErrInvalidHierarchy
		}
	case model.PageScenario:
		if conceptKind != nil || parentID == nil {
			return store.ErrInvalidHierarchy
		}
		if err := validateBDDSteps(content.Steps); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid page kind")
	}
	return nil
}

func validateBDDSteps(steps []model.Step) error {
	if len(steps) < 3 {
		return fmt.Errorf("scenario requires Given, When and Then steps")
	}
	phase := 0
	hasGiven, hasWhen, hasThen := false, false, false
	for index, step := range steps {
		if strings.TrimSpace(step.Text) == "" {
			return fmt.Errorf("BDD step %d is empty", index+1)
		}
		switch step.Keyword {
		case model.KeywordGiven:
			if phase > 1 {
				return fmt.Errorf("Given step cannot follow When or Then")
			}
			phase = 1
			hasGiven = true
		case model.KeywordWhen:
			if !hasGiven || phase > 2 {
				return fmt.Errorf("When step must follow Given")
			}
			phase = 2
			hasWhen = true
		case model.KeywordThen:
			if !hasWhen {
				return fmt.Errorf("Then step must follow When")
			}
			phase = 3
			hasThen = true
		case model.KeywordAnd, model.KeywordBut:
			if phase == 0 {
				return fmt.Errorf("And or But must follow a primary BDD step")
			}
		default:
			return fmt.Errorf("invalid BDD keyword")
		}
	}
	if !hasGiven || !hasWhen || !hasThen {
		return fmt.Errorf("scenario requires Given, When and Then steps")
	}
	return nil
}

func (r *Repository) CreatePage(ctx context.Context, organizationID, userID string, input model.CreatePageInput, status model.RevisionStatus) (model.PageDetail, error) {
	if !slugPattern.MatchString(input.Slug) {
		return model.PageDetail{}, fmt.Errorf("slug must contain lowercase ASCII words separated by hyphens")
	}
	if status != model.RevisionDraft && status != model.RevisionAccepted {
		return model.PageDetail{}, fmt.Errorf("invalid initial revision status")
	}
	if err := validatePageInput(input.Kind, input.ConceptKind, input.ParentID, input.Content); err != nil {
		return model.PageDetail{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.PageDetail{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := validateParent(ctx, tx, organizationID, input.Kind, input.ParentID); err != nil {
		return model.PageDetail{}, err
	}
	var pageID string
	var conceptKind any
	if input.ConceptKind != nil {
		conceptKind = string(*input.ConceptKind)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO pages(organization_id, kind, concept_kind, parent_id, slug, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text
	`, organizationID, string(input.Kind), conceptKind, input.ParentID, input.Slug, userID).Scan(&pageID); err != nil {
		if strings.Contains(err.Error(), "pages_organization_id_slug_key") {
			return model.PageDetail{}, store.ErrDuplicateSlug
		}
		return model.PageDetail{}, fmt.Errorf("insert page: %w", err)
	}
	revisionID, err := r.insertRevision(ctx, tx, organizationID, pageID, userID, 1, status, nil, input.Content)
	if err != nil {
		return model.PageDetail{}, err
	}
	if status == model.RevisionAccepted {
		_, err = tx.Exec(ctx, `UPDATE pages SET accepted_revision_id = $2, updated_at = now() WHERE id = $1`, pageID, revisionID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE pages SET latest_draft_revision_id = $2, updated_at = now() WHERE id = $1`, pageID, revisionID)
	}
	if err != nil {
		return model.PageDetail{}, fmt.Errorf("attach revision to page: %w", err)
	}
	if err := audit(ctx, tx, organizationID, userID, "page.created", "page", pageID, map[string]any{"kind": input.Kind, "status": status}); err != nil {
		return model.PageDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.PageDetail{}, fmt.Errorf("commit page creation: %w", err)
	}
	return r.PageDetail(ctx, organizationID, pageID)
}

func (r *Repository) SaveRevision(ctx context.Context, organizationID, userID, pageID string, input model.SaveRevisionInput) (model.Revision, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return model.Revision{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var kind string
	var conceptKind *string
	var parentID *string
	var acceptedID, draftID *string
	err = tx.QueryRow(ctx, `
		SELECT kind, concept_kind, parent_id::text, accepted_revision_id::text, latest_draft_revision_id::text
		FROM pages WHERE organization_id = $1 AND id = $2 FOR UPDATE
	`, organizationID, pageID).Scan(&kind, &conceptKind, &parentID, &acceptedID, &draftID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Revision{}, store.ErrNotFound
	}
	if err != nil {
		return model.Revision{}, err
	}
	var currentID *string
	if draftID != nil {
		currentID = draftID
	} else {
		currentID = acceptedID
	}
	if currentID == nil || *currentID != input.BaseRevisionID {
		return model.Revision{}, store.ErrConflict
	}
	var concept *model.ConceptKind
	if conceptKind != nil {
		value := model.ConceptKind(*conceptKind)
		concept = &value
	}
	if err := validatePageInput(model.PageKind(kind), concept, parentID, input.Content); err != nil {
		return model.Revision{}, err
	}
	var nextNumber int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(number), 0) + 1 FROM revisions WHERE page_id = $1`, pageID).Scan(&nextNumber); err != nil {
		return model.Revision{}, err
	}
	if draftID != nil {
		if _, err := tx.Exec(ctx, `UPDATE revisions SET status = 'superseded' WHERE id = $1 AND status = 'draft'`, *draftID); err != nil {
			return model.Revision{}, err
		}
	}
	base := input.BaseRevisionID
	revisionID, err := r.insertRevision(ctx, tx, organizationID, pageID, userID, nextNumber, model.RevisionDraft, &base, input.Content)
	if err != nil {
		return model.Revision{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE pages SET latest_draft_revision_id = $2, updated_at = now() WHERE id = $1`, pageID, revisionID); err != nil {
		return model.Revision{}, err
	}
	if err := audit(ctx, tx, organizationID, userID, "revision.saved", "revision", revisionID, map[string]any{"pageId": pageID, "number": nextNumber}); err != nil {
		return model.Revision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Revision{}, fmt.Errorf("commit revision: %w", err)
	}
	return r.loadRevision(ctx, revisionID)
}

func validateParent(ctx context.Context, tx pgx.Tx, organizationID string, kind model.PageKind, parentID *string) error {
	if kind != model.PageScenario {
		return nil
	}
	var parentKind string
	if err := tx.QueryRow(ctx, `SELECT kind FROM pages WHERE id = $1 AND organization_id = $2`, *parentID, organizationID).Scan(&parentKind); err != nil {
		return store.ErrInvalidHierarchy
	}
	if parentKind != string(model.PageFeature) {
		return store.ErrInvalidHierarchy
	}
	return nil
}

func (r *Repository) insertRevision(ctx context.Context, tx pgx.Tx, organizationID, pageID, userID string, number int, status model.RevisionStatus, baseID *string, content model.RevisionContent) (string, error) {
	var revisionID string
	aliases := content.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	acceptedAt := any(nil)
	if status == model.RevisionAccepted {
		acceptedAt = "now"
	}
	query := `
		INSERT INTO revisions(page_id, number, status, title, body_md, aliases, base_revision_id, created_by, accepted_at, search_document)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, `
	if acceptedAt != nil {
		query += `now(), `
	} else {
		query += `NULL, `
	}
	query += `to_tsvector('simple', $4 || ' ' || $5 || ' ' || array_to_string($6::text[], ' '))) RETURNING id::text`
	if err := tx.QueryRow(ctx, query,
		pageID, number, string(status), strings.TrimSpace(content.Title), content.BodyMD,
		aliases, baseID, userID,
	).Scan(&revisionID); err != nil {
		return "", fmt.Errorf("insert revision: %w", err)
	}
	for position, step := range content.Steps {
		stableID := step.StableID
		if _, err := uuid.Parse(stableID); err != nil {
			stableID = uuid.NewString()
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO bdd_steps(revision_id, stable_id, position, keyword, text)
			VALUES ($1, $2, $3, $4, $5)
		`, revisionID, stableID, position, string(step.Keyword), strings.TrimSpace(step.Text)); err != nil {
			return "", fmt.Errorf("insert BDD step: %w", err)
		}
	}
	for _, reference := range content.References {
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pages WHERE id = $1 AND organization_id = $2)`, reference.TargetPageID, organizationID).Scan(&valid); err != nil || !valid {
			return "", store.ErrInvalidReference
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO page_references(revision_id, target_page_id, relation)
			VALUES ($1, $2, $3)
		`, revisionID, reference.TargetPageID, strings.TrimSpace(reference.Relation)); err != nil {
			return "", fmt.Errorf("insert page reference: %w", err)
		}
	}
	return revisionID, nil
}

func audit(ctx context.Context, tx pgx.Tx, organizationID, userID, action, entityType, entityID string, metadata map[string]any) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(organization_id, actor_id, action, entity_type, entity_id, metadata)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, NULLIF($5, '')::uuid, $6::jsonb)
	`, organizationID, userID, action, entityType, entityID, encoded); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}
