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
		_, _ = connection.Exec(ctx, `UPDATE pages SET accepted_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id IN ($1, $2)`, organizationID, otherOrganizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id IN ($1, $2)`, organizationID, otherOrganizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id IN ($1, $2)`, organizationID, otherOrganizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id IN ($1, $2)`, organizationID, otherOrganizationID)
	}()

	noun := model.PrimitiveNoun
	content := model.RevisionContent{Title: "Zmluva", BodyMD: "Publikovaný obsah", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}}
	detail, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{Kind: model.PagePrimitive, PrimitiveKind: &noun, Slug: "zmluva-integration", Content: content}, model.RevisionAccepted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.PageDetail(ctx, otherOrganizationID, detail.Page.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-organization read error = %v, want not found", err)
	}

	content.BodyMD = "Rozpracovaná náhrada"
	draft, err := repository.SaveRevision(ctx, organizationID, userID, detail.Page.ID, model.SaveRevisionInput{BaseRevisionID: detail.AcceptedRevision.ID, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SaveRevision(ctx, organizationID, userID, detail.Page.ID, model.SaveRevisionInput{BaseRevisionID: detail.AcceptedRevision.ID, Content: content}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale save error = %v, want conflict", err)
	}
	vote, err := repository.SetVote(ctx, organizationID, userID, draft.ID, governance.VoteReject, "Treba spresniť pravidlo")
	if err != nil {
		t.Fatal(err)
	}
	if vote.CommentID == nil {
		t.Fatal("reject vote did not create a blocking thread")
	}
	if _, err := repository.PublishRevision(ctx, organizationID, userID, draft.ID); !errors.Is(err, governance.ErrUnresolvedRejection) {
		t.Fatalf("publish with blocker error = %v, want blocked", err)
	}
	if _, err := repository.ResolveComment(ctx, organizationID, userID, *vote.CommentID); err != nil {
		t.Fatal(err)
	}
	published, err := repository.PublishRevision(ctx, organizationID, userID, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if published.AcceptedRevision == nil || published.AcceptedRevision.ID != draft.ID || published.DraftRevision != nil {
		t.Fatalf("unexpected publication state: %+v", published)
	}
	events, err := repository.ListAudit(ctx, organizationID, 50)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"page.created": false, "revision.saved": false, "vote.recorded": false, "comment.resolved": false, "revision.published": false}
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

func TestListAssistantDraftProposalsIsOwnerScopedAndPendingFirst(t *testing.T) {
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
	conversationID, otherConversationID := uuid.NewString(), uuid.NewString()
	pendingID, publishedID, otherPendingID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Draft list test')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `
		INSERT INTO users(id, organization_id, email, display_name, password_hash)
		VALUES ($1, $3, $4, 'Owner', 'unused'), ($2, $3, $5, 'Other owner', 'unused')
	`, userID, otherUserID, organizationID, userID+"@viki.test", otherUserID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `
		INSERT INTO assistant_conversations(id, organization_id, user_id)
		VALUES ($1, $3, $4), ($2, $3, $5)
	`, conversationID, otherConversationID, organizationID, userID, otherUserID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_draft_proposals WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_conversations WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()
	if _, err := connection.Exec(ctx, `
		INSERT INTO assistant_draft_proposals(id, organization_id, user_id, conversation_id, turn_id, summary, changeset, status, created_at)
		VALUES
			($1, $4, $5, $6, $1, 'Pending', '{"summary":"Pending","operations":[]}', 'awaiting_approval', now() - interval '1 hour'),
			($2, $4, $5, $6, $2, 'Published', '{"summary":"Published","operations":[]}', 'published', now()),
			($3, $4, $7, $8, $3, 'Other pending', '{"summary":"Other pending","operations":[]}', 'awaiting_approval', now())
	`, pendingID, publishedID, otherPendingID, organizationID, userID, conversationID, otherUserID, otherConversationID); err != nil {
		t.Fatal(err)
	}

	proposals, err := repository.ListAssistantDraftProposals(ctx, organizationID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals) != 2 || proposals[0].ID != pendingID || proposals[1].ID != publishedID {
		t.Fatalf("owner proposals = %+v, want pending then published", proposals)
	}
	otherProposals, err := repository.ListAssistantDraftProposals(ctx, organizationID, otherUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherProposals) != 1 || otherProposals[0].ID != otherPendingID {
		t.Fatalf("other owner proposals = %+v, want only own pending proposal", otherProposals)
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
		_, _ = connection.Exec(ctx, `UPDATE pages SET accepted_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	noun := model.PrimitiveNoun
	content := model.RevisionContent{Title: "Zmluva", BodyMD: "Obsah", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}}
	detail, err := repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{
		Kind: model.PagePrimitive, PrimitiveKind: &noun, Slug: "zmluva-changeset", Content: content,
	}, model.RevisionAccepted)
	if err != nil {
		t.Fatal(err)
	}
	changeSet := model.AIChangeSet{Summary: "invalid metadata", Operations: []model.AIChangeOperation{
		{Operation: "create", ClientKey: "new", Kind: model.PageScenario, Slug: "should-rollback", Content: model.RevisionContent{Title: "Rollback", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}}},
		{Operation: "revise", PageID: &detail.Page.ID, BaseRevisionID: &detail.AcceptedRevision.ID, Kind: model.PageScenario, Slug: detail.Page.Slug, Content: content},
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
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_draft_proposals WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_conversations WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `UPDATE pages SET accepted_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	conversation, err := repository.CreateAssistantConversation(ctx, organizationID, userID, model.AssistantEdit)
	if err != nil {
		t.Fatal(err)
	}
	invalidTurnID := uuid.NewString()
	if _, err := repository.StageAssistantDraftProposal(ctx, organizationID, userID, model.AssistantMutationContext{
		ConversationID: conversation.ID, TurnID: invalidTurnID, HermesProfile: "viki-edit", HermesSessionID: "stored-edit-session",
	}, model.AIChangeSet{Summary: "Neplatný scenár", Operations: []model.AIChangeOperation{{
		Operation: "create", ClientKey: "invalid-scenario", Kind: model.PageScenario,
		Slug: "neplatny-scenar", Content: model.RevisionContent{
			Title: "Neplatný scenár", Aliases: []string{},
			Steps:      []model.Step{{Keyword: model.KeywordGiven, Text: "zákazník chce podpísať zmluvu"}},
			References: []model.PageReference{},
		},
	}}}); err == nil {
		t.Fatal("assistant staged a parent scenario containing BDD steps")
	}
	var invalidProposalCount int
	if err := connection.QueryRow(ctx, `SELECT count(*) FROM assistant_draft_proposals WHERE id = $1`, invalidTurnID).Scan(&invalidProposalCount); err != nil {
		t.Fatal(err)
	}
	if invalidProposalCount != 0 {
		t.Fatalf("invalid proposal was persisted: count=%d", invalidProposalCount)
	}
	turnID := uuid.NewString()
	noun := model.PrimitiveNoun
	revisions, err := repository.ApplyAIChangeSet(ctx, organizationID, userID, model.AssistantMutationContext{
		ConversationID:  conversation.ID,
		TurnID:          turnID,
		HermesProfile:   "viki-edit",
		HermesSessionID: "stored-edit-session",
	}, model.AIChangeSet{Summary: "create draft", Operations: []model.AIChangeOperation{{
		Operation: "create", ClientKey: "contract", Kind: model.PagePrimitive, PrimitiveKind: &noun,
		Slug: "assistant-contract", Content: model.RevisionContent{Title: "Zmluva", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
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
	if detail.AcceptedRevision != nil || detail.DraftRevision == nil || detail.DraftRevision.ID != revisions[0].ID {
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

func TestAssistantDraftProposalPublishesAcceptedRevisionsOnlyAfterApproval(t *testing.T) {
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
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Proposal publication')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Approver', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_draft_proposals WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_conversations WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `UPDATE pages SET accepted_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM pages WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	conversation, err := repository.CreateAssistantConversation(ctx, organizationID, userID, model.AssistantEdit)
	if err != nil {
		t.Fatal(err)
	}
	turnID := uuid.NewString()
	noun := model.PrimitiveNoun
	proposal, err := repository.StageAssistantDraftProposal(ctx, organizationID, userID, model.AssistantMutationContext{
		ConversationID: conversation.ID, TurnID: turnID, HermesProfile: "viki-edit", HermesSessionID: "stored-edit-session",
	}, model.AIChangeSet{Summary: "Pridať pojem zákazník", Operations: []model.AIChangeOperation{{
		Operation: "create", ClientKey: "customer", Kind: model.PagePrimitive, PrimitiveKind: &noun,
		Slug: "zakaznik", Content: model.RevisionContent{Title: "Zákazník", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.ID != turnID || proposal.Status != model.AssistantProposalAwaitingApproval {
		t.Fatalf("unexpected staged proposal: %+v", proposal)
	}
	pages, err := repository.ListPages(ctx, organizationID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 0 {
		t.Fatalf("proposal created wiki records before approval: %+v", pages)
	}

	published, err := repository.PublishAssistantDraftProposal(ctx, organizationID, userID, proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != model.AssistantProposalPublished || len(published.PublishedRevisions) != 1 || published.PublishedRevisions[0].Status != model.RevisionAccepted {
		t.Fatalf("unexpected published proposal: %+v", published)
	}
	detail, err := repository.PageDetail(ctx, organizationID, published.PublishedRevisions[0].PageID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.AcceptedRevision == nil || detail.DraftRevision != nil || detail.AcceptedRevision.ID != published.PublishedRevisions[0].ID {
		t.Fatalf("approved proposal did not publish accepted truth: %+v", detail)
	}
}

func TestAssistantDraftProposalRejectionStoresRequiredReasonAndAudit(t *testing.T) {
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
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Proposal rejection')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Reviewer', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = connection.Exec(ctx, `DELETE FROM audit_events WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_draft_proposals WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM assistant_conversations WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM users WHERE organization_id = $1`, organizationID)
		_, _ = connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID)
	}()

	conversation, err := repository.CreateAssistantConversation(ctx, organizationID, userID, model.AssistantEdit)
	if err != nil {
		t.Fatal(err)
	}
	turnID := uuid.NewString()
	noun := model.PrimitiveNoun
	proposal, err := repository.StageAssistantDraftProposal(ctx, organizationID, userID, model.AssistantMutationContext{
		ConversationID: conversation.ID, TurnID: turnID, HermesProfile: "viki-edit", HermesSessionID: "stored-edit-session",
	}, model.AIChangeSet{Summary: "Pridať pojem cena", Operations: []model.AIChangeOperation{{
		Operation: "create", ClientKey: "price", Kind: model.PagePrimitive, PrimitiveKind: &noun,
		Slug: "cena", Content: model.RevisionContent{Title: "Cena", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.DiscardAssistantDraftProposal(ctx, organizationID, userID, proposal.ID, "   "); !errors.Is(err, governance.ErrRejectionReasonRequired) {
		t.Fatalf("blank rejection reason error = %v, want required reason", err)
	}

	reason := "Chýba presný spôsob výpočtu ceny."
	rejected, err := repository.DiscardAssistantDraftProposal(ctx, organizationID, userID, proposal.ID, reason)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != model.AssistantProposalDiscarded || rejected.RejectionReason != reason {
		t.Fatalf("unexpected rejected proposal: %+v", rejected)
	}
	pages, err := repository.ListPages(ctx, organizationID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 0 {
		t.Fatalf("rejection created wiki records: %+v", pages)
	}

	var actorID, recordedReason string
	if err := connection.QueryRow(ctx, `
		SELECT actor_id::text, metadata->>'reason'
		FROM audit_events
		WHERE organization_id = $1 AND action = 'assistant.proposal_discarded' AND entity_id = $2
	`, organizationID, proposal.ID).Scan(&actorID, &recordedReason); err != nil {
		t.Fatal(err)
	}
	if actorID != userID || recordedReason != reason {
		t.Fatalf("unexpected rejection audit: actor=%q reason=%q", actorID, recordedReason)
	}
}
