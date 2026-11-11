# Viki

**A specification wiki for understanding what a system does, why it does it, and which behavior has been checked.**

Requirements tend to end up scattered across documents, conversations, code, and tests. Viki brings them into one linked catalogue: shared vocabulary, system capabilities, and concrete examples written in Gherkin. People can read the specification, trace related ideas, inspect earlier revisions, and ask an optional assistant questions with revision citations.

Viki has two ways to maintain that catalogue. A team can write and review specifications in the wiki, or keep them in Git and publish a read-only view after CI verifies them. These are separate sources of authority: a passing test run records execution evidence; a person's approval records agreement.

## Start with the behavior

A specification has three building blocks:

| Building block | Purpose | Example in Viki itself |
| --- | --- | --- |
| **Concept** | Define a shared term. | A revision is a version of a page's content. |
| **Feature** | Explain a capability and its purpose. | Review a proposed specification change before accepting it. |
| **Scenario** | Describe a concrete example using `Given` / `When` / `Then`. | An unresolved objection prevents approval. |

For example, one of Viki's review rules can be expressed as:

```gherkin
Feature: Review specification changes
  Proposed changes remain drafts until a person approves them.

  Scenario: An unresolved objection blocks approval
    Given a draft revision has an unresolved objection
    When a reviewer tries to approve the revision
    Then approval is rejected
    And the revision remains a draft
```

This is an explanatory example. To import a repository's scenarios with execution evidence, a publisher must provide the stable IDs, supported syntax, sources, and genuine Cucumber results required by the [publication contract](backend/internal/specsync/contract/CONTRACT.md).

## Two ways to use Viki

### Write and review in the wiki

Create concepts, features, and scenarios directly in Viki. Edits become versioned drafts. Reviewers can discuss a change, raise objections, and approve it when the blockers are resolved. An unresolved objection blocks approval, and a scenario requires an approved parent feature.

This mode includes comments, objections, reusable Gherkin steps, revision history, and an audit trail. The optional assistant can prepare linked draft revisions; people make the approval decision.

### Publish specifications from Git

Keep canonical English Markdown and Gherkin in the application repository, alongside the implementation and executable step definitions. Its CI runs the scenarios and publishes sources together with Cucumber execution evidence.

```mermaid
flowchart LR
  Repository[Application repository] --> CI[Successful main-branch CI]
  CI --> Artifact[Sources and Cucumber results]
  Artifact --> Import[Viki validates and imports]
  Import --> Reader[Read-only catalogue, search, and Q&A]
```

Viki polls the configured GitHub repository, checks the publication's provenance and execution results, and imports a complete snapshot atomically. Stable source IDs preserve page identity through renames and moves. Unchanged specification content reuses its revision; a failed import retains the last valid snapshot.

Imported pages stay read-only. Changes go through the application repository and its CI. Viki presents the published evidence; it does not execute application code or deploy the application.

The generic version-2 contract supports application-independent publishers and retains compatibility with the existing Vozokany version-1 publisher. The current parser accepts a deliberately limited English Gherkin subset; outlines, backgrounds, tables, and other unsupported constructs are rejected. See [repository synchronization](docs/repository-sync.md) for configuration, validation, and deployment boundaries.

### Optional assistant and Slovak presentation

- **Q&A** retrieves workspace content and answers with revision citations.
- **Edit** prepares linked drafts in a human-governed workspace. It cannot approve them and is unavailable for Git-managed workspaces.
- **Slovak presentation** can translate imported English revisions in the background. English remains canonical, and translations are tied to the exact source revision. Reading a translation makes no model call.

The core wiki works without a model-provider key. Assistant requests and background translation require a configured provider and incur its usage charges. Translation validation checks structure and protected tokens; it does not prove that the translated meaning is correct.

## Viki describing itself

The [self-documenting catalogue](specs/viki/README.md) explains Viki using its own vocabulary: concepts, features, and scenarios. It covers specification revisions, review rules, repository authority, assistant permissions, and translations, with links to the implementation and existing tests.

**[Open the public demo →](https://viki.matej-7d2.workers.dev/)**

The public demo uses Viki's reader to present this catalogue without signing in. Visitors can search the documentation and ask questions with source citations. Each browser tab keeps its own conversation; history clears on reload and is not stored on the server.

To fit Cloudflare's free runtime, the demo uses a small Workers adapter rather than the full Go/PostgreSQL/Hermes stack. It bundles the checked-in catalogue and uses gpt-oss-20b through Workers AI. D1 stores usage counters only. The limits are five questions per visitor and twenty model attempts globally per UTC day, including at most one retry per question. Each attempt has a 15-second timeout. Reaching a limit pauses Q&A while browsing and search remain available. No NVIDIA hosted trial endpoint is used.

The catalogue's Gherkin is explanatory: it has no Cucumber step bindings or eligible CI publication and is not imported execution evidence. See [public demo deployment](public-demo/README.md) for the architecture, free-tier boundaries, local checks, and release commands.

## Project status

Viki is an early-stage, single-organization pilot built with React, Go, PostgreSQL, and Hermes. The Git-managed reader has been used with Vozokany, including automatic CI-artifact imports and Slovak presentation. The generic publication contract still needs validation with a second application.

The local stack is available below, and [the Umbrel configuration](deploy/umbrel/compose.yaml) supports the existing private installation. The full wiki still uses a fixed bootstrap account; anonymous access is available through the separate public-demo adapter. Git write-back and autonomous software delivery remain deferred. The optional developer worker is experimental; its bundled target only returns a receipt.

> [!NOTE]
> The default Docker Compose stack binds to `127.0.0.1`. It is a local setup, not a public hosting configuration. The fixed bootstrap account is another current pilot limitation.

## Quick start

### Prerequisites

- Docker Engine with Docker Compose
- An OpenAI API key only if you want to enable the assistant

Clone the repository and create a local environment file:

```sh
git clone https://github.com/matk0/viki.git
cd viki
cp .env.example .env
```

Set unique values for these required local credentials in `.env`:

- `INITIAL_USER_PASSWORD`
- `HERMES_QA_TOKEN`
- `HERMES_EDIT_TOKEN`
- `VIKI_HERMES_TOOL_TOKEN`
- `DEVELOPMENT_TARGET_TOKEN`

Generate each token independently, for example with `openssl rand -hex 32`. To enable the assistant, also set `OPENAI_API_KEY`; `OPENAI_BASE_URL` and `HERMES_MODEL` are available for a compatible endpoint and model choice.

Start the complete local stack:

```sh
docker compose up --build
```

For a detached process, use:

```sh
make compose-up
```

Open [http://localhost:8080](http://localhost:8080). The current pilot bootstraps one fixed local account:

```text
Email:    matej@matejlukasik.com
Password: the value of INITIAL_USER_PASSWORD
```

The password is applied again whenever the server starts. The fixed bootstrap identity is a current single-organization pilot limitation, not a general multi-user onboarding flow.

The experimental developer worker is disabled by default. It is the only component that can send a model-authored implementation to the configured development target. Enable it only after setting a separate high-entropy `VIKI_DEVELOPER_TOOL_TOKEN` and reviewing the target boundary:

```dotenv
VIKI_DEVELOPER_ENABLED=true
VIKI_DEVELOPER_TOOL_TOKEN=replace-with-a-separate-random-token
```

Developer claims are bound to the Hermes turn that acquired them, and the mock target requires `DEVELOPMENT_TARGET_TOKEN`. The bundled mock target returns a receipt only; it never deploys code. A real target must authenticate requests and should provide its own idempotency and audit controls.

`VIKI_HOST_BIND` and `VIKI_HOST_PORT` default to `127.0.0.1` and `8080`. Change the bind address only when a protected reverse proxy or private network boundary is in place.

Check that the application and database are ready:

```sh
curl -fsS http://localhost:8080/readyz
docker compose ps
```

Stop the stack while preserving local PostgreSQL and Hermes data:

```sh
docker compose down
```

To remove all local application data, including the PostgreSQL and Hermes volumes, use the destructive command below:

```sh
docker compose down -v
```

## Local development

The Docker Compose path above is the supported full-stack setup. For focused source development, install the host dependencies first:

- Go 1.26 or newer
- Node.js 22 or newer
- Python 3, for the Hermes test suite

```sh
(cd frontend && npm ci)
(cd public-demo && npm ci)
python3 -m pip install -r hermes/requirements-test.txt
```

To run the Go API and Vite frontend directly, start only PostgreSQL in Docker, point the API at its mapped port, and run the development target:

```sh
docker compose up -d database
export DATABASE_URL='postgres://viki:viki@127.0.0.1:5433/viki?sslmode=disable'
export INITIAL_USER_PASSWORD='choose-a-local-password'
make dev
```

Open [http://localhost:5173](http://localhost:5173). This source-development path runs the core wiki; it does not configure the optional assistant profile.

## Architecture

| Area | Responsibility |
| --- | --- |
| `frontend/` | React and Vite interface for the wiki, review flow, search, and assistant drawer. |
| `backend/` | Go HTTP API, authentication, governance rules, migrations, and revision lifecycle. |
| `backend/internal/postgres/` | PostgreSQL and pgvector persistence for pages, revisions, search, discussions, and audit events. |
| `hermes/` | Separate Q&A, Edit, and opt-in Developer profiles plus authenticated, profile-scoped Viki tools. |
| `openapi/` | API contract; generated backend and frontend clients are derived from it. |
| `mock-target/` | Local development handoff stub. It returns a receipt; it does not deploy code. |

The Compose stack runs Viki, PostgreSQL, Hermes, and the mock development target. A development handoff is experimental and disabled unless explicitly configured; the included target is intentionally a mock rather than an autonomous delivery system.

## Test and verify

Run the standard unit suites:

```sh
make test
```

Run PostgreSQL integration tests:

```sh
make test-integration
```

Run the strict coverage gate used by this repository:

```sh
make test-coverage
```

`make test-integration` and `make test-coverage` recreate the `viki_test` database. They do not touch the main `viki` database, but they are destructive to that test database. The coverage gate enforces 100% coverage for Go, frontend, and the Hermes plugin, and also runs the public-demo API tests. Reports go to the ignored `coverage/` directory. Public-demo model calls are mocked in automated tests.

The Compose file declares a fixed project name, so a separate Git worktree alone does not isolate its containers or volumes. For isolated checks, set a unique project name, for example `COMPOSE_PROJECT_NAME=viki-checks make test-coverage`. The test targets expect PostgreSQL on `127.0.0.1:5433`; that port must be available to the selected test stack.

Run browser tests against a running application (default: `http://127.0.0.1:8080`):

```sh
make test-e2e
```

Set `VIKI_BASE_URL` to test another reachable instance. Playwright Chromium must already be installed.

When changing the OpenAPI contract, refresh generated code with:

```sh
make generate
```

## Contributing

Contributions are welcome. Keep each change focused, add or update tests for behavioural changes, and run `make test-coverage` before opening a pull request. If you change `openapi/openapi.yaml`, include its generated backend and frontend outputs in the same change.

Please never commit `.env`, API keys, session tokens, or other credentials.

## License

Copyright (c) 2026 Matej Lukasik.

Viki source code is released under the [MIT License](LICENSE).
