# Repository-managed business specifications

An explicitly configured workspace mirrors the complete canonical English business catalogue from `matk0/vozokany`. Other workspaces retain Viki's existing human-governed draft/review behavior. Git-managed specifications are read-only: no independent page/draft/approval or Hermes Edit fork, and no developer queue. Repository CI evidence never means human approval or production deployment.

The producer owns schema version 1. Its schema, contract and minimal synthetic consumer fixture are mirrored under `backend/internal/specsync/contract/` and `testdata/`. Exact document UTF-8 bytes are hashed with SHA-256. The catalogue hashes ASCII-sorted `path + NUL + contentHash + LF` records. The official Go Gherkin parser independently derives the displayed fields and compares them with the snapshot. A modified parsed title, description, ID, parent, relationship, location or step cannot hide behind an unchanged source hash. Unsupported Gherkin is rejected.

Repository pages and immutable business revisions use separate database tables and internal UUIDs. They never call `ApproveRevision`, have no human approval timestamp and create no development task. Stable source IDs map to persistent internal page identities. Content/run metadata is recorded per immutable snapshot; identical business content reuses its revision even when code/source/run/path metadata changes. A complete import locks its workspace, writes all entries and switches the current snapshot in one transaction. Removed entries leave current catalogue, search and Q&A but their versions remain addressable through authenticated history. A retry is idempotent; stale runs, conflicts and interrupted imports retain the last committed snapshot. Database triggers also reject writes to the human content tables for a Git-managed workspace.

## Private importer and poller

`VIKI_GIT_REPOSITORY=https://github.com/matk0/vozokany` explicitly enables the initial workspace during startup. Activation refuses to hide a pre-existing human-authored catalogue. The mode persists in the database. Set a new random `VIKI_SNAPSHOT_IMPORT_TOKEN` of at least 32 characters and keep `VIKI_DEVELOPER_ENABLED=false`. All internal sync routes require that separate bearer token, never a browser session or the Hermes tool token. They are registered only on Viki's internal listener. The public authenticated `GET /api/v1/workspace/sync` exposes authority, exact last import, last completed poll, freshness and errors.

`viki-sync` polls every five minutes with a 90-second attempt deadline, bounded HTTP retries, ordinary Docker restart behavior and rotated logs. It has no LLM, shell execution, repository write access, database credential or model-provider key. It reads only the approved private repository through a new dedicated fine-grained token: **resource owner matk0; only repository vozokany; Actions read, Contents read, Metadata read; no write permissions**. Use a bounded expiry and replace it before expiry. A failed/expired token leaves the baseline intact and exposes an authentication error/stale polling state. GitHub documents these [fine-grained token controls](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens), [Actions read permission](https://docs.github.com/en/rest/actions/artifacts) and [Contents read permission](https://docs.github.com/en/rest/commits/commits).

The worker checks the live repository ID, workflow ID/path, push event, main branch and current main SHA, completed successful run and exact attempt, then the artifact's immutable-style name, run/repository/SHA association, expiry, size and SHA-256 archive digest. An older or diverged candidate cannot replace a newer baseline. Branch and run eligibility are checked again after download to close the in-flight window.

Expected artifact: `vozokany-business-specs-v1-<source-sha>-<run-id>-<attempt>`, containing exactly `snapshot.json`, `execution.json` and `cucumber-messages.ndjson`. ZIP contents are read with size/name/type limits and never executed or extracted onto the host. The execution wrapper is the producer's `{source:{commit,dirty,fingerprint},publication,verification}`; clean-source fingerprint and all copied provenance/results must match. The raw message hash, exact executed feature-source set, scenario set, ordered source/executable step texts, passing steps/hooks, single real step-definition matches, complete non-retried run and per-scenario Rust fixture receipts are independently checked. The local and PR publications are always ineligible for live import.

The UI polls authority/catalogue status every thirty seconds and keeps the last successful view on a failed refresh. Imported pages use Viki's standard catalogue, document reader and history dialog. Canonical English prose and Gherkin remain unchanged in storage. Repository identifiers, hashes, CI receipts, polling state and exact-source panels are not added to the reading interface; provenance remains available through the authenticated API and Q&A citations. Write controls are disabled and human approval/discussion panels are unavailable for imported pages. Q&A remains available and cites exact imported revisions.

## Verification and deployment boundary

Run the required `make test-coverage` gate with a fresh isolated test database. Meaningful tests cover malformed/altered source copies, ID/result set failures, unsupported syntax, metadata-only updates, rename/move/addition/removal/reappearance, stale/duplicate/conflicting/out-of-order/concurrent receipts, rollback at database interruption points, authenticated read-only API/Hermes boundaries and worker eligibility/transport/archive/execution failures. Existing human-mode tests remain.

To validate actual producer interchange locally without pretending it is live evidence:

```sh
cd backend
VIKI_TEST_PRODUCER_ARTIFACT_DIR=/absolute/path/to/vozokany/artifacts/business-specifications go test ./internal/specsync -run TestProducerArtifactInteroperability -count=1 -v
```

The interoperability test requires the local artifact to remain ineligible. The first validated full interchange was 20 documents, 103 scenarios and 349 business steps at producer commit `507082dee1f3740374d8a6c1e5cceccdbfbd91c2`, catalogue `sha256:cef3342d9b71ba6d4b343784665d1a6a83b109b2522387e7eb32b17d6effb5ae`.

`deploy/umbrel/compose.yaml` upgrades only the separate Vozokany Viki stack and adds the small outbound pull worker. Keep `.env` and `.sync.env` root-only. Ingress remains loopback plus existing private Tailscale Serve; no inbound CI access, webhook, public port or tailnet/firewall change is required. Preserve the existing Umbrel Hermes and all other home services.

Before upgrading, take the existing consistent instance backup and verify its dump into a disposable database. Retain the previous image IDs, source release, Compose/environment and pre-upgrade database dump together. Migration 019 is additive, but an image-only rollback cannot correctly display the imported catalogue: restore the matching pre-upgrade database schema and image/configuration as a unit, after first saving the upgraded state. Never delete the canonical repository specs or any other Umbrel app's data. No whole-host reboot or application production deployment belongs to this feature.

Live completion additionally requires genuine successful-main artifacts: the first automatic import and a subsequent automatic eligible main update must be observed. Unit fixtures, local exports and PR artifacts are not substitutes. Viki-to-Git editing/approval write-back, Gherkin execution by Viki, the software factory and Vozokany application deployment remain deferred.

## Slovak presentation of canonical English revisions

English remains the canonical source, including its exact source bytes, hashes,
IDs, verification receipts and read-only status. Optional `localizations.sk` in
catalogue/detail/revision/retrieval responses is derived content tied to the exact
English business revision. The existing locale switch selects that presentation
through one frontend projection. Titles, prose, BDD text, children, related labels,
search excerpts and historical readers use it without adding another page layout.
Missing translations show the current English revision with a small Slovak notice;
an older translation is never attached to a newer revision. Existing chat messages
retain their original text. New Q&A turns carry a validated `sk`/`en` response locale
in the signed host envelope; citations retain canonical revision IDs and may carry
localized titles. Search matches both languages, including Slovak diacritics.

Migration 020 adds translation versions and a durable job table separately from
business revisions. Set `VIKI_LOCALIZATION_ENABLED=true`, `OPENAI_API_KEY` and
optionally `VIKI_LOCALIZATION_MODEL` (default: the existing `gpt-5.6-terra`) to run
the background translator inside Viki. It uses the official Responses endpoint,
strict structured output, no tools, no stored provider responses and a fixed farm
terminology glossary. Reading never calls the model. Verified imports are independent
of translation availability. The Git poller retains its existing capabilities and
has no model credential. The Umbrel app receives the same existing provider key
already used by its dedicated Hermes; the key is never returned to the browser.

A database lease prevents two workers from claiming the same revision. Jobs have a
ten-minute lease and at most three attempts, with five-minute backoff after a
provider/validation failure. Cancellation releases runtime resources; an interrupted
lease becomes eligible after its deadline. Structure, step count/order and protected
numeric tokens, URLs, inline code, IDs and placeholders are checked before publishing
one complete translation. These checks do not prove semantic equivalence: review
business-critical Slovak wording and the initial backfill separately. Provider or
translation errors retain the English baseline. Repeated CI receipts reuse existing
business revisions and translations. Existing backups include both new tables.

Translation corrections append a translation version without changing an English
business revision. Operators can requeue an explicitly selected revision by resetting
its job's `completed=false, attempts=0, available_at=now()` and clearing its lease;
only do this after stopping localization and reviewing the change. Existing
translations remain visible until the complete replacement is stored. There is no
public translation-write route or human draft/approval workflow for this derived data.
An image rollback to the previous release can leave migration 020 and its additive
tables in place; preserve the pre-upgrade configuration, database and Hermes backup.
