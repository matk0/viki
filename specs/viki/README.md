# Viki describing itself

This catalogue describes current Viki behavior in the same concepts, features,
and scenarios that Viki presents to its readers. Its intended public audience is
someone deciding whether to use Viki or trying to understand how it works.

Start with [specifications](concepts/specification.md), then explore
[review](review.feature), [repository publication](repository-publication.feature),
[assistant permissions](assistant.feature), and [Slovak presentation](presentation.feature).

## Concepts

| Concept | Explains |
| --- | --- |
| [Specification](concepts/specification.md) | The catalogue's concepts, features, and concrete scenarios. |
| [Revision](concepts/revision.md) | Content versions, drafts, and repository baselines. |
| [Authority and evidence](concepts/authority.md) | The different meanings of human approval and CI execution. |
| [Assistant](concepts/assistant.md) | Read-only answers, citations, and proposed draft changes. |
| [Architecture and the public demo](concepts/architecture.md) | The full runtime, the Cloudflare adapter, and visitor privacy. |

## Relationship to the implementation

The feature files use Viki's supported Gherkin subset and stable source IDs.
They are explanatory sources, not executable acceptance tests yet. They have no
Cucumber step bindings or eligible CI artifact. Existing tests exercise the
underlying behavior; the table below is a reading guide, not an automated mapping
or a claim that every sentence has an executable assertion.

| Specification | Implementation and existing tests |
| --- | --- |
| [Review](review.feature) | [Governance policy](../../backend/internal/governance/policy.go), [policy tests](../../backend/internal/governance/policy_test.go), [review persistence](../../backend/internal/postgres/review.go), [review-state integration tests](../../backend/internal/postgres/review_state_integration_test.go). |
| [Repository publication](repository-publication.feature) | [Snapshot validation](../../backend/internal/specsync/snapshot.go), [poller](../../backend/internal/specsync/poller.go), [import transactions](../../backend/internal/postgres/repository_sync.go), [API boundary tests](../../backend/internal/app/repository_sync_test.go), [business revision integration tests](../../backend/internal/postgres/repository_business_revision_integration_test.go). |
| [Assistant permissions](assistant.feature) | [Profile-scoped tools](../../backend/internal/app/assistant_tools.go), [tool tests](../../backend/internal/app/assistant_tools_test.go), [Q&A instructions](../../hermes/profiles/qa/SOUL.md), [Edit instructions](../../hermes/profiles/edit/SOUL.md). |
| [Slovak presentation](presentation.feature) | [Translation worker](../../backend/internal/localize/localize.go), [worker tests](../../backend/internal/localize/localize_test.go), [frontend projection](../../frontend/src/localization.ts), [translation integration tests](../../backend/internal/postgres/localization_integration_test.go). |

For a full Viki installation to import this catalogue with CI evidence, the next step is to bind these scenarios
to real acceptance checks, run them in CI, and publish the resulting sources and
Cucumber Messages under the [version-2 contract](../../backend/internal/specsync/contract/CONTRACT.md).
Descriptive Gherkin and unit test results alone do not satisfy that contract.

The [public demo](../../public-demo/README.md) bundles these descriptive sources
directly at build time and reuses Viki's reader. It has anonymous browsing,
browser-tab-local conversations, and a separate Workers Q&A endpoint. It has no
access to the private installation or its data. The full private application
continues to use its existing bootstrap account and Hermes conversations.
