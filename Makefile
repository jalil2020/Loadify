# Loadify — common tasks.
#
# Everything here works from a clean checkout with only Docker installed;
# targets that need Go are marked.

COMPOSE := docker compose -f deploy/docker-compose.yml
BACKEND := backend

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ─── stack ──────────────────────────────────────────────────────────

.PHONY: up
up: ## Start the whole stack
	$(COMPOSE) up -d
	@echo "API:    http://localhost:8080/health"
	@echo "MinIO:  http://localhost:9001"

.PHONY: down
down: ## Stop the stack, keeping data
	$(COMPOSE) down

.PHONY: reset
reset: ## Stop the stack and DELETE all data
	$(COMPOSE) down -v

.PHONY: infra
infra: ## Start only Postgres, Redis and MinIO (run the API on the host)
	$(COMPOSE) up -d postgres redis minio minio-init

.PHONY: logs
logs: ## Tail the API log
	$(COMPOSE) logs -f api

.PHONY: ps
ps: ## Show service status
	$(COMPOSE) ps

.PHONY: psql
psql: ## Open a database shell
	docker exec -it loadify-postgres psql -U loadify -d loadify

# ─── backend (needs Go) ─────────────────────────────────────────────

.PHONY: run
run: ## Run the API on the host
	cd $(BACKEND) && go run ./cmd/api

.PHONY: migrate
migrate: ## Apply migrations
	cd $(BACKEND) && go run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: ## Roll back every migration
	cd $(BACKEND) && go run ./cmd/migrate down

.PHONY: migrate-status
migrate-status: ## Show the migration state
	cd $(BACKEND) && go run ./cmd/migrate status

.PHONY: seed
seed: ## Load the demo dataset
	cd $(BACKEND) && go run ./cmd/seed

.PHONY: test
test: ## Run the test suite
	cd $(BACKEND) && go test ./...

.PHONY: test-race
test-race: ## Run tests with the race detector
	cd $(BACKEND) && go test -race ./...

.PHONY: cover
cover: ## Run tests and open the coverage report
	cd $(BACKEND) && go test -coverprofile=coverage.out ./... \
		&& go tool cover -html=coverage.out

.PHONY: lint
lint: ## Format check and vet
	cd $(BACKEND) && gofmt -l . && go vet ./...

.PHONY: fmt
fmt: ## Format the code
	cd $(BACKEND) && gofmt -w .

.PHONY: tidy
tidy: ## Tidy the module graph
	cd $(BACKEND) && go mod tidy

.PHONY: check
check: fmt lint test ## Format, lint and test — run before pushing
