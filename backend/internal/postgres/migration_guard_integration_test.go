package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestHermesMigrationRefusesToDiscardLegacyTranscriptData(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	if _, err := repository.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatal(err)
	}
	var hermesMigrationApplied bool
	if err := repository.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = '002_hermes_assistant.sql')`).Scan(&hermesMigrationApplied); err != nil {
		t.Fatal(err)
	}
	if hermesMigrationApplied {
		t.Skip("requires a fresh integration database before the Hermes migration")
	}
	var initialMigrationApplied bool
	if err := repository.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = '001_initial.sql')`).Scan(&initialMigrationApplied); err != nil {
		t.Fatal(err)
	}
	if !initialMigrationApplied {
		initialSQL, err := migrationFiles.ReadFile("migrations/001_initial.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repository.pool.Exec(ctx, string(initialSQL)); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.pool.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES ('001_initial.sql')`); err != nil {
			t.Fatal(err)
		}
	}

	organizationID, userID, chatID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := repository.pool.Exec(ctx, `INSERT INTO organizations(id, name) VALUES ($1, 'Legacy migration')`, organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `INSERT INTO users(id, organization_id, email, display_name, password_hash) VALUES ($1, $2, $3, 'Legacy', 'unused')`, userID, organizationID, userID+"@viki.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `INSERT INTO chats(id, organization_id, user_id, title) VALUES ($1, $2, $3, 'Legacy')`, chatID, organizationID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `INSERT INTO chat_messages(chat_id, role, mode, content) VALUES ($1, 'user', 'qa', 'must survive')`, chatID); err != nil {
		t.Fatal(err)
	}

	err = repository.Migrate(ctx)
	if err == nil || !strings.Contains(err.Error(), "chat_messages is not empty") {
		t.Fatalf("migration error = %v, want explicit legacy transcript refusal", err)
	}
	var retained string
	if err := repository.pool.QueryRow(ctx, `SELECT content FROM chat_messages WHERE chat_id = $1`, chatID).Scan(&retained); err != nil || retained != "must survive" {
		t.Fatalf("legacy transcript was not retained after refusal: value=%q error=%v", retained, err)
	}

	if _, err := repository.pool.Exec(ctx, `DELETE FROM chat_messages WHERE chat_id = $1`, chatID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `DELETE FROM chats WHERE id = $1`, chatID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, organizationID); err != nil {
		t.Fatal(err)
	}
	if err := repository.Migrate(ctx); err != nil {
		t.Fatalf("migration did not proceed after legacy data was explicitly removed: %v", err)
	}
}

func TestMigrationsRemoveIllustrativeData(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var exists bool
	if err := repository.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'revisions' AND column_name = 'illustrative'
		)
	`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("revisions.illustrative still exists after migrations")
	}

	var proposalCount int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM assistant_draft_proposals
		WHERE jsonb_path_exists(changeset, '$.**.illustrative')
	`).Scan(&proposalCount); err != nil {
		t.Fatal(err)
	}
	if proposalCount != 0 {
		t.Fatalf("%d assistant proposals still contain illustrative metadata", proposalCount)
	}
}

func TestMigrationsRemoveRevisionProvenanceData(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var columnCount int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'revisions'
		  AND column_name IN ('source_urls', 'provenance')
	`).Scan(&columnCount); err != nil {
		t.Fatal(err)
	}
	if columnCount != 0 {
		t.Fatalf("%d revision provenance columns still exist after migrations", columnCount)
	}

	var proposalCount int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM assistant_draft_proposals
		WHERE jsonb_path_exists(changeset, '$.operations[*].content.sourceUrls')
		   OR jsonb_path_exists(changeset, '$.operations[*].content.provenance')
	`).Scan(&proposalCount); err != nil {
		t.Fatal(err)
	}
	if proposalCount != 0 {
		t.Fatalf("%d assistant proposals still contain provenance data", proposalCount)
	}
}

func TestMigrationsRemoveCommentAnchorData(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var columnCount int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'comments'
		  AND column_name IN ('anchor_kind', 'anchor_id')
	`).Scan(&columnCount); err != nil {
		t.Fatal(err)
	}
	if columnCount != 0 {
		t.Fatalf("%d comment anchor columns still exist after migrations", columnCount)
	}
}

func TestMigrationsRemoveRevisionAliases(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var aliasColumnExists bool
	if err := repository.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'revisions' AND column_name = 'aliases'
		)
	`).Scan(&aliasColumnExists); err != nil {
		t.Fatal(err)
	}
	if aliasColumnExists {
		t.Fatal("revisions.aliases still exists after migrations")
	}

	var proposalCount int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*) FROM assistant_draft_proposals
		WHERE jsonb_path_exists(changeset, '$.operations[*].content.aliases')
	`).Scan(&proposalCount); err != nil {
		t.Fatal(err)
	}
	if proposalCount != 0 {
		t.Fatalf("%d assistant proposals still contain aliases", proposalCount)
	}
}

func TestMigrationsUseFirstClassObjectionStorage(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var objectionTable, voteTable bool
	if err := repository.pool.QueryRow(ctx, `SELECT to_regclass('public.objections') IS NOT NULL, to_regclass('public.votes') IS NOT NULL`).Scan(&objectionTable, &voteTable); err != nil {
		t.Fatal(err)
	}
	if !objectionTable || voteTable {
		t.Fatalf("objections table=%v votes table=%v, want first-class objections without votes", objectionTable, voteTable)
	}

	var legacyCommentColumns int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'comments'
		  AND column_name IN ('blocking', 'resolved_at', 'resolved_by')
	`).Scan(&legacyCommentColumns); err != nil {
		t.Fatal(err)
	}
	if legacyCommentColumns != 0 {
		t.Fatalf("comments retain %d objection-specific columns", legacyCommentColumns)
	}
}

func TestMigrationsUseApprovedRevisionLifecycle(t *testing.T) {
	databaseURL := os.Getenv("VIKI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set VIKI_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var approvedColumns int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND ((table_name = 'pages' AND column_name = 'approved_revision_id')
		    OR (table_name = 'revisions' AND column_name = 'approved_at'))
	`).Scan(&approvedColumns); err != nil {
		t.Fatal(err)
	}
	if approvedColumns != 2 {
		t.Fatalf("approved lifecycle columns = %d, want 2", approvedColumns)
	}

	var legacyColumns int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND ((table_name = 'pages' AND column_name = 'accepted_revision_id')
		    OR (table_name = 'revisions' AND column_name = 'accepted_at'))
	`).Scan(&legacyColumns); err != nil {
		t.Fatal(err)
	}
	if legacyColumns != 0 {
		t.Fatalf("legacy accepted lifecycle columns = %d, want 0", legacyColumns)
	}

	var approvedStatusAllowed, acceptedStatusAllowed bool
	if err := repository.pool.QueryRow(ctx, `
		SELECT
		  pg_get_constraintdef(oid) LIKE '%approved%' AS approved,
		  pg_get_constraintdef(oid) LIKE '%accepted%' AS accepted
		FROM pg_constraint
		WHERE conrelid = 'revisions'::regclass AND conname = 'revisions_status_check'
	`).Scan(&approvedStatusAllowed, &acceptedStatusAllowed); err != nil {
		t.Fatal(err)
	}
	if !approvedStatusAllowed || acceptedStatusAllowed {
		t.Fatalf("revision status constraint: approved=%v accepted=%v", approvedStatusAllowed, acceptedStatusAllowed)
	}

	var currentRevisionIndexes int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_indexes
		WHERE schemaname = 'public'
		  AND indexname IN ('revisions_one_draft_per_page_idx', 'revisions_one_approved_per_page_idx')
	`).Scan(&currentRevisionIndexes); err != nil {
		t.Fatal(err)
	}
	if currentRevisionIndexes != 2 {
		t.Fatalf("current revision indexes = %d, want 2", currentRevisionIndexes)
	}

	var orphanedApprovedPages int
	if err := repository.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM pages AS page
		WHERE page.approved_revision_id IS NULL
		  AND EXISTS (
		      SELECT 1 FROM revisions AS revision
		      WHERE revision.page_id = page.id AND revision.status = 'approved'
		  )
	`).Scan(&orphanedApprovedPages); err != nil {
		t.Fatal(err)
	}
	if orphanedApprovedPages != 0 {
		t.Fatalf("orphaned pages with an approved revision = %d, want 0", orphanedApprovedPages)
	}
}
