# Khan's Bike Zone API — developer tasks.
#
# Release builds disable cgo: the artifact is a single cross-compiled Windows
# .exe and no dependency may pull in cgo. Tests are the exception: the race
# detector needs cgo (and a C compiler), and test binaries never ship.

GO      ?= go
PKG     := ./...
BINDIR  := bin

# Local settings: `cp .env.example .env` and every target below sees them.
# make parses the file, so write a literal $ as $$ and a literal # as \#.
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build every binary for this machine (cgo disabled)
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BINDIR)/ ./cmd/...

.PHONY: web
web: ## Build the React front end into web/dist, which the Go binaries embed
	cd frontend && npm ci && npm run build

.PHONY: web-dev
web-dev: ## Run the front-end dev server (proxies /api and /media to make run)
	cd frontend && npm run dev

.PHONY: build-windows
build-windows: ## Cross-compile the Windows .exe files into bin/windows
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags="-s -w" -o $(BINDIR)/windows/ ./cmd/...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKG)

.PHONY: test
test: ## Run the unit tests (no database) with the race detector
	CGO_ENABLED=1 $(GO) test -race $(PKG)

.PHONY: test-integration
test-integration: ## Unit + integration tests against Postgres (TEST_DATABASE_URL or DATABASE_URL)
	CGO_ENABLED=1 $(GO) test -race -tags=integration $(PKG)

.PHONY: lint
lint: ## Run golangci-lint v2 (must be installed)
	golangci-lint run

.PHONY: tidy
tidy: ## Sync go.mod/go.sum
	$(GO) mod tidy

.PHONY: sqlc
sqlc: ## Regenerate typed queries from db/queries into internal/store/gen
	sqlc generate

.PHONY: sqlc-check
sqlc-check: ## Fail if the generated queries are out of date
	sqlc diff

.PHONY: migrate-up
migrate-up: ## Apply all pending migrations (needs DATABASE_URL)
	$(GO) run ./cmd/migrate up

.PHONY: migrate-down
migrate-down: ## Roll back the most recent migration
	$(GO) run ./cmd/migrate down

.PHONY: migrate-status
migrate-status: ## Show migration status
	$(GO) run ./cmd/migrate status

.PHONY: seed
seed: ## Load the sample catalogue into an empty, migrated database
	$(GO) run scripts/seed.go

.PHONY: admin
admin: ## Create the owner account: make admin EMAIL=you@example.com NAME="Your Name"
	$(GO) run ./cmd/admin create-user -email "$(EMAIL)" -name "$(NAME)"

.PHONY: db-up
db-up: ## Start the dev Postgres container
	docker compose up -d postgres

.PHONY: db-down
db-down: ## Stop the dev Postgres container
	docker compose down

.PHONY: run
run: ## Run the API server (with the in-process job worker)
	$(GO) run ./cmd/api

.PHONY: worker
worker: ## Run the standalone job worker
	$(GO) run ./cmd/worker

.PHONY: check
check: vet test ## Vet + test; the pre-commit gate
