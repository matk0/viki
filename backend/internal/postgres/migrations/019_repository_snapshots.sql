-- Repository baselines are separate from drafts/human approvals and development queues.
CREATE TABLE workspace_repository_sync (
 organization_id UUID PRIMARY KEY REFERENCES organizations(id),
 repository TEXT NOT NULL,
 current_snapshot_id UUID,
 last_checked_at TIMESTAMPTZ,
 last_error TEXT NOT NULL DEFAULT '',
 last_error_at TIMESTAMPTZ
);
CREATE TABLE repository_snapshots (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id UUID NOT NULL REFERENCES workspace_repository_sync(organization_id),
 source_commit TEXT NOT NULL,
 content_hash TEXT NOT NULL,
 run_id BIGINT NOT NULL,
 run_attempt INTEGER NOT NULL,
 snapshot JSONB NOT NULL,
 imported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,run_id,run_attempt)
);
ALTER TABLE workspace_repository_sync ADD CONSTRAINT workspace_current_snapshot_fk FOREIGN KEY(current_snapshot_id) REFERENCES repository_snapshots(id);
CREATE TABLE repository_pages (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id UUID NOT NULL REFERENCES workspace_repository_sync(organization_id),
 external_id TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('concept','feature','scenario')),
 current_revision_id UUID,
 archived BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,external_id)
);
CREATE TABLE repository_revisions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 page_id UUID NOT NULL REFERENCES repository_pages(id),
 number INTEGER NOT NULL,
 business_hash TEXT NOT NULL,
 entry JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(page_id,business_hash),
 UNIQUE(page_id,number)
);
ALTER TABLE repository_pages ADD CONSTRAINT repository_current_revision_fk FOREIGN KEY(current_revision_id) REFERENCES repository_revisions(id);
CREATE TABLE repository_snapshot_entries (
 snapshot_id UUID NOT NULL REFERENCES repository_snapshots(id),
 page_id UUID NOT NULL REFERENCES repository_pages(id),
 revision_id UUID NOT NULL REFERENCES repository_revisions(id),
 entry JSONB NOT NULL,
 PRIMARY KEY(snapshot_id,page_id)
);
-- No deletion is required when a source disappears. Immutable snapshots keep its history.

CREATE FUNCTION reject_git_managed_content() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE workspace UUID;
BEGIN
 IF TG_TABLE_NAME IN ('pages','step_definitions') THEN
  workspace := NEW.organization_id;
 ELSE
  SELECT organization_id INTO workspace FROM pages WHERE id = NEW.page_id;
 END IF;
 IF EXISTS(SELECT 1 FROM workspace_repository_sync WHERE organization_id=workspace) THEN
  RAISE EXCEPTION 'workspace specifications are managed in Git and are read-only' USING ERRCODE='25006';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER pages_git_managed BEFORE INSERT OR UPDATE ON pages FOR EACH ROW EXECUTE FUNCTION reject_git_managed_content();
CREATE TRIGGER revisions_git_managed BEFORE INSERT OR UPDATE ON revisions FOR EACH ROW EXECUTE FUNCTION reject_git_managed_content();
CREATE TRIGGER definitions_git_managed BEFORE INSERT OR UPDATE ON step_definitions FOR EACH ROW EXECUTE FUNCTION reject_git_managed_content();
