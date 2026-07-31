.PHONY: dev backend frontend test test-backend test-integration test-frontend test-hermes test-e2e generate compose-up compose-down

dev:
	@trap 'kill 0' INT TERM EXIT; \
	(cd backend && INITIAL_USER_PASSWORD=$${INITIAL_USER_PASSWORD:-password} go run ./cmd/viki) & \
	(cd frontend && npm run dev) & \
	wait

backend:
	cd backend && INITIAL_USER_PASSWORD=$${INITIAL_USER_PASSWORD:-password} go run ./cmd/viki

frontend:
	cd frontend && npm run dev

test: test-backend test-frontend test-hermes

test-backend:
	cd backend && go test ./...

test-integration:
	cd backend && VIKI_TEST_DATABASE_URL=$${VIKI_TEST_DATABASE_URL:-postgres://viki:viki@127.0.0.1:5433/viki?sslmode=disable} go test ./internal/postgres -count=1

test-frontend:
	cd frontend && npm test -- --run

test-hermes:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s hermes/tests -v

test-e2e:
	cd frontend && npm run test:e2e

generate:
	cd backend && go generate ./internal/api
	cd frontend && npm run generate:api

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down
