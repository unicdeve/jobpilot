# Build flags
BINARY_NAME=job-agent
CLI_BINARY_NAME=job-agent-cli
DB_PATH ?= ./job-agent.db

.PHONY: all build clean run fmt fmt-check vet lint tidy test check \
	migrate-up migrate-down migrate-create migrate-status

# Path to the SQLite database file the migrate-* targets operate on (the
# running app finds this itself via system.GetDataDir(), e.g.
# ~/Library/Application Support/job-agent/job-agent.db on macOS or
# ~/.config/job-agent/job-agent.db on Linux). Not needed for migrate-create.

all: build

build:
	go build -o bin/$(BINARY_NAME) ./cmd/job-agent
	go build -o bin/$(CLI_BINARY_NAME) ./cmd/job-agent-cli

# migrate-* wrap cmd/migrate, a thin dev-only wrapper around the same goose
# library calls the app itself uses (sqlite.DB.InitializeSchema and friends).
# Deliberately not the `goose` CLI: its main package unconditionally imports
# every driver it supports (Postgres, MySQL, ClickHouse, Azure SQL, ...)
# unless excluded via build tags, and those tags can't be persisted in
# go.mod — every `go mod tidy` would silently re-add them. This project is
# SQLite-only (see docs/09_decisions/0003-sqlite.md); Postgres and friends
# are explicitly out of scope per AGENTS.md.
migrate-up:
	@if [ -z "$(DB_PATH)" ]; then echo "Usage: make migrate-up DB_PATH=/path/to/job-agent.db"; exit 1; fi
	go run ./cmd/migrate -db "$(DB_PATH)" up

migrate-down:
	@if [ -z "$(DB_PATH)" ]; then echo "Usage: make migrate-down DB_PATH=/path/to/job-agent.db"; exit 1; fi
	go run ./cmd/migrate -db "$(DB_PATH)" down

migrate-status:
	@if [ -z "$(DB_PATH)" ]; then echo "Usage: make migrate-status DB_PATH=/path/to/job-agent.db"; exit 1; fi
	go run ./cmd/migrate -db "$(DB_PATH)" status

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=migration_name"; exit 1; fi
	go run ./cmd/migrate create $(NAME)

run:
	go run ./cmd/job-agent

clean:
	rm -rf bin/

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Not gofmt'd:"; gofmt -l .; exit 1)

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

test:
	go test ./...

# Formatting/linting/static checks + tests passing, per docs/08_development/definition_of_done.md.
check: fmt-check vet lint test
