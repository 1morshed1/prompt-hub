# PromptHub developer commands. Run `make help` for the list.
#
# Most targets assume a local Postgres from scripts/dev_db.sh. Override the URL
# with DATABASE_URL=... on the command line if you run your own.

DATABASE_URL ?= postgres://postgres@localhost:5433/prompthub?sslmode=disable
TEST_DATABASE_URL ?= postgres://postgres@localhost:5433/prompthub_test?sslmode=disable

.PHONY: help db-start db-stop run build test test-go test-sdk seed fmt vet docker-up docker-down

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

db-start: ## Start the local throwaway Postgres (port 5433)
	./scripts/dev_db.sh start

db-stop: ## Stop the local Postgres
	./scripts/dev_db.sh stop

run: ## Run the server (applies migrations, serves API + dashboard on :8080)
	cd server && DATABASE_URL="$(DATABASE_URL)" WEB_DIR=../web go run ./cmd/server

build: ## Build the server binary to server/bin/prompthub
	cd server && go build -o bin/prompthub ./cmd/server

test: test-go test-sdk ## Run all tests (Go + SDK)

test-go: ## Run Go tests (integration tests need TEST_DATABASE_URL)
	cd server && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test ./...

test-sdk: ## Run the Python SDK tests
	cd sdk/python && PYTHONPATH=. python3 -m pytest tests/ -q

seed: ## Seed demo data + drive an A/B experiment (server must be running)
	python3 scripts/seed.py

fmt: ## Format Go code
	cd server && gofmt -w .

vet: ## Vet Go code
	cd server && go vet ./...

docker-up: ## Run everything via docker compose (Postgres + server)
	docker compose up --build

docker-down: ## Stop and remove docker compose services
	docker compose down
