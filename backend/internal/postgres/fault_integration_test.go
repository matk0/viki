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
		Content: model.RevisionContent{Title: "Fault concept", BodyMD: "Fault body", Steps: []model.Step{}, References: []model.PageReference{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err = repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, page.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.pageID = page.Page.ID
	fixture.revisionID = page.ApprovedRevision.ID
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
			Content: model.RevisionContent{Title: "Concept", Steps: []model.Step{}, References: []model.PageReference{}},
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
			name: "create page", method: "begin_tx",
			run: func(f *faultFixture) error {
				noun := model.ConceptNoun
				_, err := f.repository.CreatePage(f.ctx, f.organizationID, f.userID, model.CreatePageInput{
					Kind: model.PageConcept, ConceptKind: &noun, Slug: "create-begin",
					Content: model.RevisionContent{Title: "Concept", Steps: []model.Step{}, References: []model.PageReference{}},
				})
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
			name: "approve revision", method: "begin_tx",
			run: func(f *faultFixture) error {
				_, err := f.repository.ApproveRevision(f.ctx, f.organizationID, f.userID, f.revisionID)
				return err
			},
		},
		{
			name: "add comment", method: "begin",
			run: func(f *faultFixture) error {
				_, err := f.repository.AddComment(f.ctx, f.organizationID, f.userID, f.pageID, f.revisionID, nil, "Comment")
				return err
			},
		},
		{
			name: "add objection", method: "begin",
			run: func(f *faultFixture) error {
				_, err := f.repository.AddObjection(f.ctx, f.organizationID, f.userID, f.revisionID, "Reason")
				return err
			},
		},
		{
			name: "resolve objection", method: "begin",
			run: func(f *faultFixture) error {
				_, err := f.repository.ResolveObjection(f.ctx, f.organizationID, f.userID, uuid.NewString())
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
			name: "create page",
			run: func(_ *testing.T, f *faultFixture) error {
				noun := model.ConceptNoun
				_, err := f.repository.CreatePage(f.ctx, f.organizationID, f.userID, model.CreatePageInput{
					Kind: model.PageConcept, ConceptKind: &noun, Slug: "create-" + uuid.NewString(),
					Content: model.RevisionContent{Title: "Concept", Steps: []model.Step{}, References: []model.PageReference{}},
				})
				return err
			},
		},
		{
			name: "save revision",
			run: func(_ *testing.T, f *faultFixture) error {
				_, err := f.repository.SaveRevision(f.ctx, f.organizationID, f.userID, f.pageID, model.SaveRevisionInput{
					BaseRevisionID: f.revisionID,
					Content:        model.RevisionContent{Title: "Updated", Steps: []model.Step{}, References: []model.PageReference{}},
				})
				return err
			},
		},
		{
			name: "approve revision",
			run: func(t *testing.T, f *faultFixture) error {
				f.fault.activate("", "", 0)
				draft, err := f.repository.SaveRevision(f.ctx, f.organizationID, f.userID, f.pageID, model.SaveRevisionInput{
					BaseRevisionID: f.revisionID,
					Content:        model.RevisionContent{Title: "Draft", Steps: []model.Step{}, References: []model.PageReference{}},
				})
				if err != nil {
					t.Fatal(err)
				}
				f.fault.activate("commit", "", 1)
				_, err = f.repository.ApproveRevision(f.ctx, f.organizationID, f.userID, draft.ID)
				return err
			},
		},
		{
			name: "add comment",
			run: func(_ *testing.T, f *faultFixture) error {
				_, err := f.repository.AddComment(f.ctx, f.organizationID, f.userID, f.pageID, f.revisionID, nil, "Comment")
				return err
			},
		},
		{
			name: "add objection",
			run: func(_ *testing.T, f *faultFixture) error {
				_, err := f.repository.AddObjection(f.ctx, f.organizationID, f.userID, f.revisionID, "Reason")
				return err
			},
		},
		{
			name: "resolve objection",
			run: func(t *testing.T, f *faultFixture) error {
				f.fault.activate("", "", 0)
				objection, err := f.repository.AddObjection(f.ctx, f.organizationID, f.userID, f.revisionID, "Reason")
				if err != nil {
					t.Fatal(err)
				}
				f.fault.activate("commit", "", 1)
				_, err = f.repository.ResolveObjection(f.ctx, f.organizationID, f.userID, objection.ID)
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
	return model.RevisionContent{Title: title, Steps: []model.Step{}, References: []model.PageReference{}}
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
	validFeature := func() model.CreatePageInput {
		return model.CreatePageInput{
			Kind: model.PageFeature, Slug: "feature-" + uuid.NewString(), Content: conceptContent("Feature"),
			InitialScenario: &model.InitialScenarioInput{
				Slug:    "scenario-" + uuid.NewString(),
				Content: model.RevisionContent{Title: "Initial scenario", Steps: scenarioSteps()},
			},
		}
	}

	for name, run := range map[string]func() error{
		"invalid slug": func() error {
			input := valid
			input.Slug = "Invalid slug"
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input)
			return err
		},
		"invalid content": func() error {
			input := valid
			input.Content.Title = ""
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input)
			return err
		},
		"initial scenario on concept": func() error {
			input := valid
			input.InitialScenario = validFeature().InitialScenario
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input)
			return err
		},
		"invalid initial scenario slug": func() error {
			input := validFeature()
			input.InitialScenario.Slug = "Invalid scenario"
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input)
			return err
		},
		"invalid initial scenario content": func() error {
			input := validFeature()
			input.InitialScenario.Content.Steps = nil
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input)
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
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, duplicate); !errors.Is(err, store.ErrDuplicateSlug) {
		t.Fatalf("expected duplicate slug, got %v", err)
	}
	duplicateInitialScenario := validFeature()
	duplicateInitialScenario.InitialScenario.Slug = fixture.repository.repositorySlug(t, fixture.pageID)
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, duplicateInitialScenario); !errors.Is(err, store.ErrDuplicateSlug) {
		t.Fatalf("expected duplicate initial scenario slug, got %v", err)
	}

	parentID := fixture.pageID
	scenario := model.CreatePageInput{Kind: model.PageScenario, ParentID: &parentID, Slug: "invalid-parent", Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{}}}
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, scenario); !errors.Is(err, store.ErrInvalidHierarchy) {
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
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, input)
			requireInjectedFailure(t, err)
			fixture.fault.activate("", "", 0)
		})
	}

	for _, test := range []struct {
		name, method, contains string
		occurrence             int
	}{
		{name: "initial scenario page insert", method: "tx_query_row", contains: "INSERT INTO pages", occurrence: 2},
		{name: "initial scenario revision insert", method: "tx_query_row", contains: "INSERT INTO revisions", occurrence: 2},
		{name: "initial scenario page pointer", method: "tx_exec", contains: "UPDATE pages SET latest_draft_revision_id", occurrence: 2},
		{name: "initial scenario audit", method: "tx_exec", contains: "INSERT INTO audit_events", occurrence: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture.fault.activate(test.method, test.contains, test.occurrence)
			_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, validFeature())
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

	minimalInput := model.CreatePageInput{Kind: model.PageConcept, ConceptKind: &noun, Slug: "minimal-content-" + uuid.NewString(), Content: model.RevisionContent{Title: "Minimal content"}}
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, minimalInput); err != nil {
		t.Fatal(err)
	}

	feature, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageFeature, Slug: "feature-" + uuid.NewString(), Content: conceptContent("Feature"),
	})
	if err != nil {
		t.Fatal(err)
	}
	parentID := feature.Page.ID
	scenario := model.CreatePageInput{Kind: model.PageScenario, ParentID: &parentID, Slug: "scenario-" + uuid.NewString(), Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{}}}
	fixture.fault.activate("tx_exec", "INSERT INTO bdd_steps", 1)
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, scenario); !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("expected BDD insert failure, got %v", err)
	}

	invalidReference := model.CreatePageInput{Kind: model.PageFeature, Slug: "invalid-reference-" + uuid.NewString(), Content: model.RevisionContent{
		Title: "Invalid reference", Steps: []model.Step{}, References: []model.PageReference{{TargetPageID: uuid.NewString(), Relation: "uses"}},
	}}
	fixture.fault.activate("", "", 0)
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, invalidReference); !errors.Is(err, store.ErrInvalidReference) {
		t.Fatalf("expected invalid reference, got %v", err)
	}

	validReference := invalidReference
	validReference.Slug = "valid-reference-" + uuid.NewString()
	validReference.Content.References[0].TargetPageID = fixture.pageID
	fixture.fault.activate("tx_exec", "INSERT INTO page_references", 1)
	if _, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, validReference); !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("expected reference insert failure, got %v", err)
	}
}

func TestReusableStepDefinitionFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	var definitionID string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		INSERT INTO step_definitions(organization_id, expression, role, approved_at)
		VALUES ($1, 'an approved action', 'action', now())
		RETURNING id::text
	`, fixture.organizationID).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct{ name, method string }{
		{name: "query", method: "query"},
		{name: "scan", method: "rows_scan"},
		{name: "rows", method: "rows_error"},
	} {
		t.Run("catalog "+test.name, func(t *testing.T) {
			fixture.fault.activate(test.method, "FROM step_definitions", 1)
			_, err := fixture.repository.ListStepDefinitions(fixture.ctx, fixture.organizationID, "approved", nil)
			requireInjectedFailure(t, err)
			fixture.fault.activate("", "", 0)
		})
	}

	insert := func(t *testing.T, steps []model.Step, activate func()) error {
		t.Helper()
		tx, err := fixture.pool.BeginTx(fixture.ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(fixture.ctx) }()
		wrapped := &faultTx{Tx: tx, fault: fixture.fault}
		if activate != nil {
			activate()
		}
		_, err = fixture.repository.insertRevision(fixture.ctx, wrapped, fixture.organizationID, fixture.pageID, fixture.userID, 999, nil, model.RevisionContent{Title: "Step failure", Steps: steps})
		fixture.fault.activate("", "", 0)
		return err
	}

	if err := insert(t, []model.Step{{Keyword: model.KeywordGiven, DefinitionID: uuid.NewString()}}, nil); !errors.Is(err, store.ErrInvalidReference) {
		t.Fatalf("missing definition error = %v", err)
	}
	if err := insert(t, []model.Step{{Keyword: model.KeywordGiven, DefinitionID: definitionID}}, nil); err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("mismatched definition error = %v", err)
	}
	err := insert(t, []model.Step{{Keyword: model.KeywordGiven, Text: "new context"}}, func() {
		fixture.fault.activate("tx_query_row", "INSERT INTO step_definitions", 1)
	})
	requireInjectedFailure(t, err)
	if err := insert(t, []model.Step{{Keyword: model.KeywordGiven, Expression: "there are {int}", Arguments: []string{"two"}}}, nil); err == nil || !strings.Contains(err.Error(), "must be an integer") {
		t.Fatalf("invalid parameter error = %v", err)
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

func TestApproveRevisionInternalFailures(t *testing.T) {
	t.Run("missing and conflicting revisions", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expected missing revision, got %v", err)
		}
		if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("expected revision conflict, got %v", err)
		}
	})

	tests := []struct {
		name, method, contains string
		objection              bool
	}{
		{name: "page lock", method: "tx_query_row", contains: "FOR UPDATE OF p"},
		{name: "objection query", method: "tx_query", contains: "FROM objections WHERE page_id"},
		{name: "objection scan", method: "tx_rows_scan", contains: "FROM objections WHERE page_id", objection: true},
		{name: "supersede approved", method: "tx_exec", contains: "UPDATE revisions SET status = 'superseded'"},
		{name: "approve draft", method: "tx_exec", contains: "UPDATE revisions SET status = 'approved'"},
		{name: "page pointer", method: "tx_exec", contains: "UPDATE pages SET approved_revision_id"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			draft := saveFaultDraft(t, fixture)
			if test.objection {
				if _, err := fixture.repository.AddObjection(fixture.ctx, fixture.organizationID, fixture.userID, draft.ID, "Blocker"); err != nil {
					t.Fatal(err)
				}
			}
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, draft.ID)
			requireInjectedFailure(t, err)
		})
	}

	t.Run("publish scenario step definitions", func(t *testing.T) {
		fixture := newFaultFixture(t)
		feature, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
			Kind: model.PageFeature, Slug: "step-definition-feature-" + uuid.NewString(), Content: conceptContent("Feature"),
			InitialScenario: &model.InitialScenarioInput{Slug: "step-definition-scenario-" + uuid.NewString(), Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, feature.DraftRevision.ID); err != nil {
			t.Fatal(err)
		}
		var scenarioRevisionID string
		if err := fixture.pool.QueryRow(fixture.ctx, `SELECT latest_draft_revision_id::text FROM pages WHERE parent_id = $1`, feature.Page.ID).Scan(&scenarioRevisionID); err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("tx_exec", "UPDATE step_definitions", 1)
		_, err = fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, scenarioRevisionID)
		requireInjectedFailure(t, err)
	})
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
			_, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, "Comment")
			requireInjectedFailure(t, err)
		})
	}

	t.Run("post-commit projection", func(t *testing.T) {
		fixture := newFaultFixture(t)
		fixture.fault.activate("query", "FROM comments c", 1)
		_, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, "Comment")
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
		comment, err := fixture.repository.AddComment(fixture.ctx, fixture.organizationID, fixture.userID, fixture.pageID, fixture.revisionID, nil, "Comment")
		if err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("rows_scan", "FROM comments c", 1)
		_, err = fixture.repository.commentByID(fixture.ctx, fixture.organizationID, comment.ID)
		requireInjectedFailure(t, err)
	})
}

func TestObjectionMutationAndResolutionFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	if _, err := fixture.repository.objectionByID(fixture.ctx, fixture.organizationID, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing objection projection, got %v", err)
	}
	if _, err := fixture.repository.AddObjection(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, " "); !errors.Is(err, governance.ErrObjectionReasonRequired) {
		t.Fatalf("expected objection reason error, got %v", err)
	}
	if _, err := fixture.repository.AddObjection(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString(), "Reason"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing revision, got %v", err)
	}
	if _, err := fixture.repository.ResolveObjection(fixture.ctx, fixture.organizationID, fixture.userID, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing objection, got %v", err)
	}

	for _, test := range []struct{ name, method, contains string }{
		{name: "revision lookup", method: "tx_query_row", contains: "SELECT p.id::text"},
		{name: "insert", method: "tx_query_row", contains: "INSERT INTO objections"},
		{name: "create audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	} {
		t.Run("create "+test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.AddObjection(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, "Reason")
			requireInjectedFailure(t, err)
		})
	}

	t.Run("create post-commit projection", func(t *testing.T) {
		fixture := newFaultFixture(t)
		fixture.fault.activate("query_row", "FROM objections o", 1)
		_, err := fixture.repository.AddObjection(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, "Reason")
		requireInjectedFailure(t, err)
	})

	for _, test := range []struct{ name, method, contains string }{
		{name: "update", method: "tx_exec", contains: "UPDATE objections o"},
		{name: "audit", method: "tx_exec", contains: "INSERT INTO audit_events"},
	} {
		t.Run("resolve "+test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			objection, err := fixture.repository.AddObjection(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, "Reason")
			if err != nil {
				t.Fatal(err)
			}
			fixture.fault.activate(test.method, test.contains, 1)
			_, err = fixture.repository.ResolveObjection(fixture.ctx, fixture.organizationID, fixture.userID, objection.ID)
			requireInjectedFailure(t, err)
		})
	}

	t.Run("resolve post-commit projection", func(t *testing.T) {
		fixture := newFaultFixture(t)
		objection, err := fixture.repository.AddObjection(fixture.ctx, fixture.organizationID, fixture.userID, fixture.revisionID, "Reason")
		if err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("query_row", "FROM objections o", 1)
		_, err = fixture.repository.ResolveObjection(fixture.ctx, fixture.organizationID, fixture.userID, objection.ID)
		requireInjectedFailure(t, err)
	})
}

func createFaultFeature(t *testing.T, fixture *faultFixture) model.PageDetail {
	t.Helper()
	fixture.fault.activate("", "", 0)
	feature, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageFeature, Slug: "feature-" + uuid.NewString(),
		Content: model.RevisionContent{
			Title: "Feature", Steps: []model.Step{},
			References: []model.PageReference{{TargetPageID: fixture.pageID, Relation: "uses"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	feature, err = fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, feature.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	return feature
}

func createFaultScenario(t *testing.T, fixture *faultFixture) string {
	t.Helper()
	feature := createFaultFeature(t, fixture)
	parentID := feature.Page.ID
	scenario, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: &parentID, Slug: "scenario-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return scenario.Page.ID
}

func queueFaultScenario(t *testing.T, fixture *faultFixture) model.PageDetail {
	t.Helper()
	pageID := createFaultScenario(t, fixture)
	detail, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, pageID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err = fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, detail.DraftRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

func TestScenarioDevelopmentDatabaseFailures(t *testing.T) {
	t.Run("queue status", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.HasQueuedScenarioDevelopment(fixture.ctx); err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("query_row", "SELECT EXISTS(SELECT 1 FROM scenario_developments", 1)
		_, err := fixture.repository.HasQueuedScenarioDevelopment(fixture.ctx)
		requireInjectedFailure(t, err)
	})

	for _, test := range []struct{ name, method, contains string }{
		{name: "begin", method: "begin"},
		{name: "select", method: "tx_query_row", contains: "FROM scenario_developments sd"},
		{name: "mark running", method: "tx_query_row", contains: "UPDATE scenario_developments"},
		{name: "commit", method: "commit"},
		{name: "load scenario", method: "query_row", contains: "FROM revisions r"},
	} {
		t.Run("claim "+test.name, func(t *testing.T) {
			fixture := newFaultFixture(t)
			if test.name != "begin" {
				queueFaultScenario(t, fixture)
			}
			fixture.fault.activate(test.method, test.contains, 1)
			_, err := fixture.repository.ClaimScenarioDevelopment(fixture.ctx)
			requireInjectedFailure(t, err)
		})
	}

	t.Run("block and missing running task", func(t *testing.T) {
		fixture := newFaultFixture(t)
		queueFaultScenario(t, fixture)
		task, err := fixture.repository.ClaimScenarioDevelopment(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		development, err := fixture.repository.BlockScenarioDevelopment(fixture.ctx, task.RevisionID, "Target API missing")
		if err != nil || development.Status != model.DevelopmentBlocked || development.Detail != "Target API missing" {
			t.Fatalf("development=%+v err=%v", development, err)
		}
		if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE scenario_developments SET status = 'blocked' WHERE status = 'running'`); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.repository.BlockScenarioDevelopment(fixture.ctx, task.RevisionID, "again"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("missing running task error=%v, want not found", err)
		}
	})

	t.Run("finish query", func(t *testing.T) {
		fixture := newFaultFixture(t)
		queueFaultScenario(t, fixture)
		task, err := fixture.repository.ClaimScenarioDevelopment(fixture.ctx)
		if err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("query_row", "UPDATE scenario_developments", 1)
		_, err = fixture.repository.CompleteScenarioDevelopment(fixture.ctx, task.RevisionID, "receipt")
		requireInjectedFailure(t, err)
	})

	t.Run("projection missing and query failure", func(t *testing.T) {
		fixture := newFaultFixture(t)
		if _, err := fixture.repository.scenarioDevelopment(fixture.ctx, uuid.NewString()); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("missing projection error=%v, want not found", err)
		}
		fixture.fault.activate("query_row", "FROM scenario_developments", 1)
		_, err := fixture.repository.scenarioDevelopment(fixture.ctx, uuid.NewString())
		requireInjectedFailure(t, err)
	})

	t.Run("page projection query failure", func(t *testing.T) {
		fixture := newFaultFixture(t)
		detail := queueFaultScenario(t, fixture)
		fixture.fault.activate("query_row", "FROM scenario_developments", 1)
		_, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, detail.Page.ID)
		requireInjectedFailure(t, err)
	})

	t.Run("page projection tolerates no development row", func(t *testing.T) {
		fixture := newFaultFixture(t)
		detail := queueFaultScenario(t, fixture)
		if _, err := fixture.pool.Exec(fixture.ctx, `DELETE FROM scenario_developments WHERE revision_id = $1`, detail.ApprovedRevision.ID); err != nil {
			t.Fatal(err)
		}
		detail, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, detail.Page.ID)
		if err != nil || detail.Development != nil {
			t.Fatalf("development=%+v err=%v, want nil and nil", detail.Development, err)
		}
	})
}

func TestApprovingScenarioPropagatesQueueInsertFailure(t *testing.T) {
	fixture := newFaultFixture(t)
	pageID := createFaultScenario(t, fixture)
	detail, err := fixture.repository.PageDetail(fixture.ctx, fixture.organizationID, pageID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.fault.activate("tx_exec", "INSERT INTO scenario_developments", 1)
	_, err = fixture.repository.ApproveRevision(fixture.ctx, fixture.organizationID, fixture.userID, detail.DraftRevision.ID)
	requireInjectedFailure(t, err)
}

func TestPageDetailPropagatesNestedReadFailures(t *testing.T) {
	tests := []struct {
		name, method, contains string
		occurrence             int
		setup                  func(*testing.T, *faultFixture) string
	}{
		{name: "page", method: "query_row", contains: "WHERE p.organization_id = $1 AND p.id = $2", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "approved revision", method: "query_row", contains: "FROM revisions r", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "draft revision", method: "query_row", contains: "FROM revisions r", occurrence: 2, setup: func(t *testing.T, f *faultFixture) string { _ = saveFaultDraft(t, f); return f.pageID }},
		{name: "revision summaries", method: "query", contains: "FROM revisions r JOIN users", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "comments", method: "query", contains: "FROM comments c", occurrence: 1, setup: func(_ *testing.T, f *faultFixture) string { return f.pageID }},
		{name: "scenario parent review", method: "query_row", contains: "SELECT COALESCE(dr.title, ar.title, p.slug), p.approved_revision_id IS NOT NULL", occurrence: 1, setup: createFaultScenario},
		{name: "review objections", method: "query", contains: "FROM objections o", occurrence: 1, setup: func(t *testing.T, f *faultFixture) string { _ = saveFaultDraft(t, f); return f.pageID }},
		{name: "review objection scan", method: "rows_scan", contains: "FROM objections o", occurrence: 1, setup: func(t *testing.T, f *faultFixture) string {
			draft := saveFaultDraft(t, f)
			if _, err := f.repository.AddObjection(f.ctx, f.organizationID, f.userID, draft.ID, "Reason"); err != nil {
				t.Fatal(err)
			}
			return f.pageID
		}},
		{name: "review objection iteration", method: "rows_error", contains: "FROM objections o", occurrence: 1, setup: func(t *testing.T, f *faultFixture) string { _ = saveFaultDraft(t, f); return f.pageID }},
		{name: "feature children query", method: "query", contains: "p.parent_id = $2", occurrence: 1, setup: func(t *testing.T, f *faultFixture) string { return createFaultFeature(t, f).Page.ID }},
		{name: "feature children iteration", method: "rows_error", contains: "p.parent_id = $2", occurrence: 1, setup: func(t *testing.T, f *faultFixture) string { return createFaultFeature(t, f).Page.ID }},
		{name: "feature development progress", method: "query_row", contains: "count(*) FILTER", occurrence: 1, setup: func(t *testing.T, f *faultFixture) string { return createFaultFeature(t, f).Page.ID }},
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
			Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{}},
		}); err != nil {
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

	t.Run("revision batch load", func(t *testing.T) {
		fixture := newFaultFixture(t)
		fixture.fault.activate("query_row", "FROM revisions r", 1)
		_, err := fixture.repository.revisionsByID(fixture.ctx, []string{fixture.revisionID})
		requireInjectedFailure(t, err)
	})

	t.Run("step scan", func(t *testing.T) {
		fixture := newFaultFixture(t)
		feature := createFaultFeature(t, fixture)
		parentID := feature.Page.ID
		scenario, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
			Kind: model.PageScenario, ParentID: &parentID, Slug: "scenario-" + uuid.NewString(),
			Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{}},
		})
		if err != nil {
			t.Fatal(err)
		}
		fixture.fault.activate("rows_scan", "FROM bdd_steps", 1)
		_, err = fixture.repository.loadRevision(fixture.ctx, scenario.DraftRevision.ID)
		requireInjectedFailure(t, err)
	})

	t.Run("reference scan", func(t *testing.T) {
		fixture := newFaultFixture(t)
		feature := createFaultFeature(t, fixture)
		fixture.fault.activate("rows_scan", "FROM page_references", 1)
		_, err := fixture.repository.loadRevision(fixture.ctx, feature.ApprovedRevision.ID)
		requireInjectedFailure(t, err)
	})
}

func runAIChangeSetTx(t *testing.T, fixture *faultFixture, changeSet model.AIChangeSet) ([]string, error) {
	t.Helper()
	tx, err := fixture.repository.pool.BeginTx(fixture.ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(fixture.ctx) }()
	return fixture.repository.applyAIChangeSetTx(fixture.ctx, tx, fixture.organizationID, fixture.userID, changeSet)
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
	fixture.fault.activate("tx_query_row", "INSERT INTO pages", 1)
	_, err := fixture.repository.ApplyAIChangeSet(fixture.ctx, fixture.organizationID, fixture.userID, model.AssistantMutationContext{}, simpleConceptChangeSet("apply-"+uuid.NewString()))
	requireInjectedFailure(t, err)
	fixture.fault.activate("tx_exec", "INSERT INTO audit_events", 1)
	_, err = fixture.repository.ApplyAIChangeSet(fixture.ctx, fixture.organizationID, fixture.userID, model.AssistantMutationContext{}, simpleConceptChangeSet("audit-"+uuid.NewString()))
	requireInjectedFailure(t, err)
}

func TestApplyAIChangeSetCreateOperationFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	unknownParent := simpleConceptChangeSet("unknown-parent")
	unknownParent.Operations[0].ParentClientKey = "missing"
	if _, err := runAIChangeSetTx(t, fixture, unknownParent); err == nil {
		t.Fatal("expected unknown parent key")
	}
	unknownReference := simpleConceptChangeSet("unknown-reference")
	unknownReference.Operations[0].Content.References = []model.PageReference{{TargetClientKey: "missing", Relation: "uses"}}
	if _, err := runAIChangeSetTx(t, fixture, unknownReference); err == nil {
		t.Fatal("expected unknown reference key")
	}
	invalidSlug := simpleConceptChangeSet("Invalid slug")
	if _, err := runAIChangeSetTx(t, fixture, invalidSlug); err == nil {
		t.Fatal("expected invalid slug")
	}
	invalidContent := simpleConceptChangeSet("invalid-content")
	invalidContent.Operations[0].Content.Title = ""
	if _, err := runAIChangeSetTx(t, fixture, invalidContent); err == nil {
		t.Fatal("expected invalid content")
	}

	feature := createFaultFeature(t, fixture)
	parentID := fixture.pageID
	invalidParent := model.AIChangeSet{Operations: []model.AIChangeOperation{{
		Operation: "create", Kind: model.PageScenario, ParentID: &parentID, Slug: "invalid-parent", Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{}},
	}}}
	if _, err := runAIChangeSetTx(t, fixture, invalidParent); !errors.Is(err, store.ErrInvalidHierarchy) {
		t.Fatalf("expected invalid hierarchy, got %v", err)
	}
	_ = feature

	duplicate := simpleConceptChangeSet(fixture.repository.repositorySlug(t, fixture.pageID))
	if _, err := runAIChangeSetTx(t, fixture, duplicate); !errors.Is(err, store.ErrDuplicateSlug) {
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
			_, err := runAIChangeSetTx(t, fixture, simpleConceptChangeSet("create-"+uuid.NewString()))
			requireInjectedFailure(t, err)
		})
	}

	unsupported := simpleConceptChangeSet("unsupported")
	unsupported.Operations[0].Operation = "delete"
	if _, err := runAIChangeSetTx(t, fixture, unsupported); err == nil {
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
			Content: model.RevisionContent{Title: "Feature", Steps: []model.Step{}, References: []model.PageReference{{TargetClientKey: "concept", Relation: "uses"}}},
		},
		{
			Operation: "create", Kind: model.PageScenario, ParentClientKey: "feature", Slug: "scenario-" + uuid.NewString(),
			Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{{TargetClientKey: "concept", Relation: "uses"}}},
		},
	}}
	ids, err := runAIChangeSetTx(t, fixture, changeSet)
	if err != nil || len(ids) != 3 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

func TestApplyAIChangeSetReviseOperationFailures(t *testing.T) {
	fixture := newFaultFixture(t)
	if ids, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID)); err != nil || len(ids) != 1 {
		t.Fatalf("successful revise ids=%v err=%v", ids, err)
	}
	missingIDs := model.AIChangeSet{Operations: []model.AIChangeOperation{{Operation: "revise"}}}
	if _, err := runAIChangeSetTx(t, fixture, missingIDs); err == nil {
		t.Fatal("expected revise identifiers")
	}
	missing := reviseFaultChangeSet(t, fixture, fixture.revisionID)
	missingPageID := uuid.NewString()
	missing.Operations[0].PageID = &missingPageID
	if _, err := runAIChangeSetTx(t, fixture, missing); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing page, got %v", err)
	}

	fixture.fault.activate("tx_query_row", "FROM pages WHERE id = $1", 1)
	if _, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID)); !errors.Is(err, errInjectedDatabase) {
		t.Fatalf("expected page lookup failure, got %v", err)
	}
	fixture.fault.activate("", "", 0)

	mismatch := reviseFaultChangeSet(t, fixture, fixture.revisionID)
	mismatch.Operations[0].Slug = "wrong-slug"
	if _, err := runAIChangeSetTx(t, fixture, mismatch); err == nil {
		t.Fatal("expected immutable metadata mismatch")
	}
	conflict := reviseFaultChangeSet(t, fixture, uuid.NewString())
	if _, err := runAIChangeSetTx(t, fixture, conflict); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	invalid := reviseFaultChangeSet(t, fixture, fixture.revisionID)
	invalid.Operations[0].Content.Title = ""
	if _, err := runAIChangeSetTx(t, fixture, invalid); err == nil {
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
			_, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, fixture.revisionID))
			requireInjectedFailure(t, err)
		})
	}

	t.Run("supersede draft", func(t *testing.T) {
		fixture := newFaultFixture(t)
		draft := saveFaultDraft(t, fixture)
		fixture.fault.activate("tx_exec", "AND status = 'draft'", 1)
		_, err := runAIChangeSetTx(t, fixture, reviseFaultChangeSet(t, fixture, draft.ID))
		requireInjectedFailure(t, err)
	})
}

func TestScenarioParentLookupFailureIsRejected(t *testing.T) {
	fixture := newFaultFixture(t)
	feature := createFaultFeature(t, fixture)
	parentID := feature.Page.ID
	fixture.fault.activate("tx_query_row", "SELECT kind FROM pages", 1)
	_, err := fixture.repository.CreatePage(fixture.ctx, fixture.organizationID, fixture.userID, model.CreatePageInput{
		Kind: model.PageScenario, ParentID: &parentID, Slug: "scenario-" + uuid.NewString(),
		Content: model.RevisionContent{Title: "Scenario", Steps: scenarioSteps(), References: []model.PageReference{}},
	})
	if !errors.Is(err, store.ErrInvalidHierarchy) {
		t.Fatalf("expected invalid hierarchy, got %v", err)
	}
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
