# Build flags
BINARY_NAME=jobpilot
CLI_BINARY_NAME=jobpilot-cli

# Optional override for the migrate-* targets' database file. Left empty by
# default so cmd/migrate falls back to system.DefaultDBPath() — the exact
# same path cmd/jobpilot-cli itself opens — so the two can never point at
# different databases. Only set this to target a scratch/test database
# instead of the real one, e.g. make migrate-up DB_PATH=/tmp/test.db.
DB_PATH ?=

.PHONY: all build clean run fmt fmt-check vet vet-integration lint lint-integration \
	tidy test test-integration check \
	migrate-up migrate-down migrate-create migrate-status

all: build

build:
	go build -o bin/$(BINARY_NAME) ./cmd/jobpilot
	go build -o bin/$(CLI_BINARY_NAME) ./cmd/jobpilot-cli

run:
	go build -o bin/$(BINARY_NAME) ./cmd/jobpilot
	go run ./cmd/jobpilot

clean:
	rm -rf bin/

# migrate-* wrap cmd/migrate, a thin dev-only wrapper around the same goose
# library calls the app itself uses (sqlite.DB.InitializeSchema and friends).
# Deliberately not the `goose` CLI: its main package unconditionally imports
# every driver it supports (Postgres, MySQL, ClickHouse, Azure SQL, ...)
# unless excluded via build tags, and those tags can't be persisted in
# go.mod — every `go mod tidy` would silently re-add them. This project is
# SQLite-only (see docs/09_decisions/0003-sqlite.md); Postgres and friends
# are explicitly out of scope per AGENTS.md.
migrate-up:
	go run ./cmd/migrate -db "$(DB_PATH)" up

migrate-down:
	go run ./cmd/migrate -db "$(DB_PATH)" down

migrate-status:
	go run ./cmd/migrate -db "$(DB_PATH)" status

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=migration_name"; exit 1; fi
	go run ./cmd/migrate create $(NAME)

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Not gofmt'd:"; gofmt -l .; exit 1)

vet:
	go vet ./...

vet-integration:
	go vet -tags=integration ./...

lint:
	golangci-lint run ./...

# Integration-tagged test files (see docs/08_development/testing.md) are
# invisible to golangci-lint without this flag.
lint-integration:
	golangci-lint run --build-tags=integration ./...

tidy:
	go mod tidy

# Fast unit tests only (internal/core/services, plus pure-logic files like
# sqlite/utils.go and llm/presets.go). No real DB or network I/O.
test:
	go test -v ./...

# Integration tests: SQLite repositories against a real temp-file database,
# LLM provider adapters against a fake HTTP server. See
# docs/08_development/testing.md.
test-integration:
	go test -race -tags=integration ./...

# Formatting/linting/static checks + tests passing, per docs/08_development/definition_of_done.md.
check: fmt-check vet vet-integration lint lint-integration test test-integration
