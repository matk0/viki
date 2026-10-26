# Business snapshot contract, version 1

The tracked JSON Schema is `snapshot-v1.schema.json`; `fixtures/snapshot-v1.valid.json` is synthetic local evidence for consumer contract tests. Its zero source commit and message hash are deliberate: it must never be eligible for import. This fixture demonstrates shape and hashing, not an executed acceptance run.

The producer writes `artifacts/business-specifications/snapshot.json` and retains `cucumber-messages.ndjson` plus `execution.json` alongside it. A successful **push to main**, after every required verification step, uploads the single GitHub Actions v4 artifact named:

`vozokany-business-specs-v1-<40-character-source-sha>-<run-id>-<run-attempt>`

PR evidence uses the same name but `publication.kind = pull-request` and `eligibleForImport = false`. Local evidence always has kind `local`, event `local`, and is ineligible, even on a branch named main. Importers must independently verify the trusted repository, push event, `refs/heads/main`, workflow, successful completed GitHub run, matching head SHA/run ID/attempt and artifact identity. Snapshot metadata alone is not an authentication receipt. No field attests human approval or production deployment.

## Exact content and deterministic hashes

Strings contain the original UTF-8 source, including all comments, tags, prose, blank lines and final newline. Sources must use UTF-8 without BOM, LF newlines, and contain no NUL. Do not normalize Unicode, line endings, whitespace, or rewrite prose. Paths are repository-relative ASCII with `/` separators and cannot contain `..` segments.

`contentHash` on each document is `sha256:` followed by lowercase hex SHA-256 of the exact UTF-8 bytes of `content`. It excludes all run/source metadata.

`source.contentHash` and `verification.sourceContentHash` hash the UTF-8 concatenation of all documents sorted by **ASCII path bytes**, each rendered as `path + NUL + document.contentHash + LF`. There is no JSON canonicalization. `verification.messagesHash` hashes the raw bytes of the retained Cucumber message stream. Decimal nanoseconds are strings to avoid cross-language numeric precision differences.

## Stable identity and supported language

Concept Markdown begins with `<!-- business-id: VOZ-CON-... -->`; optional `<!-- relates-to: VOZ-CON-..., VOZ-CON-... -->` declares references. The first level-one Markdown heading is its title. Feature tags are `@id:VOZ-FEAT-...` and one or more `@concept:VOZ-CON-...`. Every scenario has exactly one `@id:VOZ-SC-...` tag. IDs are permanent; changing a name or path must preserve its ID. Renaming a scenario never creates a second result identity.

Only English `Feature`, `Scenario`, `Given`, `When`, `Then`, `And`, `But`, descriptions, comments and the metadata tags above are accepted. Background, Rule, outlines/examples, tables, doc strings, wildcard steps, extra tags and other languages fail. Use the official Gherkin AST/parser, never a regex approximation. Scenario source locations are one-based. Step keywords exclude trailing spaces; texts and descriptions are the parser's exact values. Full source content is authoritative.

## Versioned semantic checks

In addition to JSON Schema validation, both sides reject duplicate/missing IDs or paths, dangling references, wrong ID kind, wrong hashes, source/results commit or content mismatch, unsupported syntax and differing source/executed scenario sets. Each scenario must have nonempty steps and exactly one completed passing execution, with identical ordered step texts and every step passing. The producer also validates hooks, test-run completion and real executable step matches from the official Cucumber Messages stream; a dry run is never proof. Neither side may silently ignore fields, flatten unsupported constructs, or import partial evidence. Validation rules belong to schema version 1; incompatible additions require an explicitly brokered contract revision.
