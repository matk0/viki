.PHONY: dev backend frontend install-git-hooks test test-guardrails test-backend test-integration test-frontend test-hermes test-e2e test-coverage test-coverage-backend test-coverage-frontend test-coverage-hermes reset-test-database generate compose-up compose-down

GO_COVERAGE_MIN ?= 100
HERMES_COVERAGE_MIN ?= 100
TEST_COMPOSE_ENV = INITIAL_USER_PASSWORD=viki-test-password HERMES_QA_TOKEN=viki-test-qa-token HERMES_EDIT_TOKEN=viki-test-edit-token VIKI_HERMES_TOOL_TOKEN=viki-test-tool-token DEVELOPMENT_TARGET_TOKEN=viki-test-target-token VIKI_DEVELOPER_ENABLED=false

dev:
	@: "$${INITIAL_USER_PASSWORD:?Set INITIAL_USER_PASSWORD before running make dev}"; \
	trap 'kill 0' INT TERM EXIT; \
	(cd backend && INITIAL_USER_PASSWORD="$$INITIAL_USER_PASSWORD" go run ./cmd/viki) & \
	(cd frontend && npm run dev) & \
	wait

backend:
	@: "$${INITIAL_USER_PASSWORD:?Set INITIAL_USER_PASSWORD before running make backend}"
	cd backend && INITIAL_USER_PASSWORD="$$INITIAL_USER_PASSWORD" go run ./cmd/viki

frontend:
	cd frontend && npm run dev

install-git-hooks:
	./scripts/install_git_hooks.sh

test: test-guardrails test-backend test-frontend test-hermes

test-guardrails:
	node --test scripts/lefthook_contract.test.mjs

test-backend:
	cd backend && go test ./...

test-integration: reset-test-database
	cd backend && VIKI_TEST_DATABASE_URL=postgres://viki:viki@127.0.0.1:5433/viki_test?sslmode=disable go test ./internal/postgres -count=1

test-frontend:
	cd frontend && npm test -- --run

test-hermes:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s hermes/tests -v

test-e2e:
	cd frontend && npm run test:e2e

test-coverage: test-guardrails test-coverage-backend test-coverage-frontend test-coverage-hermes

test-coverage-backend: reset-test-database
	mkdir -p coverage
	cd backend && VIKI_TEST_DATABASE_URL=postgres://viki:viki@127.0.0.1:5433/viki_test?sslmode=disable go test -count=1 -coverpkg=./... -covermode=atomic -coverprofile=../coverage/backend.out ./...
	@actual="$$(cd backend && go tool cover -func=../coverage/backend.out | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}')"; \
		awk -v actual="$$actual" -v minimum="$(GO_COVERAGE_MIN)" 'BEGIN { \
			if (actual + 0 < minimum + 0) { \
				printf "Go coverage %.1f%% is below the %.1f%% minimum\n", actual, minimum; \
				exit 1; \
			} \
			printf "Go coverage %.1f%% meets the %.1f%% minimum\n", actual, minimum; \
		}'
	cd backend && go tool cover -html=../coverage/backend.out -o ../coverage/backend.html

test-coverage-frontend:
	cd frontend && npm run test:coverage

test-coverage-hermes:
	@python3 -m coverage --version >/dev/null 2>&1 || { echo "Run: python3 -m pip install -r hermes/requirements-test.txt"; exit 1; }
	mkdir -p coverage
	COVERAGE_FILE=coverage/hermes.data PYTHONDONTWRITEBYTECODE=1 python3 -m coverage run --branch --source=hermes/plugins/viki -m unittest discover -s hermes/tests -q
	COVERAGE_FILE=coverage/hermes.data python3 -m coverage report --precision=2 --fail-under=$(HERMES_COVERAGE_MIN) -m
	COVERAGE_FILE=coverage/hermes.data python3 -m coverage html -d coverage/hermes

reset-test-database:
	@$(TEST_COMPOSE_ENV) docker compose up -d database
	@until $(TEST_COMPOSE_ENV) docker compose exec -T database pg_isready -U viki -d postgres >/dev/null 2>&1; do sleep 1; done
	@$(TEST_COMPOSE_ENV) docker compose exec -T database dropdb -U viki --if-exists --force viki_test
	@$(TEST_COMPOSE_ENV) docker compose exec -T database createdb -U viki viki_test

generate:
	cd backend && go generate ./internal/api
	cd frontend && npm run generate:api

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down
