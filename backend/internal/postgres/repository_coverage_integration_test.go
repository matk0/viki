package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"viki/internal/model"
	"viki/internal/postgres"
	"viki/internal/store"
)

type repositoryFixture struct {
	ctx            context.Context
	repository     *postgres.Repository
	connection     *pgx.Conn
	organizationID string
	userID         string
}

func newRepositoryFixture(t *testing.T) *repositoryFixture {
	t.Helper()
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Migrate(ctx); err != nil {
		repository.Close()
		t.Fatal(err)
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		repository.Close()
		t.Fatal(err)
	}
	fixture := &repositoryFixture{
		ctx: ctx, repository: repository, connection: connection,
		organizationID: uuid.NewString(), userID: uuid.NewString(),
	}
	if _, err := connection.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Coverage fixture')`, fixture.organizationID); err != nil {
		fixture.close()
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `
		INSERT INTO users(id, organization_id, email, display_name, password_hash)
		VALUES ($1, $2, $3, 'Coverage User', 'unused')
	`, fixture.userID, fixture.organizationID, fixture.userID+"@viki.test"); err != nil {
		fixture.close()
		t.Fatal(err)
	}
	t.Cleanup(fixture.close)
	return fixture
}

func (f *repositoryFixture) close() {
	if f.connection != nil {
		_, _ = f.connection.Exec(f.ctx, `DELETE FROM sessions WHERE user_id = $1`, f.userID)
		_, _ = f.connection.Exec(f.ctx, `DELETE FROM audit_events WHERE organization_id = $1`, f.organizationID)
		_, _ = f.connection.Exec(f.ctx, `DELETE FROM assistant_draft_proposals WHERE organization_id = $1`, f.organizationID)
		_, _ = f.connection.Exec(f.ctx, `DELETE FROM assistant_conversations WHERE organization_id = $1`, f.organizationID)
		_, _ = f.connection.Exec(f.ctx, `UPDATE pages SET approved_revision_id = NULL, latest_draft_revision_id = NULL WHERE organization_id = $1`, f.organizationID)
		_, _ = f.connection.Exec(f.ctx, `DELETE FROM pages WHERE organization_id = $1`, f.organizationID)
		_, _ = f.connection.Exec(f.ctx, `DELETE FROM users WHERE organization_id = $1`, f.organizationID)
		_, _ = f.connection.Exec(f.ctx, `DELETE FROM organizations WHERE id = $1`, f.organizationID)
		_ = f.connection.Close(f.ctx)
		f.connection = nil
	}
	if f.repository != nil {
		f.repository.Close()
		f.repository = nil
	}
}

func (f *repositoryFixture) createApprovedPage(t *testing.T, input model.CreatePageInput) model.PageDetail {
	t.Helper()
	detail, err := f.repository.CreatePage(f.ctx, f.organizationID, f.userID, input)
	if err != nil {
		t.Fatal(err)
	}
	detail, err = f.repository.ApproveRevision(f.ctx, f.organizationID, f.userID, detail.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

func TestAuthenticationRepositoryLifecycleAndInitialUser(t *testing.T) {
	fixture := newRepositoryFixture(t)
	ctx := fixture.ctx
	repository := fixture.repository

	credential, err := repository.CredentialByEmail(ctx, fixture.userID+"@VIKI.TEST")
	if err != nil || credential.ID != fixture.userID || credential.OrganizationID != fixture.organizationID || !credential.Active {
		t.Fatalf("credential=%+v err=%v", credential, err)
	}
	if _, err := repository.CredentialByEmail(ctx, "missing@viki.test"); !errors.Is(err, store.ErrUnauthorized) {
		t.Fatalf("missing credential error=%v", err)
	}
	organizationID, err := repository.OrganizationIDForUser(ctx, fixture.userID)
	if err != nil || organizationID != fixture.organizationID {
		t.Fatalf("organization=%q err=%v", organizationID, err)
	}
	if _, err := repository.OrganizationIDForUser(ctx, uuid.NewString()); !errors.Is(err, store.ErrUnauthorized) {
		t.Fatalf("missing user organization error=%v", err)
	}

	tokenHash := []byte("token-hash")
	csrfHash := []byte("csrf-hash")
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	if err := repository.CreateSession(ctx, fixture.userID, tokenHash, csrfHash, expires); err != nil {
		t.Fatal(err)
	}
	session, err := repository.SessionByHash(ctx, tokenHash)
	if err != nil || session.User.ID != fixture.userID || string(session.CSRFHash) != string(csrfHash) {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	if _, err := repository.SessionByHash(ctx, []byte("missing")); !errors.Is(err, store.ErrUnauthorized) {
		t.Fatalf("missing session error=%v", err)
	}
	if err := repository.DeleteSession(ctx, tokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SessionByHash(ctx, tokenHash); !errors.Is(err, store.ErrUnauthorized) {
		t.Fatalf("deleted session error=%v", err)
	}

	if err := repository.EnsureInitialUser(ctx, "password"); err != nil {
		t.Fatal(err)
	}
	initial, err := repository.CredentialByEmail(ctx, "matej@matejlukasik.com")
	if err != nil || initial.DisplayName != "Matej" || !initial.Active {
		t.Fatalf("initial credential=%+v err=%v", initial, err)
	}
	if err := repository.EnsureInitialUser(ctx, "password-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.connection.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, initial.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.connection.Exec(ctx, `DELETE FROM users WHERE id = $1`, initial.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.connection.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, initial.OrganizationID); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantConversationRepositoryCoversBindingsModesCursorsListingAndReceipts(t *testing.T) {
	fixture := newRepositoryFixture(t)
	ctx := fixture.ctx
	repository := fixture.repository

	qa, err := repository.CreateAssistantConversation(ctx, fixture.organizationID, fixture.userID, model.AssistantMode("invalid"))
	if err != nil || qa.PrimaryMode != model.AssistantQA {
		t.Fatalf("default conversation=%+v err=%v", qa, err)
	}
	edit, err := repository.CreateAssistantConversation(ctx, fixture.organizationID, fixture.userID, model.AssistantEdit)
	if err != nil || edit.PrimaryMode != model.AssistantEdit {
		t.Fatalf("edit conversation=%+v err=%v", edit, err)
	}
	conversations, err := repository.ListAssistantConversations(ctx, fixture.organizationID, fixture.userID)
	if err != nil || len(conversations) != 2 {
		t.Fatalf("conversations=%+v err=%v", conversations, err)
	}
	if missing, err := repository.ListAssistantConversations(ctx, fixture.organizationID, uuid.NewString()); err != nil || len(missing) != 0 {
		t.Fatalf("missing owner conversations=%+v err=%v", missing, err)
	}

	if err := repository.SetAssistantSession(ctx, fixture.organizationID, fixture.userID, qa.ID, model.AssistantQA, "qa-session"); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetAssistantSession(ctx, fixture.organizationID, fixture.userID, qa.ID, model.AssistantEdit, "edit-session"); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetAssistantSession(ctx, fixture.organizationID, fixture.userID, qa.ID, model.AssistantMode("invalid"), "bad"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("invalid session mode error=%v", err)
	}
	if err := repository.SetAssistantPrimaryMode(ctx, fixture.organizationID, fixture.userID, qa.ID, model.AssistantEdit); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateAssistantMode(ctx, fixture.organizationID, fixture.userID, qa.ID, model.AssistantEdit); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateAssistantHandoffCursor(ctx, fixture.organizationID, fixture.userID, qa.ID, model.AssistantQA, 4); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateAssistantHandoffCursor(ctx, fixture.organizationID, fixture.userID, qa.ID, model.AssistantEdit, 7); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateAssistantHandoffCursor(ctx, fixture.organizationID, fixture.userID, qa.ID, model.AssistantMode("invalid"), 8); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("invalid cursor mode error=%v", err)
	}
	if err := repository.SetAssistantPrimaryMode(ctx, fixture.organizationID, uuid.NewString(), qa.ID, model.AssistantQA); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign mode update error=%v", err)
	}

	updated, err := repository.AssistantConversation(ctx, fixture.organizationID, fixture.userID, qa.ID)
	if err != nil || updated.QASessionID == nil || *updated.QASessionID != "qa-session" || updated.EditSessionID == nil || *updated.EditSessionID != "edit-session" || updated.PrimaryMode != model.AssistantEdit || updated.LastMode != model.AssistantEdit || updated.QAHandoffCursor != 4 || updated.EditHandoffCursor != 7 {
		t.Fatalf("updated conversation=%+v err=%v", updated, err)
	}
	for mode, sessionID := range map[model.AssistantMode]string{model.AssistantQA: "qa-session", model.AssistantEdit: "edit-session"} {
		resolved, err := repository.AssistantConversationBySession(ctx, mode, sessionID)
		if err != nil || resolved.ID != qa.ID {
			t.Fatalf("resolved mode=%s conversation=%+v err=%v", mode, resolved, err)
		}
	}
	if _, err := repository.AssistantConversationBySession(ctx, model.AssistantMode("invalid"), "qa-session"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("invalid binding mode error=%v", err)
	}
	if _, err := repository.AssistantConversationBySession(ctx, model.AssistantQA, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing binding error=%v", err)
	}

	noun := model.ConceptNoun
	page, err := repository.CreatePage(ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageConcept, ConceptKind: &noun, Slug: "receipt-concept",
		Content: model.RevisionContent{Title: "Receipt concept", Steps: []model.Step{}, References: []model.PageReference{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.connection.Exec(ctx, `
		INSERT INTO audit_events(organization_id, actor_id, action, entity_type, entity_id, metadata)
		VALUES ($1, $2, 'assistant.drafts_created', 'assistant_conversation', $3, jsonb_build_object('turnId', 'turn-1', 'revisionIds', jsonb_build_array($4::text)))
	`, fixture.organizationID, fixture.userID, qa.ID, page.DraftRevision.ID); err != nil {
		t.Fatal(err)
	}
	receipts, err := repository.AssistantDraftReceipts(ctx, fixture.organizationID, qa.ID)
	if err != nil || len(receipts["turn-1"]) != 1 || receipts["turn-1"][0].RevisionID != page.DraftRevision.ID {
		t.Fatalf("receipts=%+v err=%v", receipts, err)
	}
}

func TestWikiRepositoryCoversSearchRetrievalRevisionsCommentsVotesAndChildren(t *testing.T) {
	fixture := newRepositoryFixture(t)
	ctx := fixture.ctx
	repository := fixture.repository
	noun := model.ConceptNoun

	concept := fixture.createApprovedPage(t, model.CreatePageInput{
		Kind: model.PageConcept, ConceptKind: &noun, Slug: "adresa-pripojenia",
		Content: model.RevisionContent{Title: "Adresa pripojenia", BodyMD: "Miesto dostupnosti internetu", Steps: []model.Step{}, References: []model.PageReference{}},
	})
	feature := fixture.createApprovedPage(t, model.CreatePageInput{
		Kind: model.PageFeature, Slug: "rezervacia-sluzby",
		Content: model.RevisionContent{Title: "Rezervácia služby", BodyMD: "Zákazník rezervuje internet", Steps: []model.Step{}, References: []model.PageReference{{TargetPageID: concept.Page.ID, Relation: "uses"}}},
	})
	scenario := fixture.createApprovedPage(t, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: &feature.Page.ID, Slug: "uspesna-rezervacia",
		Content: model.RevisionContent{Title: "Úspešná rezervácia", BodyMD: "Rezervácia prejde", Steps: []model.Step{
			{Keyword: model.KeywordGiven, Text: "zákazník zadal adresu"},
			{Keyword: model.KeywordWhen, Text: "odošle rezerváciu"},
			{Keyword: model.KeywordThen, Text: "systém ju uloží"},
		}, References: []model.PageReference{{TargetPageID: concept.Page.ID, Relation: "uses"}}},
	})

	pages, err := repository.ListPages(ctx, fixture.organizationID, nil)
	if err != nil || len(pages) != 3 {
		t.Fatalf("pages=%+v err=%v", pages, err)
	}
	kind := model.PageConcept
	concepts, err := repository.ListPages(ctx, fixture.organizationID, &kind)
	if err != nil || len(concepts) != 1 || concepts[0].ConceptKind == nil || *concepts[0].ConceptKind != model.ConceptNoun {
		t.Fatalf("concepts=%+v err=%v", concepts, err)
	}

	detail, err := repository.PageDetail(ctx, fixture.organizationID, feature.Page.ID)
	if err != nil || len(detail.Children) != 1 || detail.Children[0].ID != scenario.Page.ID {
		t.Fatalf("feature detail=%+v err=%v", detail, err)
	}
	if _, err := repository.PageDetail(ctx, uuid.NewString(), feature.Page.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign page error=%v", err)
	}
	revision, err := repository.Revision(ctx, fixture.organizationID, scenario.ApprovedRevision.ID)
	if err != nil || revision.ID != scenario.ApprovedRevision.ID || len(revision.Steps) != 3 || len(revision.References) != 1 {
		t.Fatalf("revision=%+v err=%v", revision, err)
	}
	if _, err := repository.Revision(ctx, uuid.NewString(), scenario.ApprovedRevision.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign revision error=%v", err)
	}

	results, err := repository.SearchPages(ctx, fixture.organizationID, model.SearchOptions{Query: "adresa", Limit: -1})
	if err != nil || len(results) == 0 || results[0].Page.ID != concept.Page.ID {
		t.Fatalf("search results=%+v err=%v", results, err)
	}
	featureKind := model.PageFeature
	results, err = repository.SearchPages(ctx, fixture.organizationID, model.SearchOptions{Query: "", Kind: &featureKind, Limit: 1000})
	if err != nil || len(results) != 1 || results[0].Page.ID != feature.Page.ID {
		t.Fatalf("kind search results=%+v err=%v", results, err)
	}
	documents, err := repository.Retrieve(ctx, fixture.organizationID, "internet", false, 0)
	if err != nil || len(documents) == 0 {
		t.Fatalf("documents=%+v err=%v", documents, err)
	}

	draftContent := concept.ApprovedRevision
	draftContent.BodyMD = "Draft s verejnou IP adresou"
	draft, err := repository.SaveRevision(ctx, fixture.organizationID, fixture.userID, concept.Page.ID, model.SaveRevisionInput{
		BaseRevisionID: concept.ApprovedRevision.ID,
		Content:        model.RevisionContent{Title: draftContent.Title, BodyMD: draftContent.BodyMD, Steps: draftContent.Steps, References: draftContent.References},
	})
	if err != nil {
		t.Fatal(err)
	}
	results, err = repository.SearchPages(ctx, fixture.organizationID, model.SearchOptions{Query: "verejnou", IncludeDrafts: true, Limit: 10})
	if err != nil || len(results) != 1 || !results[0].Draft || results[0].RevisionID != draft.ID {
		t.Fatalf("draft search=%+v err=%v", results, err)
	}
	documents, err = repository.Retrieve(ctx, fixture.organizationID, "verejnou", true, 50)
	if err != nil || len(documents) == 0 || !documents[0].Draft {
		t.Fatalf("draft documents=%+v err=%v", documents, err)
	}
	if _, err := repository.ApproveRevision(ctx, fixture.organizationID, fixture.userID, concept.ApprovedRevision.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("approving approved revision error=%v", err)
	}

	if _, err := repository.AddComment(ctx, fixture.organizationID, fixture.userID, concept.Page.ID, draft.ID, nil, "   "); err == nil {
		t.Fatal("empty comment was accepted")
	}
	if _, err := repository.AddComment(ctx, fixture.organizationID, fixture.userID, concept.Page.ID, uuid.NewString(), nil, "Missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing revision comment error=%v", err)
	}
	root, err := repository.AddComment(ctx, fixture.organizationID, fixture.userID, concept.Page.ID, draft.ID, nil, "Root")
	if err != nil || root.Author.ID != fixture.userID {
		t.Fatalf("root comment=%+v err=%v", root, err)
	}
	wrongParent := uuid.NewString()
	if _, err := repository.AddComment(ctx, fixture.organizationID, fixture.userID, concept.Page.ID, draft.ID, &wrongParent, "Reply"); err == nil {
		t.Fatal("reply to missing parent was accepted")
	}
	reply, err := repository.AddComment(ctx, fixture.organizationID, fixture.userID, concept.Page.ID, draft.ID, &root.ID, "Reply")
	if err != nil || reply.ParentCommentID == nil || *reply.ParentCommentID != root.ID {
		t.Fatalf("reply=%+v err=%v", reply, err)
	}
	if _, err := repository.ResolveObjection(ctx, fixture.organizationID, fixture.userID, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing objection resolve error=%v", err)
	}
	objection, err := repository.AddObjection(ctx, fixture.organizationID, fixture.userID, draft.ID, "Needs detail")
	if err != nil || objection.RevisionID != draft.ID || objection.Author.ID != fixture.userID {
		t.Fatalf("objection=%+v err=%v", objection, err)
	}
	resolved, err := repository.ResolveObjection(ctx, fixture.organizationID, fixture.userID, objection.ID)
	if err != nil || resolved.ResolvedBy == nil || resolved.ResolvedBy.ID != fixture.userID {
		t.Fatalf("resolved objection=%+v err=%v", resolved, err)
	}
	finalDetail, err := repository.PageDetail(ctx, fixture.organizationID, concept.Page.ID)
	if err != nil || len(finalDetail.Comments) != 1 || len(finalDetail.Comments[0].Replies) != 1 {
		t.Fatalf("final detail=%+v err=%v", finalDetail, err)
	}
}

func TestRepositoryMethodsReturnErrorsAfterDatabaseShutdown(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	repository.Close()

	assertError := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded after database shutdown", name)
		}
	}
	assertError("ping", repository.Ping(ctx))
	assertError("migrate", repository.Migrate(ctx))
	_, err = repository.CredentialByEmail(ctx, "user@example.com")
	assertError("credential", err)
	assertError("create session", repository.CreateSession(ctx, uuid.NewString(), []byte("token"), []byte("csrf"), time.Now().Add(time.Hour)))
	_, err = repository.SessionByHash(ctx, []byte("token"))
	assertError("session", err)
	assertError("delete session", repository.DeleteSession(ctx, []byte("token")))
	_, err = repository.OrganizationIDForUser(ctx, uuid.NewString())
	assertError("organization", err)
	assertError("initial user", repository.EnsureInitialUser(ctx, "password"))

	organizationID, userID, pageID, revisionID, conversationID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	noun := model.ConceptNoun
	content := model.RevisionContent{Title: "Title", Steps: []model.Step{}, References: []model.PageReference{}}
	_, err = repository.CreatePage(ctx, organizationID, userID, model.CreatePageInput{Kind: model.PageConcept, ConceptKind: &noun, Slug: "valid-slug", Content: content})
	assertError("create page", err)
	_, err = repository.SaveRevision(ctx, organizationID, userID, pageID, model.SaveRevisionInput{BaseRevisionID: revisionID, Content: content})
	assertError("save revision", err)
	_, err = repository.ListPages(ctx, organizationID, nil)
	assertError("list pages", err)
	_, err = repository.SearchPages(ctx, organizationID, model.SearchOptions{Query: "title"})
	assertError("search pages", err)
	_, err = repository.PageDetail(ctx, organizationID, pageID)
	assertError("page detail", err)
	_, err = repository.Revision(ctx, organizationID, revisionID)
	assertError("revision", err)
	_, err = repository.Retrieve(ctx, organizationID, "title", false, 10)
	assertError("retrieve", err)
	_, err = repository.ApproveRevision(ctx, organizationID, userID, revisionID)
	assertError("approve revision", err)
	_, err = repository.AddComment(ctx, organizationID, userID, pageID, revisionID, nil, "Comment")
	assertError("add comment", err)
	_, err = repository.AddObjection(ctx, organizationID, userID, revisionID, "Reason")
	assertError("add objection", err)
	_, err = repository.ResolveObjection(ctx, organizationID, userID, uuid.NewString())
	assertError("resolve objection", err)
	_, err = repository.ListAudit(ctx, organizationID, 20)
	assertError("list audit", err)

	_, err = repository.ListAssistantConversations(ctx, organizationID, userID)
	assertError("list conversations", err)
	_, err = repository.CreateAssistantConversation(ctx, organizationID, userID, model.AssistantQA)
	assertError("create conversation", err)
	_, err = repository.AssistantConversation(ctx, organizationID, userID, conversationID)
	assertError("conversation", err)
	_, err = repository.AssistantConversationBySession(ctx, model.AssistantQA, "session")
	assertError("conversation by session", err)
	assertError("set assistant session", repository.SetAssistantSession(ctx, organizationID, userID, conversationID, model.AssistantQA, "session"))
	assertError("set primary mode", repository.SetAssistantPrimaryMode(ctx, organizationID, userID, conversationID, model.AssistantEdit))
	assertError("update mode", repository.UpdateAssistantMode(ctx, organizationID, userID, conversationID, model.AssistantEdit))
	assertError("update cursor", repository.UpdateAssistantHandoffCursor(ctx, organizationID, userID, conversationID, model.AssistantQA, 1))
	_, err = repository.AssistantDraftReceipts(ctx, organizationID, conversationID)
	assertError("assistant receipts", err)

	mutation := model.AssistantMutationContext{ConversationID: conversationID, TurnID: uuid.NewString()}
	changeSet := model.AIChangeSet{Summary: "Create", Operations: []model.AIChangeOperation{{Operation: "create", ClientKey: "concept", Kind: model.PageConcept, ConceptKind: &noun, Slug: "concept", Content: content}}}
	_, err = repository.ApplyAIChangeSet(ctx, organizationID, userID, mutation, changeSet)
	assertError("apply change set", err)
}

func TestOpenRejectsInvalidAndUnreachableDatabaseURLs(t *testing.T) {
	ctx := context.Background()
	if _, err := postgres.Open(ctx, "%"); err == nil {
		t.Fatal("invalid database URL was accepted")
	}
	if _, err := postgres.Open(ctx, "postgres://viki:viki@127.0.0.1:1/viki?sslmode=disable&connect_timeout=1"); err == nil {
		t.Fatal("unreachable database URL was accepted")
	}
}
