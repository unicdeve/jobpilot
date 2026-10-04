# Build flags
BINARY_NAME=jobpilot
CLI_BINARY_NAME=jobpilot-cli

# golangci-lint is pinned and installed into ./bin rather than relied on from
# the machine, so every contributor and CI run lints against the same rules:
# a different version reports a different set of problems, which would make
# `make check` mean different things in different places.
#
# It is deliberately NOT a go.mod tool dependency. `go install pkg@version`
# builds it in isolation from this module, whereas a tool directive would
# pull golangci-lint's dependency tree into go.mod as indirect requires and
# let its constraints raise shared dependencies for the application itself.
# go.mod stays the application's dependency contract.
GOLANGCI_LINT_VERSION ?= v2.13.2
GOLANGCI_LINT=bin/golangci-lint
GO_VERSION=$(shell go env GOVERSION)

# The operating systems JobPilot ships for (ADR-0029), which vet-integration
# and lint-integration check the code as.
PLATFORMS=darwin linux windows

# air rebuilds and restarts the daemon on change (`make dev`). Pinned and
# installed into ./bin for the reason golangci-lint is, and a development
# tool only: nothing the application builds depends on it.
AIR_VERSION ?= v1.67.4
AIR=bin/air

# Optional override for the migrate-* targets' database file. Left empty by
# default so cmd/migrate falls back to system.DefaultDBPath() — the exact
# same path cmd/jobpilot-cli itself opens — so the two can never point at
# different databases. Only set this to target a scratch/test database
# instead of the real one, e.g. make migrate-up DB_PATH=/tmp/test.db.
DB_PATH ?=

.PHONY: all build clean run run-desktop dev air fmt fmt-check vet vet-integration lint lint-integration lint-fix tools \
	tidy test test-integration check starter-list \
	migrate-up migrate-down migrate-create migrate-status

all: build

build:
	go build -o bin/$(BINARY_NAME) ./cmd/jobpilot
	go build -o bin/$(CLI_BINARY_NAME) ./cmd/jobpilot-cli

run:
	go build -o bin/$(BINARY_NAME) ./cmd/jobpilot
	go run ./cmd/jobpilot

run-desktop:
	cd desktop && bun tauri dev

# dev runs the daemon under air, rebuilt and restarted whenever its Go or
# migrations change. Start it before the desktop or CLI, which find the
# running daemon and reconnect to it after each restart; see
# docs/08_development/hot_reload.md.
dev: air
	./$(AIR)

# Installs the pinned air when it is missing or the wrong version.
air:
	@if ! ./$(AIR) -v 2>/dev/null | grep -q "^$(AIR_VERSION),"; then \
		echo "installing air $(AIR_VERSION) into ./bin"; \
		GOBIN="$(CURDIR)/bin" go install github.com/air-verse/air@$(AIR_VERSION); \
	fi

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

# Vetted as each platform JobPilot ships for, wherever it runs: the alarm,
# keystore and process code differs per platform, and a file built only for
# Windows is otherwise never checked on a Mac. The integration tag adds files
# rather than excluding any, so this covers everything `vet` does.
vet-integration:
	@for goos in $(PLATFORMS); do \
		echo "go vet -tags=integration ./... (GOOS=$$goos)"; \
		GOOS=$$goos go vet -tags=integration ./... || exit 1; \
	done

# Installs the pinned linter when it is missing, the wrong version, or built
# against a different Go toolchain than the one now on PATH.
#
# That last condition is not fussiness. golangci-lint type-checks with the
# go/types it was compiled against, so a binary built by an older Go cannot
# parse a newer standard library: upgrading Go to 1.27 made the 1.26-built
# binary report invented errors inside crypto/internal/randutil. Rebuilding
# it is the fix, and comparing the build version here means nobody has to
# recognise that failure to get it.
#
# Deliberately a phony target rather than a file rule on $(GOLANGCI_LINT):
# make skips a file rule whenever the file exists, so a developer with a
# stale binary already on disk would keep linting with it and never be told.
# Re-checking costs one exec.
tools:
	@if ! ./$(GOLANGCI_LINT) --version 2>/dev/null | \
		grep -q "version $(patsubst v%,%,$(GOLANGCI_LINT_VERSION)) built with $(GO_VERSION) "; then \
		echo "installing golangci-lint $(GOLANGCI_LINT_VERSION) (for $(GO_VERSION)) into ./bin"; \
		GOBIN="$(CURDIR)/bin" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

lint: tools
	./$(GOLANGCI_LINT) run ./...

# Integration-tagged test files (see docs/08_development/testing.md) are
# invisible to golangci-lint without this flag. Linted as each platform, for
# the reason vet-integration is.
lint-integration: tools
	@for goos in $(PLATFORMS); do \
		echo "golangci-lint run --build-tags=integration ./... (GOOS=$$goos)"; \
		GOOS=$$goos ./$(GOLANGCI_LINT) run --build-tags=integration ./... || exit 1; \
	done

lint-fix: tools
	./$(GOLANGCI_LINT) run --fix ./...

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

# starter-list measures the boards in internal/tools/genstarter/seed.txt and
# writes the companies JobPilot suggests watching. It reads every board over
# the network, so it is run by hand before each release rather than by
# check; see docs/02_features/17_company_watchlists.md.
starter-list:
	go generate ./internal/core/domain/

# Formatting/linting/static checks + tests passing, per docs/08_development/definition_of_done.md.
check: fmt-check vet vet-integration lint lint-integration test test-integration
