# Gherkin snapshot contract, version 2

The consumer schema is `snapshot-v2.schema.json`. New publications use `format: "gherkin-specification"`, top-level `schemaVersion: 2` and `verification.schemaVersion: 2`. The frozen `snapshot-v1.schema.json` records the historical Vozokany producer contract; its `vozokany-business-specification` / version 1 envelope remains accepted by the consumer as a wire compatibility alias. Both envelopes use the same generic semantic validation. `../testdata/snapshot-v1.valid.json` is retained synthetic evidence for consumer contract tests. Its zero source commit and message hash are deliberate: it must never be eligible for import. This fixture demonstrates shape and hashing, not an executed acceptance run.

The producer publishes an archive containing exactly `snapshot.json`, `cucumber-messages.ndjson` and `execution.json` at its root. Its local output directory is application-owned. A successful **push to main**, after every required verification step, uploads the single GitHub Actions v4 artifact named:

`<configured-artifact-prefix>-<40-character-source-sha>-<run-id>-<run-attempt>`

The default prefix is `gherkin-specs-v2`. The existing Vozokany deployment explicitly configures `vozokany-business-specs-v1`. Repository identity comes from the authenticated workspace configuration; the worker workflow is configured with `VIKI_GIT_WORKFLOW` (default `.github/workflows/verify.yml`). Neither is inferred from untrusted artifact metadata.

PR evidence uses the same name but `publication.kind = pull-request` and `eligibleForImport = false`. Local evidence always has kind `local`, event `local`, and is ineligible, even on a branch named main. Importers must independently verify the trusted repository, push event, `refs/heads/main`, workflow, successful completed GitHub run, matching head SHA/run ID/attempt and artifact identity. Snapshot metadata alone is not an authentication receipt. No field attests human approval or production deployment.

## Exact content and deterministic hashes

Strings contain the original UTF-8 source, including all comments, tags, prose, blank lines and final newline. Sources must use UTF-8 without BOM, LF newlines, and contain no NUL. Do not normalize Unicode, line endings, whitespace, or rewrite prose. Paths are repository-relative ASCII with `/` separators and cannot contain `..` segments.

`contentHash` on each document is `sha256:` followed by lowercase hex SHA-256 of the exact UTF-8 bytes of `content`. It excludes all run/source metadata.

`source.contentHash` and `verification.sourceContentHash` hash the UTF-8 concatenation of all documents sorted by **ASCII path bytes**, each rendered as `path + NUL + document.contentHash + LF`. There is no JSON canonicalization. `verification.messagesHash` hashes the raw bytes of the retained Cucumber message stream. Decimal nanoseconds are strings to avoid cross-language numeric precision differences.

## Stable identity and supported language

Every source identity is an opaque, case-sensitive string matching `[A-Za-z0-9][A-Za-z0-9_.-]{0,127}`. IDs are unique across concepts, features and scenarios; kind is derived from the source structure, never an ID prefix. IDs must survive renames and moves.

Concept Markdown begins with `<!-- business-id: <id> -->`; optional `<!-- relates-to: <id>, <id> -->` declares references. The first level-one Markdown heading is its title. Feature tags contain exactly one `@id:<id>` and optional `@concept:<id>` references. Every scenario has exactly one `@id:<id>` tag. Concepts and concept references are optional; a catalogue may consist only of features and scenarios. Paths are relative to the repository and need not live under `specs/business/`.

Only English `Feature`, `Scenario`, `Given`, `When`, `Then`, `And`, `But`, descriptions, comments and the metadata tags above are accepted. Background, Rule, outlines/examples, tables, doc strings, wildcard steps, extra tags and other languages fail. Use the official Gherkin AST/parser, never a regex approximation. Scenario source locations are one-based. Step keywords exclude trailing spaces; texts and descriptions are the parser's exact values. Full source content is authoritative. Each scenario starts with `Given` and includes `When` and `Then`, matching the existing supported subset.

## Versioned semantic checks

In addition to JSON Schema validation, both sides reject duplicate/missing IDs or paths, dangling references, cross-kind identity collisions, wrong hashes, source/results commit or content mismatch, unsupported syntax and differing source/executed scenario sets. Each scenario must have nonempty steps and exactly one completed passing execution, with identical ordered step texts and every step passing. The producer also validates hooks, test-run completion and real executable step matches from the official Cucumber Messages stream; a dry run is never proof. Neither side may silently ignore fields, flatten unsupported constructs, or import partial evidence. Validation rules belong to the consumer contract version; incompatible additions require an explicitly brokered contract revision.

## Execution boundary

The application owns step definitions, runtime selection, fixtures and business assertions. Viki never runs the reported command or application code. `verification.backend`, `runner.name`, `runner.version` and `command` are nonempty informational strings, not a Rust/Node/runner allowlist. Viki validates the standard Cucumber Messages stream: exact executed sources, scenario identities, ordered steps, one real step definition per executed step, passing hooks/results, durations and complete non-retried execution. The message digest and copied execution summary must match the snapshot.

Producer-specific attachments (including historical Rust fixture receipts) are ignored as application evidence. A successful configured CI workflow remains the authority for application-specific checks. Changing an attachment does not bypass the archive or message digest checks. The `execution.json` wrapper retains the existing clean source fingerprint and exact publication/verification copy checks.

## Compatibility and scope

No source IDs, exact source bytes, content hashes or stored business hashes are rewritten. Re-publishing the same content in the v2 envelope under a new CI receipt reuses existing revisions and translations. A retry of an existing run/attempt must still be byte-equivalent after JSON decoding; do not republish a different envelope under the same run identity. No database migration or producer change is needed for existing v1 publications.

This refactoring preserves the existing explicit Gherkin subset above; it does not add outlines, backgrounds, rules, doc strings, data tables, extra tags or additional Gherkin languages. A different application's end-to-end integration remains a separate validation task.
