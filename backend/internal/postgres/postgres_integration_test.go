package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"viki/internal/governance"
	"viki/internal/model"
	"viki/internal/postgres"
	"viki/internal/store"
)

func TestScenarioCannotBeApprovedUntilParentFeatureIsApproved(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	organizationID, userID := uuid.NewString(), uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Scenario approval gate')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Reviewer', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `UPDATE pages SET approved_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	feature, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{
		Kind: model.PageFeature,
		Slug: "draft-feature-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Draft feature", Steps: []model.Step{}, References: []model.PageReference{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{
		Kind:     model.PageScenario,
		ParentID: &feature.Page.ID,
		Slug:     "draft-scenario-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Draft scenario", References: []model.PageReference{},
			Steps: []model.Step{
				{Keyword: model.KeywordGiven, Text: "a customer exists"},
				{Keyword: model.KeywordWhen, Text: "the customer acts"},
				{Keyword: model.KeywordThen, Text: "the result is recorded"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repository.ApproveRevision(ctx, organizationID, userID, scenario.DraftRevision.ID); !errors.Is(err, governance.ErrParentFeatureNotApproved) {
		t.Fatalf("approve scenario beneath draft feature error = %v, want parent feature approval error", err)
	}
	if _, err := repository.ApproveRevision(ctx, organizationID, userID, feature.DraftRevision.ID); err != nil {
		t.Fatalf("approve parent feature: %v", err)
	}
	approvedScenario, err := repository.ApproveRevision(ctx, organizationID, userID, scenario.DraftRevision.ID)
	if err != nil {
		t.Fatalf("approve scenario beneath approved feature: %v", err)
	}
	if approvedScenario.ApprovedRevision == nil || approvedScenario.ApprovedRevision.ID != scenario.DraftRevision.ID {
		t.Fatalf("approved scenario = %+v, want scenario revision %s", approvedScenario, scenario.DraftRevision.ID)
	}
}

func TestRevisionGovernanceIsolationAndAudit(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	organizationID, userID := uuid.NewString(), uuid.NewString()
	otherOrganizationID := uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Integration test'), ($2, 'Other organization')`, organizationID, otherOrganizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Tester', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id IN ($1, $2)`, organizationID, otherOrganizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM chats WHERE organization_id IN ($1, $2)`, organizationID, otherOrganizationID)
		_, _ = connection.Exec(ctx, `UPDATE pages SET approved_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id IN ($1, $2)`, organizationID, otherOrganizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id IN ($1, $2)`, organizationID, otherOrganizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id IN ($1, $2)`, organizationID, otherOrganizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id IN ($1, $2)`, organizationID, otherOrganizationID)
	}()

	noun := model.ConceptNoun
	content := model.RevisionContent{Title: "Zmluva", BodyMD: "Schválený obsah", Steps: []model.Step{}, References: []model.PageReference{}}
	detail, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{Kind: model.PageConcept, ConceptKind: &noun, Slug: "zmluva-integration", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = repository.ApproveRevision(ctx, organizationID, userID, detail.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.PageDetail(ctx, otherOrganizationID, detail.Page.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-organization read error = %v, want not found", err)
	}

	content.BodyMD = "Rozpracovaná náhrada"
	draft, err := repository.SaveRevision(ctx, organizationID, userID, detail.Page.ID, model.SaveRevisionInput{BaseRevisionID: detail.ApprovedRevision.ID, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SaveRevision(ctx, organizationID, userID, detail.Page.ID, model.SaveRevisionInput{BaseRevisionID: detail.ApprovedRevision.ID, Content: content}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale save error = %v, want conflict", err)
	}
	objection, err := repository.AddObjection(ctx, organizationID, userID, draft.ID, "Treba spresniť pravidlo")
	if err != nil {
		t.Fatal(err)
	}
	if objection.ID == "" {
		t.Fatal("objection was not created")
	}
	if _, err := repository.ApproveRevision(ctx, organizationID, userID, draft.ID); !errors.Is(err, governance.ErrUnresolvedObjection) {
		t.Fatalf("approval with blocker error = %v, want blocked", err)
	}
	if _, err := repository.ResolveObjection(ctx, organizationID, userID, objection.ID); err != nil {
		t.Fatal(err)
	}
	approved, err := repository.ApproveRevision(ctx, organizationID, userID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if approved.ApprovedRevision == nil || approved.ApprovedRevision.ID != draft.ID || approved.DraftRevision != nil {
		t.Fatalf("unexpected approval state: %+v", approved)
	}
	events, err := repository.ListAudit(ctx, organizationID, 50)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"page.created": false, "revision.saved": false, "objection.created": false, "objection.resolved": false, "revision.approved": false}
	for _, event := range events {
		if _, ok := wanted[event.Action]; ok {
			wanted[event.Action] = true
		}
	}
	for action, found := range wanted {
		if !found {
			t.Errorf("missing audit event %s", action)
		}
	}
}

func TestListAuditExcludesAuthenticationEvents(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	organizationID, userID := uuid.NewString(), uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Audit filtering test')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Tester', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()
	if _, err := connection.Exec(ctx, `
		INSERT INTO audit_events(organization_id, actor_id, action, entity_type, entity_id)
		VALUES ($1, $2, 'auth.login', 'user', $2),
		       ($1, $2, 'auth.logout', 'user', $2),
		       ($1, $2, 'page.created', 'page', $2)
	`, organizationID, userID); err != nil {
		t.Fatal(err)
	}

	events, err := repository.ListAudit(ctx, organizationID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "page.created" {
		t.Fatalf("audit events = %+v, want only page.created", events)
	}
}

func TestAssistantConversationStoresBindingsWithoutTranscriptBodies(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	organizationID, userID, otherUserID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Assistant integration')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `
		INSERT INTO users(id, organization_id, email, display_name, password_hash)
		VALUES ($1, $3, $4, 'Owner', 'unused'),
		       ($2, $3, $5, 'Other', 'unused')
	`, userID, otherUserID, organizationID, userID+"@viki.test", otherUserID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_conversations WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	conversation, err := repository.CreateAssistantConversation(ctx, organizationID, userID, model.AssistantQA)
	if err != nil {
		t.Fatal(err)
	}
	if conversation.PrimaryMode != model.AssistantQA {
		t.Fatalf("unexpected new conversation: %+v", conversation)
	}
	if _, err := repository.AssistantConversation(ctx, organizationID, otherUserID, conversation.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("other user read error = %v, want not found", err)
	}
	if err := repository.SetAssistantSession(ctx, organizationID, userID, conversation.ID, model.AssistantQA, "hermes-stored-qa"); err != nil {
		t.Fatal(err)
	}
	conversation, err = repository.AssistantConversation(ctx, organizationID, userID, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if conversation.QASessionID == nil || *conversation.QASessionID != "hermes-stored-qa" {
		t.Fatalf("binding update was not retained: %+v", conversation)
	}
	resolved, err := repository.AssistantConversationBySession(ctx, model.AssistantQA, "hermes-stored-qa")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != conversation.ID || resolved.UserID != userID || resolved.OrganizationID != organizationID {
		t.Fatalf("unexpected resolved binding: %+v", resolved)
	}
}

func TestAIChangeSetRejectsMismatchedRevisionMetadataAtomically(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	organizationID, userID := uuid.NewString(), uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Changeset integration')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Owner', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `UPDATE pages SET approved_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	noun := model.ConceptNoun
	content := model.RevisionContent{Title: "Zmluva", BodyMD: "Obsah", Steps: []model.Step{}, References: []model.PageReference{}}
	detail, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{
		Kind: model.PageConcept, ConceptKind: &noun, Slug: "zmluva-changeset", Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = repository.ApproveRevision(ctx, organizationID, userID, detail.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	changeSet := model.AIChangeSet{Summary: "invalid metadata", Operations: []model.AIChangeOperation{
		{Operation: "create", ClientKey: "new", Kind: model.PageFeature, Slug: "should-rollback", Content: model.RevisionContent{Title: "Rollback", Steps: []model.Step{}, References: []model.PageReference{}}},
		{Operation: "revise", PageID: &detail.Page.ID, BaseRevisionID: &detail.ApprovedRevision.ID, Kind: model.PageFeature, Slug: detail.Page.Slug, Content: content},
	}}
	_, err = repository.ApplyAIChangeSet(ctx, organizationID, userID, model.AssistantMutationContext{
		ConversationID: uuid.NewString(), TurnID: uuid.NewString(), HermesProfile: "viki-edit", HermesSessionID: "stored-edit",
	}, changeSet)
	if err == nil {
		t.Fatal("mismatched immutable page metadata was accepted")
	}
	pages, err := repository.ListPages(ctx, organizationID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].ID != detail.Page.ID {
		t.Fatalf("change set was not rolled back atomically: %+v", pages)
	}
}

func TestAssistantChangeSetCreatesDraftsAndAttributesTheInitiatingUser(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	organizationID, userID := uuid.NewString(), uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Assistant attribution')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Initiator', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_conversations WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `UPDATE pages SET approved_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	conversation, err := repository.CreateAssistantConversation(ctx, organizationID, userID, model.AssistantEdit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyAIChangeSet(ctx, organizationID, userID, model.AssistantMutationContext{
		ConversationID: conversation.ID, TurnID: uuid.NewString(), HermesProfile: "viki-edit", HermesSessionID: "stored-edit-session",
	}, model.AIChangeSet{Summary: "Neplatný scenár", Operations: []model.AIChangeOperation{{
		Operation: "create", ClientKey: "invalid-feature", Kind: model.PageFeature,
		Slug: "neplatny-scenar", Content: model.RevisionContent{
			Title: "Neplatný scenár", Steps: []model.Step{{Keyword: model.KeywordGiven, Text: "zákazník chce podpísať zmluvu"}},
			References: []model.PageReference{},
		},
	}}}); err == nil {
		t.Fatal("assistant applied a feature containing BDD steps")
	}
	turnID := uuid.NewString()
	noun := model.ConceptNoun
	revisions, err := repository.ApplyAIChangeSet(ctx, organizationID, userID, model.AssistantMutationContext{
		ConversationID:  conversation.ID,
		TurnID:          turnID,
		HermesProfile:   "viki-edit",
		HermesSessionID: "stored-edit-session",
	}, model.AIChangeSet{Summary: "create draft", Operations: []model.AIChangeOperation{{
		Operation: "create", ClientKey: "contract", Kind: model.PageConcept, ConceptKind: &noun,
		Slug: "assistant-contract", Content: model.RevisionContent{Title: "Zmluva", Steps: []model.Step{}, References: []model.PageReference{}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 || revisions[0].Status != model.RevisionDraft {
		t.Fatalf("assistant revisions = %+v, want one draft", revisions)
	}
	detail, err := repository.PageDetail(ctx, organizationID, revisions[0].PageID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ApprovedRevision != nil || detail.DraftRevision == nil || detail.DraftRevision.ID != revisions[0].ID {
		t.Fatalf("assistant change was not draft-only: %+v", detail)
	}

	var action, actorID, entityID, recordedTurn, profile, sessionID string
	if err := connection.QueryRow(ctx, `
		SELECT action, actor_id::text, entity_id::text,
			metadata->>'turnId', metadata->>'hermesProfile', metadata->>'hermesSessionId'
		FROM audit_events
		WHERE organization_id = $1 AND action = 'assistant.drafts_created'
	`, organizationID).Scan(&action, &actorID, &entityID, &recordedTurn, &profile, &sessionID); err != nil {
		t.Fatal(err)
	}
	if action != "assistant.drafts_created" || actorID != userID || entityID != conversation.ID || recordedTurn != turnID || profile != "viki-edit" || sessionID != "stored-edit-session" {
		t.Fatalf("unexpected assistant audit attribution: action=%q actor=%q entity=%q turn=%q profile=%q session=%q", action, actorID, entityID, recordedTurn, profile, sessionID)
	}
}
