package postgres

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"viki/internal/governance"
	"viki/internal/model"
	"viki/internal/store"
)

var errInjectedDatabase = errors.New("injected database failure")

type databaseFault struct {
	method     string
	contains   string
	occurrence int
	hits       int
}

func (f *databaseFault) activate(method, contains string, occurrence int) {
	f.method = method
	f.contains = contains
	f.occurrence = occurrence
	f.hits = 0
}

func (f *databaseFault) shouldFail(method, query string) bool {
	if f == nil || f.method != method || (f.contains != "" && !strings.Contains(query, f.contains)) {
		return false
	}
	f.hits++
	return f.occurrence <= 1 || f.hits == f.occurrence
}

type faultPool struct {
	databasePool
	fault *databaseFault
}

func (p *faultPool) Begin(ctx context.Context) (pgx.Tx, error) {
	if p.fault.shouldFail("begin", "") {
		return nil, errInjectedDatabase
	}
	tx, err := p.databasePool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &faultTx{Tx: tx, fault: p.fault}, nil
}

func (p *faultPool) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	if p.fault.shouldFail("begin_tx", "") {
		return nil, errInjectedDatabase
	}
	tx, err := p.databasePool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &faultTx{Tx: tx, fault: p.fault}, nil
}

func (p *faultPool) Exec(ctx context.Context, query string, arguments ...any) (pgconn.CommandTag, error) {
	if p.fault.shouldFail("exec", query) {
		return pgconn.CommandTag{}, errInjectedDatabase
	}
	return p.databasePool.Exec(ctx, query, arguments...)
}

func (p *faultPool) Query(ctx context.Context, query string, arguments ...any) (pgx.Rows, error) {
	if p.fault.shouldFail("query", query) {
		return nil, errInjectedDatabase
	}
	rows, err := p.databasePool.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	if p.fault.shouldFail("rows_scan", query) || p.fault.shouldFail("rows_error", query) {
		return &faultRows{Rows: rows, scanError: p.fault.method == "rows_scan", rowsError: p.fault.method == "rows_error"}, nil
	}
	return rows, nil
}

func (p *faultPool) QueryRow(ctx context.Context, query string, arguments ...any) pgx.Row {
	if p.fault.shouldFail("query_row", query) {
		return faultRow{}
	}
	return p.databasePool.QueryRow(ctx, query, arguments...)
}

func (p *faultPool) Ping(ctx context.Context) error {
	if p.fault.shouldFail("ping", "") {
		return errInjectedDatabase
	}
	return p.databasePool.Ping(ctx)
}

type faultTx struct {
	pgx.Tx
	fault *databaseFault
}

func (tx *faultTx) Commit(ctx context.Context) error {
	if tx.fault.shouldFail("commit", "") {
		_ = tx.Tx.Rollback(ctx)
		return errInjectedDatabase
	}
	return tx.Tx.Commit(ctx)
}

func (tx *faultTx) Exec(ctx context.Context, query string, arguments ...any) (pgconn.CommandTag, error) {
	if tx.fault.shouldFail("tx_exec", query) {
		return pgconn.CommandTag{}, errInjectedDatabase
	}
	return tx.Tx.Exec(ctx, query, arguments...)
}

func (tx *faultTx) Query(ctx context.Context, query string, arguments ...any) (pgx.Rows, error) {
	if tx.fault.shouldFail("tx_query", query) {
		return nil, errInjectedDatabase
	}
	rows, err := tx.Tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	if tx.fault.shouldFail("tx_rows_scan", query) || tx.fault.shouldFail("tx_rows_error", query) {
		return &faultRows{Rows: rows, scanError: tx.fault.method == "tx_rows_scan", rowsError: tx.fault.method == "tx_rows_error"}, nil
	}
	return rows, nil
}

func (tx *faultTx) QueryRow(ctx context.Context, query string, arguments ...any) pgx.Row {
	if tx.fault.shouldFail("tx_query_row", query) {
		return faultRow{}
	}
	return tx.Tx.QueryRow(ctx, query, arguments...)
}

type faultRow struct{}

func (faultRow) Scan(...any) error { return errInjectedDatabase }

type faultRows struct {
	pgx.Rows
	scanError bool
	rowsError bool
}

func (r *faultRows) Scan(destinations ...any) error {
	if r.scanError {
		return errInjectedDatabase
	}
	return r.Rows.Scan(destinations...)
}

func (r *faultRows) Err() error {
	if r.rowsError {
		return errInjectedDatabase
	}
	return r.Rows.Err()
}

func openFaultTestRepository(t *testing.T) (*Repository, *pgxpool.Pool, *databaseFault) {
	t.Helper()
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	if err := (&Repository{pool: pool}).Migrate(context.Background()); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	fault := &databaseFault{}
	return &Repository{pool: &faultPool{databasePool: pool, fault: fault}}, pool, fault
}

func requireInjectedFailure(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("expected injected database failure, got %v", err)
	}
}

type faultFixture struct {
	ctx            context.Context
	repository     *Repository
	pool           *pgxpool.Pool
	fault          *databaseFault
	organizationID string
	userID         string
	conversationID string
	pageID         string
	revisionID     string
}

func newFaultFixture(t *testing.T) *faultFixture {
	t.Helper()
	repository, pool, fault := openFaultTestRepository(t)
	fixture := &faultFixture{
		ctx: context.Background(), repository: repository, pool: pool, fault: fault,
		organizationID: uuid.NewString(), userID: uuid.NewString(),
	}
	if _, err := pool.Exec(fixture.ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Fault fixture')`, fixture.organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(fixture.ctx, `
		INSERT INTO users(id, organization_id, email, display_name, password_hash)
		VALUES ($1, $2, $3, 'Fault User', 'unused')
	`, fixture.userID, fixture.organizationID, fixture.userID+"@fault.viki.test"); err != nil {
		t.Fatal(err)
	}
	conversation, err := repository.CreateAssistantConversation(fixture.ctx, fixture.organizationID, fixture.userID, model.AssistantQA)
	if err != nil {
		t.Fatal(err)
	}
	fixture.conversationID = conversation.ID
	noun := model.ConceptNoun
	page, err := repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageConcept, ConceptKind: &noun, Slug: "fault-" + strings.ToLower(uuid.NewString()),
		Content: model.RevisionContent{Title: "Fault concept", BodyMD: "Fault body", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
	}, model.RevisionAccepted)
	if err != nil {
		t.Fatal(err)
	}
	fixture.pageID = page.Page.ID
	fixture.revisionID = page.AcceptedRevision.ID
	if _, err := pool.Exec(fixture.ctx, `
		INSERT INTO audit_events(organization_id, actor_id, action, entity_type, entity_id, metadata)
		VALUES ($1, $2, 'page.created', 'page', $3, '{}'::jsonb)
	`, fixture.organizationID, fixture.userID, fixture.pageID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(fixture.ctx, `
		INSERT INTO audit_events(organization_id, actor_id, action, entity_type, entity_id, metadata)
		VALUES ($1, $2, 'assistant.drafts_created', 'assistant_conversation', $3,
			jsonb_build_object('turnId', 'fault-turn', 'revisionIds', jsonb_build_array($4::text)))
	`, fixture.organizationID, fixture.userID, fixture.conversationID, fixture.revisionID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestRepositoryReadPathsPropagateRowScanFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	if _, err := fixture.repository.ListAudit(fixture.ctx, fixture.organizationID, 0); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		contains string
		run      func() error
	}{
		{
			name: "audit", contains: "FROM audit_events a",
			run: func() error {
				_, err := fixture.repository.ListAudit(fixture.ctx, fixture.organizationID, 10)
				return err
			},
		},
		{
			name: "assistant conversations", contains: "FROM assistant_conversations",
			run: func() error {
				_, err := fixture.repository.ListAssistantConversations(fixture.ctx, fixture.organizationID, fixture.userID)
				return err
			},
		},
		{
			name: "assistant receipts", contains: "CROSS JOIN LATERAL",
			run: func() error {
				_, err := fixture.repository.AssistantDraftReceipts(fixture.ctx, fixture.organizationID, fixture.conversationID)
				return err
			},
		},
		{
			name: "retrieval", contains: "WITH candidates AS",
			run: func() error {
				_, err := fixture.repository.Retrieve(fixture.ctx, fixture.organizationID, "Fault", false, 5)
				return err
			},
		},
		{
			name: "page listing", contains: "ORDER BY CASE p.kind",
			run: func() error {
				_, err := fixture.repository.ListPages(fixture.ctx, fixture.organizationID, nil)
				return err
			},
		},
		{
			name: "page search", contains: "WITH eligible AS",
			run: func() error {
				_, err := fixture.repository.SearchPages(fixture.ctx, fixture.organizationID, model.SearchOptions{Query: "Fault"})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture.fault.activate("rows_scan", test.contains, 1)
			requireInjectedFailure(t, test.run())
			fixture.fault.activate("", "", 0)
		})
	}
}

func TestRepositoryPingWrapsDatabaseFailures(t *testing.T) {
	repository, _, fault := openFaultTestRepository(t)
	fault.activate("ping", "", 1)
	if err := repository.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "ping database") || !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("unexpected ping error: %v", err)
	}
}

func simpleConceptChangeSet(slug string) model.AIChangeSet {
	noun := model.ConceptNoun
	return model.AIChangeSet{
		Summary: "Create concept",
		Operations: []model.AIChangeOperation{{
			Operation: "create", ClientKey: "concept", Kind: model.PageConcept, ConceptKind: &noun, Slug: slug,
			Content: model.RevisionContent{Title: "Concept", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
		}},
	}
}

func TestRepositoryMutationsPropagateBeginFailures(t *testing.T) {
	tests := []struct {
		name   string
		method string
		run    func(*faultFixture) error
	}{
		{
			name: "apply AI changes", method: "begin_tx",
			run: func(f *faultFixture) error {
				_, err := f.repository.ApplyAIChangeSet(f.ctx, f.organizationID, f.userID, model.AssistantMutationContext{}, simpleConceptChangeSet("apply-begin"))
				return err
			},
		},
		{
			name: "stage assistant proposal", method: "begin",
			run: func(f *faultFixture) error {
				_, err := f.repository.StageAssistantDraftProposal(f.ctx, f.organizationID, f.userID, model.AssistantMutationContext{ConversationID: f.conversationID, TurnID: uuid.NewString()}, simpleConceptChangeSet("stage-begin"))
				return err
			},
		},
		{
			name: "review assistant proposal", method: "begin_tx",
			run: func(f *faultFixture) error {
				_, err := f.repository.ReviewAssistantDraftProposalOperation(f.ctx, f.organizationID, f.userID, uuid.NewString(), "operation", model.AssistantReviewApprove, "", false)
				return err
			},
		},
		{
			name: "publish assistant proposal", method: "begin_tx",
			run: func(f *faultFixture) error {
				_, err := f.repository.PublishAssistantDraftProposal(f.ctx, f.organizationID, f.userID, uuid.NewString())
				return err
			},
		},
		{
			name: "discard assistant proposal", method: "begin",
			run: func(f *faultFixture) error {
				_, err := f.repository.DiscardAssistantDraftProposal(f.ctx, f.organizationID, f.userID, uuid.NewString(), "Reason")
				return err
			},
		},
		{
			name: "create page", method: "begin_tx",
			run: func(f *faultFixture) error {
				noun := model.ConceptNoun
				_, err := f.repository.CreatePage(f.ctx, f.organizationID, f.userID, model.CreatePageInput{
					Kind: model.PageConcept, ConceptKind: &noun, Slug: "create-begin",
					Content: model.RevisionContent{Title: "Concept", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
				}, model.RevisionDraft)
				return err
			},
		},
		{
			name: "save revision", method: "begin_tx",
			run: func(f *faultFixture) error {
				_, err := f.repository.SaveRevision(f.ctx, f.organizationID, f.userID, f.pageID, model.SaveRevisionInput{BaseRevisionID: f.revisionID})
				return err
			},
		},
		{
			name: "publish revision", method: "begin_tx",
			run: func(f *faultFixture) error {
				_, err := f.repository.PublishRevision(f.ctx, f.organizationID, f.userID, f.revisionID)
				return err
			},
		},
		{
			name: "add comment", method: "begin",
			run: func(f *faultFixture) error {
				_, err := f.repository.AddComment(f.ctx, f.organizationID, f.userID, f.pageID, f.revisionID, nil, nil, nil, "Comment", false)
				return err
			},
		},
		{
			name: "resolve comment", method: "begin",
			run: func(f *faultFixture) error {
				_, err := f.repository.ResolveComment(f.ctx, f.organizationID, f.userID, uuid.NewString())
				return err
			},
		},
		{
			name: "vote", method: "begin",
			run: func(f *faultFixture) error {
				_, err := f.repository.SetVote(f.ctx, f.organizationID, f.userID, f.revisionID, governance.VoteApprove, "")
				return err
			},
		},
		{
			name: "initial user", method: "begin",
			run: func(f *faultFixture) error { return f.repository.EnsureInitialUser(f.ctx, "password") },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate(test.method, "", 1)
			requireInjectedFailure(t, test.run(fixture))
		})
	}
}

func stageFaultProposal(t *testing.T, fixture *faultFixture, operations int) model.AssistantDraftProposal {
	t.Helper()
	changeSet := simpleConceptChangeSet("proposal-" + uuid.NewString())
	if operations > 1 {
		noun := model.ConceptNoun
		changeSet.Operations = append(changeSet.Operations, model.AIChangeOperation{
			Operation: "create", ClientKey: "second", Kind: model.PageConcept, ConceptKind: &noun, Slug: "proposal-" + uuid.NewString(),
			Content: model.RevisionContent{Title: "Second", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
		})
	}
	proposal, err := fixture.repository.StageAssistantDraftProposal(
		fixture.ctx,
		fixture.organizationID,
		fixture.userID,
		model.AssistantMutationContext{ConversationID: fixture.conversationID, TurnID: uuid.NewString()},
		changeSet,
	)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func TestRepositoryMutationsPropagateCommitFailures(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, *faultFixture) error
	}{
		{
			name: "apply AI changes",
			run: func(_ *testing.T, f *faultFixture) error {
				_, err := f.repository.ApplyAIChangeSet(f.ctx, f.organizationID, f.userID, model.AssistantMutationContext{}, simpleConceptChangeSet("apply-"+uuid.NewString()))
				return err
			},
		},
		{
			name: "stage assistant proposal",
			run: func(_ *testing.T, f *faultFixture) error {
				_, err := f.repository.StageAssistantDraftProposal(f.ctx, f.organizationID, f.userID, model.AssistantMutationContext{ConversationID: f.conversationID, TurnID: uuid.NewString()}, simpleConceptChangeSet("stage-"+uuid.NewString()))
				return err
			},
		},
		{
			name: "review assistant proposal",
			run: func(t *testing.T, f *faultFixture) error {
				f.fault.activate("", "", 0)
				proposal := stageFaultProposal(t, f, 1)
				f.fault.activate("commit", "", 1)
				_, err := f.repository.ReviewAssistantDraftProposalOperation(f.ctx, f.organizationID, f.userID, proposal.ID, "concept", model.AssistantReviewApprove, "", false)
				return err
			},
		},
		{
			name: "publish assistant proposal",
			run: func(t *testing.T, f *faultFixture) error {
				f.fault.activate("", "", 0)
				proposal := stageFaultProposal(t, f, 1)
				f.fault.activate("commit", "", 1)
				_, err := f.repository.PublishAssistantDraftProposal(f.ctx, f.organizationID, f.userID, proposal.ID)
				return err
			},
		},
		{
			name: "discard assistant proposal",
			run: func(t *testing.T, f *faultFixture) error {
				f.fault.activate("", "", 0)
				proposal := stageFaultProposal(t, f, 1)
				f.fault.activate("commit", "", 1)
				_, err := f.repository.DiscardAssistantDraftProposal(f.ctx, f.organizationID, f.userID, proposal.ID, "Reason")
				return err
			},
		},
		{
			name: "create page",
			run: func(_ *testing.T, f *faultFixture) error {
				noun := model.ConceptNoun
				_, err := f.repository.CreatePage(f.ctx, f.organizationID, f.userID, model.CreatePageInput{
					Kind: model.PageConcept, ConceptKind: &noun, Slug: "create-" + uuid.NewString(),
					Content: model.RevisionContent{Title: "Concept", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
				}, model.RevisionDraft)
				return err
			},
		},
		{
			name: "save revision",
			run: func(_ *testing.T, f *faultFixture) error {
				_, err := f.repository.SaveRevision(f.ctx, f.organizationID, f.userID, f.pageID, model.SaveRevisionInput{
					BaseRevisionID: f.revisionID,
					Content:        model.RevisionContent{Title: "Updated", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
				})
				return err
			},
		},
		{
			name: "publish revision",
			run: func(t *testing.T, f *faultFixture) error {
				f.fault.activate("", "", 0)
				draft, err := f.repository.SaveRevision(f.ctx, f.organizationID, f.userID, f.pageID, model.SaveRevisionInput{
					BaseRevisionID: f.revisionID,
					Content:        model.RevisionContent{Title: "Draft", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}},
				})
				if err != nil {
					t.Fatal(err)
				}
				f.fault.activate("commit", "", 1)
				_, err = f.repository.PublishRevision(f.ctx, f.organizationID, f.userID, draft.ID)
				return err
			},
		},
		{
			name: "add comment",
			run: func(_ *testing.T, f *faultFixture) error {
				_, err := f.repository.AddComment(f.ctx, f.organizationID, f.userID, f.pageID, f.revisionID, nil, nil, nil, "Comment", false)
				return err
			},
		},
		{
			name: "resolve comment",
			run: func(t *testing.T, f *faultFixture) error {
				f.fault.activate("", "", 0)
				comment, err := f.repository.AddComment(f.ctx, f.organizationID, f.userID, f.pageID, f.revisionID, nil, nil, nil, "Comment", false)
				if err != nil {
					t.Fatal(err)
				}
				f.fault.activate("commit", "", 1)
				_, err = f.repository.ResolveComment(f.ctx, f.organizationID, f.userID, comment.ID)
				return err
			},
		},
		{
			name: "vote",
			run: func(_ *testing.T, f *faultFixture) error {
				_, err := f.repository.SetVote(f.ctx, f.organizationID, f.userID, f.revisionID, governance.VoteApprove, "")
				return err
			},
		},
		{
			name: "initial user",
			run:  func(_ *testing.T, f *faultFixture) error { return f.repository.EnsureInitialUser(f.ctx, "password") },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate("commit", "", 1)
			requireInjectedFailure(t, test.run(t, fixture))
		})
	}
}

func conceptContent(title string) model.RevisionContent {
	return model.RevisionContent{Title: title, Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{}}
}

func scenarioSteps() []model.Step {
	return []model.Step{
		{Keyword: model.KeywordGiven, Text: "a precondition"},
		{Keyword: model.KeywordWhen, Text: "an action"},
		{Keyword: model.KeywordThen, Text: "an outcome"},
	}
}

func saveFaultDraft(t *testing.T, fixture *faultFixture) model.Revision {
	t.Helper()
	fixture.fault.activate("", "", 0)
	draft, err := fixture.repository.SaveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, model.SaveRevisionInput{
		BaseRevisionID: fixture.revisionID,
		Content:        conceptContent("Draft concept"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func TestCreatePageValidationAndInternalFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	noun := model.ConceptNoun
	valid := model.CreatePageInput{Kind: model.PageConcept, ConceptKind: &noun, Slug: "new-page", Content: conceptContent("New page")}

	for name, run := range map[string]func() error{
		"invalid slug": func() error {
			input := valid
			input.Slug = "Invalid slug"
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input, model.RevisionDraft)
			return err
		},
		"invalid status": func() error {
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, valid, model.RevisionStatus("invalid"))
			return err
		},
		"invalid content": func() error {
			input := valid
			input.Content.Title = ""
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input, model.RevisionDraft)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	duplicate := valid
	duplicate.Slug = fixture.repository.repositorySlug(t, fixture.pageID)
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, duplicate, model.RevisionDraft); !errors.Is(err, store.ErrDuplicateSlug) {
		t.Fatalf("expected duplicate slug, got %v", err)
	}

	parentID := fixture.pageID
	scenario := model.CreatePageInput{Kind: model.PageScenario, ParentID: &parentID, Slug: "invalid-parent", Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), Aliases: []string{}, References: []model.PageReference{}}}
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, scenario, model.RevisionDraft); !errors.Is(err, store.ErrInvalidHierarchy) {
		t.Fatalf("expected invalid parent hierarchy, got %v", err)
	}

	tests := []struct {
		name, method, contains string
		input                  model.CreatePageInput
	}{
		{name: "page insert", method: "tx_query_row", contains: "INSERT INTO pages", input: valid},
		{name: "revision insert", method: "tx_query_row", contains: "INSERT INTO revisions", input: valid},
		{name: "page pointer", method: "tx_exec", contains: "UPDATE pages SET latest_draft_revision_id", input: valid},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events", input: valid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := test.input
			input.Slug = strings.ReplaceAll(test.name, " ", "-") + "-" + uuid.NewString()
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input, model.RevisionDraft)
			requireInjectedFailure(t, err)
			fixture.fault.activate("", "", 0)
		})
	}
}

func (r *Repository) repositorySlug(t *testing.T, pageID string) string {
	t.Helper()
	var slug string
	if err := r.pool.QueryRow(context.Background(), `SELECT slug FROM pages WHERE id = $1`, pageID).Scan(&slug); err != nil {
		t.Fatal(err)
	}
	return slug
}

func TestCreatePageRevisionChildFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	noun := model.ConceptNoun

	aliasesNil := model.CreatePageInput{Kind: model.PageConcept, ConceptKind: &noun, Slug: "nil-aliases-" + uuid.NewString(), Content: model.RevisionContent{Title: "Nil aliases"}}
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, aliasesNil, model.RevisionDraft); err != nil {
		t.Fatal(err)
	}

	feature, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageFeature, Slug: "feature-" + uuid.NewString(), Content: conceptContent("Feature"),
	}, model.RevisionAccepted)
	if err != nil {
		t.Fatal(err)
	}
	parentID := feature.Page.ID
	scenario := model.CreatePageInput{Kind: model.PageScenario, ParentID: &parentID, Slug: "scenario-" + uuid.NewString(), Content: model.RevisionContent{Title: "Scenario", Aliases: []string{}, Steps: scenarioSteps(), References: []model.PageReference{}}}
	fixture.fault.activate("tx_exec", "INSERT INTO bdd_steps", 1)
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, scenario, model.RevisionDraft); !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("expected BDD insert failure, got %v", err)
	}

	invalidReference := model.CreatePageInput{Kind: model.PageFeature, Slug: "invalid-reference-" + uuid.NewString(), Content: model.RevisionContent{
		Title: "Invalid reference", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{{TargetPageID: uuid.NewString(), Relation: "uses"}},
	}}
	fixture.fault.activate("", "", 0)
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, invalidReference, model.RevisionDraft); !errors.Is(err, store.ErrInvalidReference) {
		t.Fatalf("expected invalid reference, got %v", err)
	}

	validReference := invalidReference
	validReference.Slug = "valid-reference-" + uuid.NewString()
	validReference.Content.References[0].TargetPageID = fixture.pageID
	fixture.fault.activate("tx_exec", "INSERT INTO page_references", 1)
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, validReference, model.RevisionDraft); !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("expected reference insert failure, got %v", err)
	}
}

func TestSaveRevisionInternalFailures(t *testing.T) {
	fixture := newFaultFixture(t)

	if _, err := fixture.repository.SaveRevision(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString(), model.SaveRevisionInput{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing page, got %v", err)
	}
	if _, err := fixture.repository.SaveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, model.SaveRevisionInput{BaseRevisionID: fixture.revisionID}); err == nil {
		t.Fatal("expected invalid content")
	}

	tests := []struct{ name, method, contains string }{
		{name: "page lock", method: "tx_query_row", contains: "FROM pages WHERE organization_id"},
		{name: "revision number", method: "tx_query_row", contains: "SELECT COALESCE(max(number)"},
		{name: "revision insert", method: "tx_query_row", contains: "INSERT INTO revisions"},
		{name: "page pointer", method: "tx_exec", contains: "UPDATE pages SET latest_draft_revision_id"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.SaveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, model.SaveRevisionInput{
				BaseRevisionID: fixture.revisionID, Content: conceptContent("Updated"),
			})
			requireInjectedFailure(t, err)
			fixture.fault.activate("", "", 0)
		})
	}

	draft := saveFaultDraft(t, fixture)
	fixture.fault.activate("tx_exec", "UPDATE revisions SET status = 'superseded'", 1)
	_, err := fixture.repository.SaveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, model.SaveRevisionInput{BaseRevisionID: draft.ID, Content: conceptContent("Replacement")})
	requireInjectedFailure(t, err)
}

func TestAuditRejectsUnencodableMetadataAndWriteFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	tx, err := fixture.pool.Begin(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(fixture.ctx) }()
	if err := audit(fixture.ctx, tx, fixture.organizationID, fixture.userID, "test", "page", fixture.pageID, map[string]any{"invalid": make(chan int)}); err == nil {
		t.Fatal("expected JSON encoding error")
	}

	wrapped := &faultTx{Tx: tx, fault: &databaseFault{method: "tx_exec", contains: "INSERT INTO audit_events"}}
	requireInjectedFailure(t, audit(fixture.ctx, wrapped, fixture.organizationID, fixture.userID, "test", "page", fixture.pageID, nil))
}

func TestPublishRevisionInternalFailures(t *testing.T) {
	t.Run("missing and conflicting revisions", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.PublishRevision(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected missing revision, got %v", err)
		}
		if _, err := fixture.repository.PublishRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("expected revision conflict, got %v", err)
		}
	})

	tests := []struct {
		name, method, contains string
		blocking               bool
	}{
		{name: "page lock", method: "tx_query_row", contains: "FOR UPDATE OF p"},
		{name: "comment query", method: "tx_query", contains: "FROM comments WHERE page_id"},
		{name: "comment scan", method: "tx_rows_scan", contains: "FROM comments WHERE page_id", blocking: true},
		{name: "supersede accepted", method: "tx_exec", contains: "UPDATE revisions SET status = 'superseded'"},
		{name: "accept draft", method: "tx_exec", contains: "UPDATE revisions SET status = 'accepted'"},
		{name: "page pointer", method: "tx_exec", contains: "UPDATE pages SET accepted_revision_id"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			draft := saveFaultDraft(t, fixture)
			if test.blocking {
				if _, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, draft.ID, nil, nil, nil, "Blocker", true); err != nil {
					t.Fatal(err)
				}
			}
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.PublishRevision(fixture.ctx, fixture.organizationID, fixture.userID, draft.ID)
			requireInjectedFailure(t, err)
		})
	}
}

func TestCommentMutationAndProjectionFailures(t *testing.T) {
	tests := []struct{ name, method, contains string }{
		{name: "insert", method: "tx_query_row", contains: "INSERT INTO comments"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, nil, nil, "Comment", false)
			requireInjectedFailure(t, err)
		})
	}

	t.Run("post-commit projection", func(t *testing.T) {
		fixture := newFaultFixture(t)
		fixture.fault.activate("query", "FROM comments c", 1)
		_, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, nil, nil, "Comment", false)
		requireInjectedFailure(t, err)
	})

	t.Run("comment lookup wrappers", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.commentByID(fixture.ctx, fixture.organizationID, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected missing comment, got %v", err)
		}
		fixture.fault.activate("query", "FROM comments c", 1)
		if _, err := fixture.repository.commentByID(fixture.ctx, fixture.organizationID, uuid.NewString()); !errors.Is(err, errInjectedDatabase) {
			t.Fatalf("expected comment lookup failure, got %v", err)
		}
		fixture.fault.activate("query", "FROM comments c", 1)
		if _, err := fixture.repository.listComments(fixture.ctx, fixture.organizationID, fixture.pageID); !errors.Is(err, errInjectedDatabase) {
			t.Fatalf("expected comment list failure, got %v", err)
		}
	})

	t.Run("comment row scan", func(t *testing.T) {
		fixture := newFaultFixture(t)
		comment, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, nil, nil, "Comment", false)
		if err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("rows_scan", "FROM comments c", 1)
		_, err = fixture.repository.commentByID(fixture.ctx, fixture.organizationID, comment.ID)
		requireInjectedFailure(t, err)
	})
}

func TestResolveCommentInternalFailures(t *testing.T) {
	for _, test := range []struct{ name, method, contains string }{
		{name: "update", method: "tx_exec", contains: "UPDATE comments c"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			comment, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, nil, nil, "Comment", true)
			if err != nil {
				t.Fatal(err)
			}
			fixture.fault.activate(test.method, test.contains, 1)
			_, err = fixture.repository.ResolveComment(fixture.ctx, fixture.organizationID, fixture.userID, comment.ID)
			requireInjectedFailure(t, err)
		})
	}
}

func TestVoteInternalFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	if _, err := fixture.repository.SetVote(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, governance.VoteValue("invalid"), ""); err == nil {
		t.Fatal("expected invalid vote")
	}
	if _, err := fixture.repository.SetVote(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString(), governance.VoteApprove, ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing revision, got %v", err)
	}

	tests := []struct {
		name, method, contains string
		value                  governance.VoteValue
		reason                 string
	}{
		{name: "revision lookup", method: "tx_query_row", contains: "SELECT p.id::text FROM revisions", value: governance.VoteApprove},
		{name: "rejection comment", method: "tx_query_row", contains: "INSERT INTO comments", value: governance.VoteReject, reason: "Reason"},
		{name: "vote write", method: "tx_exec", contains: "INSERT INTO votes", value: governance.VoteApprove},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events", value: governance.VoteApprove},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.SetVote(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, test.value, test.reason)
			requireInjectedFailure(t, err)
		})
	}

	t.Run("post-commit projection", func(t *testing.T) {
		fixture := newFaultFixture(t)
		fixture.fault.activate("query_row", "FROM votes v", 1)
		_, err := fixture.repository.SetVote(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, governance.VoteApprove, "")
		requireInjectedFailure(t, err)
	})

	t.Run("vote row scan", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.SetVote(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, governance.VoteApprove, ""); err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("rows_scan", "FROM votes v", 1)
		_, err := fixture.repository.listVotes(fixture.ctx, fixture.organizationID, fixture.pageID)
		requireInjectedFailure(t, err)
	})
}

func createFaultFeature(t *testing.T, fixture *faultFixture) model.PageDetail {
	t.Helper()
	fixture.fault.activate("", "", 0)
	feature, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageFeature, Slug: "feature-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Feature", Aliases: []string{}, Steps: []model.Step{},
			References: []model.PageReference{{TargetPageID: fixture.pageID, Relation: "uses"}},
		},
	}, model.RevisionAccepted)
	if err != nil {
		t.Fatal(err)
	}
	return feature
}

func TestPageDetailPropagatesNestedReadFailures(t *testing.T) {
	tests := []struct {
		name, method, contains string
		occurrence             int
		setup                  func(*testing.T, *faultFixture) string
	}{
		{name: "page", method: "query_row", contains: "WHERE p.organization_id = $1 AND p.id = $2", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "accepted revision", method: "query_row", contains: "FROM revisions r", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "draft revision", method: "query_row", contains: "FROM revisions r", occurrence: 2, setup: func(t *testing.T, f *faultFixture) string { _ = saveFaultDraft(t, f); return f.pageID }},
		{name: "revision summaries", method: "query", contains: "FROM revisions r JOIN users", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "comments", method: "query", contains: "FROM comments c", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "votes", method: "query", contains: "FROM votes v", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "feature children query", method: "query", contains: "p.parent_id = $2", occurrence: 1, setup: func(t *testing.T, f *faultFixture) string { return createFaultFeature(t, f).Page.ID }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			pageID := test.setup(t, fixture)
			fixture.fault.activate(test.method, test.contains, test.occurrence)
			_, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, pageID)
			requireInjectedFailure(t, err)
		})
	}

	t.Run("feature child scan", func(t *testing.T) {
		fixture := newFaultFixture(t)
		feature := createFaultFeature(t, fixture)
		parentID := feature.Page.ID
		if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
			Kind: model.PageScenario, ParentID: &parentID, Slug: "scenario-" + uuid.NewString(),
			Content: model.RevisionContent{Title: "Scenario", Aliases: []string{}, Steps: scenarioSteps(), References: []model.PageReference{}},
		}, model.RevisionAccepted); err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("rows_scan", "p.parent_id = $2", 1)
		_, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, feature.Page.ID)
		requireInjectedFailure(t, err)
	})
}

func TestRevisionProjectionInternalFailures(t *testing.T) {
	t.Run("missing and malformed revision row", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.loadRevision(fixture.ctx, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected missing revision, got %v", err)
		}
		fixture.fault.activate("query_row", "FROM revisions r", 1)
		_, err := fixture.repository.loadRevision(fixture.ctx, fixture.revisionID)
		requireInjectedFailure(t, err)
	})

	tests := []struct{ name, method, contains string }{
		{name: "steps query", method: "query", contains: "FROM bdd_steps"},
		{name: "references query", method: "query", contains: "FROM page_references"},
		{name: "summary query", method: "query", contains: "FROM revisions r JOIN users"},
		{name: "summary scan", method: "rows_scan", contains: "FROM revisions r JOIN users"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate(test.method, test.contains, 1)
			var err error
			if strings.HasPrefix(test.name, "summary") {
				_, err = fixture.repository.listRevisionSummaries(fixture.ctx, fixture.pageID)
			} else {
				_, err = fixture.repository.loadRevision(fixture.ctx, fixture.revisionID)
			}
			requireInjectedFailure(t, err)
		})
	}

	t.Run("step scan", func(t *testing.T) {
		fixture := newFaultFixture(t)
		feature := createFaultFeature(t, fixture)
		parentID := feature.Page.ID
		scenario, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
			Kind: model.PageScenario, ParentID: &parentID, Slug: "scenario-" + uuid.NewString(),
			Content: model.RevisionContent{Title: "Scenario", Aliases: []string{}, Steps: scenarioSteps(), References: []model.PageReference{}},
		}, model.RevisionAccepted)
		if err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("rows_scan", "FROM bdd_steps", 1)
		_, err = fixture.repository.loadRevision(fixture.ctx, scenario.AcceptedRevision.ID)
		requireInjectedFailure(t, err)
	})

	t.Run("reference scan", func(t *testing.T) {
		fixture := newFaultFixture(t)
		feature := createFaultFeature(t, fixture)
		fixture.fault.activate("rows_scan", "FROM page_references", 1)
		_, err := fixture.repository.loadRevision(fixture.ctx, feature.AcceptedRevision.ID)
		requireInjectedFailure(t, err)
	})
}

func TestStageAssistantProposalValidationAndInternalFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	mutation := model.AssistantMutationContext{ConversationID: fixture.conversationID, TurnID: uuid.NewString()}
	if _, err := fixture.repository.StageAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, mutation, model.AIChangeSet{}); err == nil {
		t.Fatal("expected empty proposal validation error")
	}
	if _, err := fixture.repository.StageAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, mutation, model.AIChangeSet{Clarification: "Question", Operations: simpleConceptChangeSet("unused").Operations}); err == nil {
		t.Fatal("expected clarification validation error")
	}
	invalid := simpleConceptChangeSet("invalid-shape")
	invalid.Operations[0].Operation = "unsupported"
	if _, err := fixture.repository.StageAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, mutation, invalid); err == nil {
		t.Fatal("expected change-set validation error")
	}

	originalMarshal := marshalAssistantJSON
	marshalAssistantJSON = func(any) ([]byte, error) { return nil, errInjectedDatabase }
	t.Cleanup(func() { marshalAssistantJSON = originalMarshal })
	if _, err := fixture.repository.StageAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, mutation, simpleConceptChangeSet("marshal-failure")); !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("expected marshal failure, got %v", err)
	}
	marshalAssistantJSON = originalMarshal

	for _, test := range []struct{ name, method, contains string }{
		{name: "proposal insert", method: "tx_exec", contains: "INSERT INTO assistant_draft_proposals"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.StageAssistantDraftProposal(
				fixture.ctx, fixture.organizationID, fixture.userID,
				model.AssistantMutationContext{ConversationID: fixture.conversationID, TurnID: uuid.NewString()},
				simpleConceptChangeSet("stage-"+uuid.NewString()),
			)
			requireInjectedFailure(t, err)
		})
	}
}

func TestAssistantProposalReadFailures(t *testing.T) {
	t.Run("row scan", func(t *testing.T) {
		fixture := newFaultFixture(t)
		_ = stageFaultProposal(t, fixture, 1)
		fixture.fault.activate("rows_scan", "FROM assistant_draft_proposals", 1)
		_, err := fixture.repository.ListAssistantDraftProposals(fixture.ctx, fixture.organizationID, fixture.userID)
		requireInjectedFailure(t, err)
	})

	for _, test := range []struct {
		name, column, value string
		list                bool
	}{
		{name: "list invalid changeset", column: "changeset", value: `'"invalid"'::jsonb`, list: true},
		{name: "list invalid reviews", column: "operation_reviews", value: `'"invalid"'::jsonb`, list: true},
		{name: "detail invalid changeset", column: "changeset", value: `'"invalid"'::jsonb`},
		{name: "detail invalid reviews", column: "operation_reviews", value: `'"invalid"'::jsonb`},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			proposal := stageFaultProposal(t, fixture, 1)
			if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE assistant_draft_proposals SET `+test.column+` = `+test.value+` WHERE id = $1`, proposal.ID); err != nil {
				t.Fatal(err)
			}
			var err error
			if test.list {
				_, err = fixture.repository.ListAssistantDraftProposals(fixture.ctx, fixture.organizationID, fixture.userID)
			} else {
				_, err = fixture.repository.AssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID)
			}
			if err == nil {
				t.Fatal("expected proposal decoding failure")
			}
		})
	}

	for _, list := range []bool{true, false} {
		name := "detail"
		if list {
			name = "list"
		}
		t.Run(name+" missing published revision", func(t *testing.T) {
			fixture := newFaultFixture(t)
			proposal := stageFaultProposal(t, fixture, 1)
			missing := uuid.NewString()
			if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE assistant_draft_proposals SET published_revision_ids = ARRAY[$2::uuid] WHERE id = $1`, proposal.ID, missing); err != nil {
				t.Fatal(err)
			}
			var err error
			if list {
				_, err = fixture.repository.ListAssistantDraftProposals(fixture.ctx, fixture.organizationID, fixture.userID)
			} else {
				_, err = fixture.repository.AssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID)
			}
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("expected missing revision, got %v", err)
			}
		})
	}
}

func TestReviewAssistantProposalValidationAndInternalFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	if _, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString(), "operation", model.AssistantOperationReviewValue("invalid"), "", false); err == nil {
		t.Fatal("expected invalid review")
	}
	if _, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString(), "operation", model.AssistantReviewReject, "", false); !errors.Is(err, governance.ErrRejectionReasonRequired) {
		t.Fatalf("expected rejection reason error, got %v", err)
	}
	if _, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString(), "operation", model.AssistantReviewApprove, "", false); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing proposal, got %v", err)
	}

	t.Run("proposal lock", func(t *testing.T) {
		fixture := newFaultFixture(t)
		proposal := stageFaultProposal(t, fixture, 1)
		fixture.fault.activate("tx_query_row", "FROM assistant_draft_proposals", 1)
		_, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID, "concept", model.AssistantReviewApprove, "", false)
		requireInjectedFailure(t, err)
	})

	for _, column := range []string{"changeset", "operation_reviews"} {
		t.Run("invalid "+column, func(t *testing.T) {
			fixture := newFaultFixture(t)
			proposal := stageFaultProposal(t, fixture, 1)
			if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE assistant_draft_proposals SET `+column+` = '"invalid"'::jsonb WHERE id = $1`, proposal.ID); err != nil {
				t.Fatal(err)
			}
			_, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID, "concept", model.AssistantReviewApprove, "", false)
			if err == nil {
				t.Fatal("expected decode failure")
			}
		})
	}

	t.Run("conflicting status and missing operation", func(t *testing.T) {
		fixture := newFaultFixture(t)
		proposal := stageFaultProposal(t, fixture, 1)
		if _, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID, "missing", model.AssistantReviewApprove, "", false); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected missing operation, got %v", err)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE assistant_draft_proposals SET status = 'discarded', rejection_reason = 'Reason' WHERE id = $1`, proposal.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID, "concept", model.AssistantReviewApprove, "", false); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("expected proposal conflict, got %v", err)
		}
	})

	t.Run("review marshal", func(t *testing.T) {
		fixture := newFaultFixture(t)
		proposal := stageFaultProposal(t, fixture, 1)
		original := marshalAssistantJSON
		marshalAssistantJSON = func(any) ([]byte, error) { return nil, errInjectedDatabase }
		t.Cleanup(func() { marshalAssistantJSON = original })
		_, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID, "concept", model.AssistantReviewApprove, "", false)
		requireInjectedFailure(t, err)
	})

	tests := []struct {
		name, method, contains string
		occurrence             int
		operations             int
		operationKey           string
	}{
		{name: "review audit", method: "tx_exec", contains: "INSERT INTO audit_events", operations: 2, operationKey: "concept"},
		{name: "partial update", method: "tx_exec", contains: "SET operation_reviews = $2, updated_at", operations: 2, operationKey: "concept"},
		{name: "partial commit", method: "commit", operations: 2, operationKey: "concept"},
		{name: "accepted change", method: "tx_query_row", contains: "INSERT INTO pages", operations: 1, operationKey: "concept"},
		{name: "final update", method: "tx_exec", contains: "status = $3", operations: 1, operationKey: "concept"},
		{name: "final audit", method: "tx_exec", contains: "assistant_draft_proposal", occurrence: 2, operations: 1, operationKey: "concept"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			proposal := stageFaultProposal(t, fixture, test.operations)
			occurrence := test.occurrence
			if occurrence == 0 {
				occurrence = 1
			}
			contains := test.contains
			if test.name == "final audit" {
				contains = "INSERT INTO audit_events"
			}
			fixture.fault.activate(test.method, contains, occurrence)
			_, err := fixture.repository.ReviewAssistantDraftProposalOperation(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID, test.operationKey, model.AssistantReviewApprove, "", false)
			requireInjectedFailure(t, err)
		})
	}
}

func TestAssistantProposalReviewHelpersCoverMissingAndInvalidShapes(t *testing.T) {
	if err := auditAssistantOperationReview(context.Background(), nil, "", "", "", "", "", nil, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing review, got %v", err)
	}
	invalid := model.AIChangeSet{Operations: []model.AIChangeOperation{{
		Operation: "create", ClientKey: "feature", Kind: model.PageFeature, Slug: "feature", Content: conceptContent("Feature"),
	}}}
	_, _, _, err := approvedAssistantChangeSet(invalid, []model.AssistantOperationReview{{OperationKey: "feature", Value: model.AssistantReviewApprove}})
	if err == nil {
		t.Fatal("expected approved change-set validation failure")
	}
}

func TestPublishAssistantProposalInternalFailures(t *testing.T) {
	t.Run("missing, published, and discarded states", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.PublishAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected missing proposal, got %v", err)
		}
		published := stageFaultProposal(t, fixture, 1)
		if _, err := fixture.repository.PublishAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, published.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.PublishAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, published.ID); err != nil {
			t.Fatalf("published proposal should be idempotent: %v", err)
		}
		discarded := stageFaultProposal(t, fixture, 1)
		if _, err := fixture.repository.DiscardAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, discarded.ID, "Reason"); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.PublishAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, discarded.ID); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("expected discarded proposal conflict, got %v", err)
		}
	})

	t.Run("proposal lock", func(t *testing.T) {
		fixture := newFaultFixture(t)
		proposal := stageFaultProposal(t, fixture, 1)
		fixture.fault.activate("tx_query_row", "FROM assistant_draft_proposals", 1)
		_, err := fixture.repository.PublishAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID)
		requireInjectedFailure(t, err)
	})

	t.Run("invalid changeset", func(t *testing.T) {
		fixture := newFaultFixture(t)
		proposal := stageFaultProposal(t, fixture, 1)
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE assistant_draft_proposals SET changeset = '"invalid"'::jsonb WHERE id = $1`, proposal.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.PublishAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID); err == nil {
			t.Fatal("expected proposal decode failure")
		}
	})

	for _, test := range []struct{ name, method, contains string }{
		{name: "accepted changes", method: "tx_query_row", contains: "INSERT INTO pages"},
		{name: "proposal update", method: "tx_exec", contains: "SET status = 'published'"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			proposal := stageFaultProposal(t, fixture, 1)
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.PublishAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID)
			requireInjectedFailure(t, err)
		})
	}
}

func TestDiscardAssistantProposalInternalFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	if _, err := fixture.repository.DiscardAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString(), "Reason"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected discard conflict, got %v", err)
	}

	for _, test := range []struct{ name, method, contains string }{
		{name: "proposal update", method: "tx_exec", contains: "SET status = 'discarded'"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			proposal := stageFaultProposal(t, fixture, 1)
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.DiscardAssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID, "Reason")
			requireInjectedFailure(t, err)
		})
	}
}

func runAIChangeSetTx(t *testing.T, fixture *faultFixture, changeSet model.AIChangeSet, status model.RevisionStatus) ([]string, error) {
	t.Helper()
	tx, err := fixture.repository.pool.BeginTx(fixture.ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(fixture.ctx) }()
	return fixture.repository.applyAIChangeSetTx(fixture.ctx, tx, fixture.organizationID, fixture.userID, changeSet, status)
}

func reviseFaultChangeSet(t *testing.T, fixture *faultFixture, baseRevisionID string) model.AIChangeSet {
	t.Helper()
	noun := model.ConceptNoun
	pageID := fixture.pageID
	baseID := baseRevisionID
	return model.AIChangeSet{Operations: []model.AIChangeOperation{{
		Operation: "revise", PageID: &pageID, BaseRevisionID: &baseID, Kind: model.PageConcept, ConceptKind: &noun,
		Slug: fixture.repository.repositorySlug(t, fixture.pageID), Content: conceptContent("Revised concept"),
	}}}
}

func TestApplyAIChangeSetValidationAndAuditFailure(t *testing.T) {
	fixture := newFaultFixture(t)
	if revisions, err := fixture.repository.ApplyAIChangeSet(fixture.ctx, fixture.organizationID, fixture.userID, model.AssistantMutationContext{}, model.AIChangeSet{Clarification: "Question"}); err != nil || len(revisions) != 0 {
		t.Fatalf("clarification should not mutate: revisions=%v err=%v", revisions, err)
	}
	if _, err := fixture.repository.ApplyAIChangeSet(fixture.ctx, fixture.organizationID, fixture.userID, model.AssistantMutationContext{}, model.AIChangeSet{}); err == nil {
		t.Fatal("expected empty change-set error")
	}
	fixture.fault.activate("tx_exec", "INSERT INTO audit_events", 1)
	_, err := fixture.repository.ApplyAIChangeSet(fixture.ctx, fixture.organizationID, fixture.userID, model.AssistantMutationContext{}, simpleConceptChangeSet("audit-"+uuid.NewString()))
	requireInjectedFailure(t, err)
}

func TestApplyAIChangeSetCreateOperationFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	if _, err := runAIChangeSetTx(t, fixture, simpleConceptChangeSet("valid"), model.RevisionStatus("invalid")); err == nil {
		t.Fatal("expected invalid status")
	}

	unknownParent := simpleConceptChangeSet("unknown-parent")
	unknownParent.Operations[0].ParentClientKey = "missing"
	if _, err := runAIChangeSetTx(t, fixture, unknownParent, model.RevisionDraft); err == nil {
		t.Fatal("expected unknown parent key")
	}
	unknownReference := simpleConceptChangeSet("unknown-reference")
	unknownReference.Operations[0].Content.References = []model.PageReference{{TargetClientKey: "missing", Relation: "uses"}}
	if _, err := runAIChangeSetTx(t, fixture, unknownReference, model.RevisionDraft); err == nil {
		t.Fatal("expected unknown reference key")
	}
	invalidSlug := simpleConceptChangeSet("Invalid slug")
	if _, err := runAIChangeSetTx(t, fixture, invalidSlug, model.RevisionDraft); err == nil {
		t.Fatal("expected invalid slug")
	}
	invalidContent := simpleConceptChangeSet("invalid-content")
	invalidContent.Operations[0].Content.Title = ""
	if _, err := runAIChangeSetTx(t, fixture, invalidContent, model.RevisionDraft); err == nil {
		t.Fatal("expected invalid content")
	}

	feature := createFaultFeature(t, fixture)
	parentID := fixture.pageID
	invalidParent := model.AIChangeSet{Operations: []model.AIChangeOperation{{
		Operation: "create", Kind: model.PageScenario, ParentID: &parentID, Slug: "invalid-parent", Content: model.RevisionContent{Title: "Scenario", Aliases: []string{}, Steps: scenarioSteps(), References: []model.PageReference{}},
	}}}
	if _, err := runAIChangeSetTx(t, fixture, invalidParent, model.RevisionDraft); !errors.Is(err, store.ErrInvalidHierarchy) {
		t.Fatalf("expected invalid hierarchy, got %v", err)
	}
	_ = feature

	duplicate := simpleConceptChangeSet(fixture.repository.repositorySlug(t, fixture.pageID))
	if _, err := runAIChangeSetTx(t, fixture, duplicate, model.RevisionDraft); !errors.Is(err, store.ErrDuplicateSlug) {
		t.Fatalf("expected duplicate slug, got %v", err)
	}

	for _, test := range []struct{ name, method, contains string }{
		{name: "page insert", method: "tx_query_row", contains: "INSERT INTO pages"},
		{name: "revision insert", method: "tx_query_row", contains: "INSERT INTO revisions"},
		{name: "page pointer", method: "tx_exec", contains: "UPDATE pages SET latest_draft_revision_id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := runAIChangeSetTx(t, fixture, simpleConceptChangeSet("create-"+uuid.NewString()), model.RevisionDraft)
			requireInjectedFailure(t, err)
		})
	}

	unsupported := simpleConceptChangeSet("unsupported")
	unsupported.Operations[0].Operation = "delete"
	if _, err := runAIChangeSetTx(t, fixture, unsupported, model.RevisionDraft); err == nil {
		t.Fatal("expected unsupported operation")
	}
}

func TestApplyAIChangeSetResolvesCreatedParentsAndReferences(t *testing.T) {
	fixture := newFaultFixture(t)
	noun := model.ConceptNoun
	changeSet := model.AIChangeSet{Operations: []model.AIChangeOperation{
		{
			Operation: "create", ClientKey: "concept", Kind: model.PageConcept, ConceptKind: &noun, Slug: "concept-" + uuid.NewString(), Content: conceptContent("Concept"),
		},
		{
			Operation: "create", ClientKey: "feature", Kind: model.PageFeature, Slug: "feature-" + uuid.NewString(),
			Content: model.RevisionContent{Title: "Feature", Aliases: []string{}, Steps: []model.Step{}, References: []model.PageReference{{TargetClientKey: "concept", Relation: "uses"}}},
		},
		{
			Operation: "create", Kind: model.PageScenario, ParentClientKey: "feature", Slug: "scenario-" + uuid.NewString(),
			Content: model.RevisionContent{Title: "Scenario", Aliases: []string{}, Steps: scenarioSteps(), References: []model.PageReference{{TargetClientKey: "concept", Relation: "uses"}}},
		},
	}}
	ids, err := runAIChangeSetTx(t, fixture, changeSet, model.RevisionDraft)
	if err != nil || len(ids) != 3 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

func TestApplyAIChangeSetReviseOperationFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	if ids, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID), model.RevisionDraft); err != nil || len(ids) != 1 {
		t.Fatalf("successful revise ids=%v err=%v", ids, err)
	}
	missingIDs := model.AIChangeSet{Operations: []model.AIChangeOperation{{Operation: "revise"}}}
	if _, err := runAIChangeSetTx(t, fixture, missingIDs, model.RevisionDraft); err == nil {
		t.Fatal("expected revise identifiers")
	}
	missing := reviseFaultChangeSet(t, fixture, fixture.revisionID)
	missingPageID := uuid.NewString()
	missing.Operations[0].PageID = &missingPageID
	if _, err := runAIChangeSetTx(t, fixture, missing, model.RevisionDraft); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing page, got %v", err)
	}

	fixture.fault.activate("tx_query_row", "FROM pages WHERE id = $1", 1)
	if _, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID), model.RevisionDraft); !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("expected page lookup failure, got %v", err)
	}
	fixture.fault.activate("", "", 0)

	mismatch := reviseFaultChangeSet(t, fixture, fixture.revisionID)
	mismatch.Operations[0].Slug = "wrong-slug"
	if _, err := runAIChangeSetTx(t, fixture, mismatch, model.RevisionDraft); err == nil {
		t.Fatal("expected immutable metadata mismatch")
	}
	conflict := reviseFaultChangeSet(t, fixture, uuid.NewString())
	if _, err := runAIChangeSetTx(t, fixture, conflict, model.RevisionDraft); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	invalid := reviseFaultChangeSet(t, fixture, fixture.revisionID)
	invalid.Operations[0].Content.Title = ""
	if _, err := runAIChangeSetTx(t, fixture, invalid, model.RevisionDraft); err == nil {
		t.Fatal("expected revised content validation error")
	}

	for _, test := range []struct{ name, method, contains string }{
		{name: "revision number", method: "tx_query_row", contains: "SELECT max(number) + 1"},
		{name: "revision insert", method: "tx_query_row", contains: "INSERT INTO revisions"},
		{name: "draft page pointer", method: "tx_exec", contains: "UPDATE pages SET latest_draft_revision_id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID), model.RevisionDraft)
			requireInjectedFailure(t, err)
		})
	}

	t.Run("supersede draft", func(t *testing.T) {
		fixture := newFaultFixture(t)
		draft := saveFaultDraft(t, fixture)
		fixture.fault.activate("tx_exec", "AND status = 'draft'", 1)
		_, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, draft.ID), model.RevisionDraft)
		requireInjectedFailure(t, err)
	})

	t.Run("supersede accepted", func(t *testing.T) {
		fixture := newFaultFixture(t)
		fixture.fault.activate("tx_exec", "AND status = 'accepted'", 1)
		_, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID), model.RevisionAccepted)
		requireInjectedFailure(t, err)
	})

	t.Run("accepted page pointer", func(t *testing.T) {
		fixture := newFaultFixture(t)
		fixture.fault.activate("tx_exec", "UPDATE pages SET accepted_revision_id", 1)
		_, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID), model.RevisionAccepted)
		requireInjectedFailure(t, err)
	})
}

func TestAssistantDraftProposalPropagatesDatabaseErrors(t *testing.T) {
	fixture := newFaultFixture(t)
	proposal := stageFaultProposal(t, fixture, 1)
	fixture.fault.activate("query_row", "FROM assistant_draft_proposals", 1)
	_, err := fixture.repository.AssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, proposal.ID)
	requireInjectedFailure(t, err)
}

func TestAssistantDraftProposalMapsMissingRows(t *testing.T) {
	fixture := newFaultFixture(t)
	_, err := fixture.repository.AssistantDraftProposal(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestScenarioParentLookupFailureIsRejected(t *testing.T) {
	fixture := newFaultFixture(t)
	feature := createFaultFeature(t, fixture)
	parentID := feature.Page.ID
	fixture.fault.activate("tx_query_row", "SELECT kind FROM pages", 1)
	_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: &parentID, Slug: "scenario-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Scenario", Aliases: []string{}, Steps: scenarioSteps(), References: []model.PageReference{}},
	}, model.RevisionDraft)
	if !errors.Is(err, store.ErrInvalidHierarchy) {
		t.Fatalf("expected invalid hierarchy, got %v", err)
	}
}

func TestEnsurePagePublishableFailures(t *testing.T) {
	for _, test := range []struct{ name, method string }{
		{name: "query", method: "tx_query"},
		{name: "scan", method: "tx_rows_scan"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			if _, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, nil, nil, "Blocker", true); err != nil {
				t.Fatal(err)
			}
			fixture.fault.activate(test.method, "FROM comments WHERE page_id", 1)
			tx, err := fixture.repository.pool.Begin(fixture.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(fixture.ctx) }()
			requireInjectedFailure(t, ensurePagePublishable(fixture.ctx, tx, fixture.pageID))
		})
	}

	t.Run("blocker", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, nil, nil, "Blocker", true); err != nil {
			t.Fatal(err)
		}
		_, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID), model.RevisionAccepted)
		if !errors.Is(err, governance.ErrUnresolvedRejection) {
			t.Fatalf("expected publication blocker, got %v", err)
		}
	})
}

type migrationEntry struct {
	name string
	dir  bool
}

func (entry migrationEntry) Name() string               { return entry.name }
func (entry migrationEntry) IsDir() bool                { return entry.dir }
func (entry migrationEntry) Type() fs.FileMode          { return 0 }
func (entry migrationEntry) Info() (fs.FileInfo, error) { return nil, nil }

func withMigrationReaders(t *testing.T, entries []fs.DirEntry, contents []byte, readError error) {
	t.Helper()
	originalDir := readMigrationDir
	originalFile := readMigrationFile
	readMigrationDir = func(fs.FS, string) ([]fs.DirEntry, error) { return entries, nil }
	readMigrationFile = func(fs.FS, string) ([]byte, error) { return contents, readError }
	t.Cleanup(func() {
		readMigrationDir = originalDir
		readMigrationFile = originalFile
	})
}

func TestMigrationDiscoveryAndDatabaseFailures(t *testing.T) {
	t.Run("read directory", func(t *testing.T) {
		fixture := newFaultFixture(t)
		original := readMigrationDir
		readMigrationDir = func(fs.FS, string) ([]fs.DirEntry, error) { return nil, errInjectedDatabase }
		t.Cleanup(func() { readMigrationDir = original })
		if err := fixture.repository.Migrate(fixture.ctx); !errors.Is(err, errInjectedDatabase) {
			t.Fatalf("expected migration directory failure, got %v", err)
		}
	})

	t.Run("directory entry", func(t *testing.T) {
		fixture := newFaultFixture(t)
		withMigrationReaders(t, []fs.DirEntry{migrationEntry{name: "nested", dir: true}}, nil, nil)
		if err := fixture.repository.Migrate(fixture.ctx); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("read file", func(t *testing.T) {
		fixture := newFaultFixture(t)
		withMigrationReaders(t, []fs.DirEntry{migrationEntry{name: "fault-read.sql"}}, nil, errInjectedDatabase)
		if err := fixture.repository.Migrate(fixture.ctx); !errors.Is(err, errInjectedDatabase) {
			t.Fatalf("expected migration file failure, got %v", err)
		}
	})

	for _, test := range []struct{ name, method, contains string }{
		{name: "check", method: "query_row", contains: "SELECT EXISTS"},
		{name: "begin", method: "begin"},
		{name: "apply", method: "tx_exec", contains: "SELECT 1"},
		{name: "record", method: "tx_exec", contains: "INSERT INTO schema_migrations"},
		{name: "commit", method: "commit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			withMigrationReaders(t, []fs.DirEntry{migrationEntry{name: "fault-test.sql"}}, []byte("SELECT 1"), nil)
			fixture.fault.activate(test.method, test.contains, 1)
			if err := fixture.repository.Migrate(fixture.ctx); !errors.Is(err, errInjectedDatabase) {
				t.Fatalf("expected injected migration failure, got %v", err)
			}
		})
	}
}

func TestInitialUserHashAndWriteFailures(t *testing.T) {
	t.Run("hash", func(t *testing.T) {
		fixture := newFaultFixture(t)
		original := hashInitialPassword
		hashInitialPassword = func(string) (string, error) { return "", errInjectedDatabase }
		t.Cleanup(func() { hashInitialPassword = original })
		requireInjectedFailure(t, fixture.repository.EnsureInitialUser(fixture.ctx, "password"))
	})

	for _, test := range []struct{ name, contains string }{
		{name: "organization", contains: "INSERT INTO organizations"},
		{name: "user", contains: "INSERT INTO users"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate("tx_exec", test.contains, 1)
			requireInjectedFailure(t, fixture.repository.EnsureInitialUser(fixture.ctx, "password"))
		})
	}
}

func TestOpenWrapsPoolConstructionFailure(t *testing.T) {
	original := newPoolWithConfig
	newPoolWithConfig = func(context.Context, *pgxpool.Config) (*pgxpool.Pool, error) { return nil, errInjectedDatabase }
	t.Cleanup(func() { newPoolWithConfig = original })
	_, err := Open(context.Background(), "postgres://viki:viki@127.0.0.1:5433/viki_test?sslmode=disable")
	if !errors.Is(err, errInjectedDatabase) || !strings.Contains(err.Error(), "open database") {
		t.Fatalf("unexpected open error: %v", err)
	}
}
