# Khan's Bike Zone API — developer tasks.
# CGO is disabled everywhere: the artifact is a single cross-compiled Windows
# .exe and no dependency may pull in cgo.
export CGO_ENABLED := 0

GO      ?= go
PKG     := ./...
BINDIR  := bin

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build all binaries with cgo disabled
	$(GO) build -o $(BINDIR)/ $(PKG)

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKG)

.PHONY: test
test: ## Run the default (no-database) test suite with the race detector
	$(GO) test -race $(PKG)

.PHONY: test-integration
test-integration: ## Run integration tests (requires a running Postgres)
	$(GO) test -race -tags=integration $(PKG)

.PHONY: lint
lint: ## Run golangci-lint (must be installed)
	golangci-lint run

.PHONY: tidy
tidy: ## Sync go.mod/go.sum
	$(GO) mod tidy

.PHONY: sqlc
sqlc: ## Regenerate typed queries from db/queries into internal/store/gen
	sqlc generate

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations (needs DATABASE_URL)
	$(GO) run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: ## Roll back the most recent migration
	$(GO) run ./cmd/migrate down

.PHONY: migrate-status
migrate-status: ## Show migration status
	$(GO) run ./cmd/migrate status

.PHONY: db-up
db-up: ## Start the dev Postgres container
	docker compose up -d postgres

.PHONY: db-down
db-down: ## Stop the dev Postgres container
	docker compose down

.PHONY: run
run: ## Run the API server
	$(GO) run ./cmd/api

.PHONY: check
check: vet test ## Vet + test; the pre-commit gate
