# viki

> A local-first, human-governed specification wiki that turns company knowledge into linked concepts, features, and Gherkin scenarios.

Viki helps a team turn informal knowledge into a shared, reviewable specification. It keeps proposed changes as drafts instead of silently overwriting agreed requirements. Its optional Slovak-first assistant can answer questions from the workspace or prepare drafts, while people remain responsible for review and approval.

> [!WARNING]
> Viki is an early-stage, local-development project. The supplied Docker Compose stack binds the web app to `127.0.0.1` by default and refuses to start without locally supplied credentials, but it is not a public-production deployment guide.

## What Viki does

Viki organizes a company's operational knowledge into a deliberate hierarchy:

1. **Concepts** define shared domain vocabulary, as nouns and actions.
2. **Features** describe business capabilities.
3. **Scenarios** sit beneath a feature and capture concrete behaviour as Gherkin `Given` / `When` / `Then` steps.

Every change is versioned. A change starts as a draft, may be discussed or challenged, and is approved only after review. Unresolved objections block approval; a scenario also requires an approved parent feature. The UI includes search, revision history, change history, comments, objections, and reusable Gherkin step definitions.

### Assistant modes

The embedded assistant is optional and deliberately constrained:

- **Q&A** searches and explains the workspace with revision citations. It is read-only.
- **Edit** identifies relevant concepts, features, and scenarios, then creates linked draft revisions for a human to review. It cannot approve or publish them.

The assistant uses the configured OpenAI-compatible provider only when it is enabled. Without an API key, the core wiki remains usable and the assistant is unavailable.

## Quick start

### Prerequisites

- Docker Engine with Docker Compose
- An OpenAI API key only if you want to enable the assistant

Clone the repository, enter it, then create a local environment file:

```sh
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

`make test-integration` and `make test-coverage` recreate the `viki_test` database. They do not touch the main `viki` database, but they are destructive to that test database. The coverage gate enforces 100% coverage for Go, frontend, and the Hermes plugin, and writes reports to the ignored `coverage/` directory.

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
