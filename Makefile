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

# What `make dist` builds the daemon and CLI for (ADR-0029 §3), and the
# version it builds them as: a release's, without the tag's "v". Every other
# build says "dev" (internal/version).
DIST_TARGETS=darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64
VERSION ?=
DIST_LDFLAGS=-s -w -X github.com/unicdev/jobpilot/internal/version.Version=$(VERSION)

# The Rust targets a release builds the desktop for (release.yml): a
# universal macOS app is both Apple targets. The third-party notices cover
# the crates every one of them links.
RUST_TARGETS=aarch64-apple-darwin x86_64-apple-darwin x86_64-pc-windows-msvc x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu

# What the third-party notices are generated from, and into, which
# `notices-check` fails on when they differ from what is committed.
NOTICES_FILES=THIRD-PARTY-NOTICES.txt desktop/bundled-packages.json internal/tools/gennotices/curated

# The Rust target `make bundle` builds the desktop for, which also names the
# sidecars Tauri looks for: desktop/src-tauri/binaries/jobpilot-<target>.
# This machine's by default; universal-apple-darwin is a release's macOS.
TARGET ?= $(shell rustc -vV 2>/dev/null | sed -n 's/^host: //p')
SIDECAR_DIR=desktop/src-tauri/binaries
SIDECAR_LDFLAGS=-s -w -X github.com/unicdev/jobpilot/internal/version.Version=$(or $(VERSION),dev)

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

.PHONY: all build web build-web dist sidecars bundle set-version version-check notices notices-check clean run run-desktop dev air fmt fmt-check vet vet-integration lint lint-integration lint-fix tools desktop-deps desktop-check check-go \
	tidy test test-integration check starter-list \
	migrate-up migrate-down migrate-create migrate-status

all: build

build:
	go build -o bin/$(BINARY_NAME) ./cmd/jobpilot
	go build -o bin/$(CLI_BINARY_NAME) ./cmd/jobpilot-cli

# The interface for a web browser (ADR-0035 §1) is the desktop's frontend,
# compiled into the daemon by a build with the webui tag. web builds it and
# copies it to where that build looks; without it, `go build` and every
# check need no Bun, and the daemon's address says how to get it.
WEBUI_DIST=internal/adapters/primary/webui/dist

web: desktop-deps
	cd desktop && bun run build
	rm -rf $(WEBUI_DIST) && cp -R desktop/dist $(WEBUI_DIST)

# build-web is build, with the interface for a browser in the daemon, which
# `jobpilot-cli ui` opens.
build-web: web
	go build -tags webui -o bin/$(BINARY_NAME) ./cmd/jobpilot
	go build -o bin/$(CLI_BINARY_NAME) ./cmd/jobpilot-cli

# dist builds the daemon and CLI as a release, for every target, into
# dist/<os>-<arch>/, and packs each target's pair as the standalone archive
# a machine without the desktop installs: a .tar.gz, or a .zip for Windows.
# Each archive carries LICENSE and NOTICE, as Apache-2.0 §4 asks of a copy,
# and the third-party notices (make notices).
# They are pure Go, so one machine cross-compiles every target (ADR-0029 §3).
# The daemon carries the interface for a browser (ADR-0035 §1).
dist: web
	@case "$(VERSION)" in \
		"") echo "make dist needs a version, as in: make dist VERSION=1.2.3" >&2; exit 1;; \
		v*) echo "VERSION is the tag without its v: $(patsubst v%,%,$(VERSION))" >&2; exit 1;; \
	esac
	rm -rf dist
	@for target in $(DIST_TARGETS); do \
		goos=$${target%/*}; goarch=$${target#*/}; ext=""; \
		if [ "$$goos" = windows ]; then ext=.exe; fi; \
		out=dist/$$goos-$$goarch; \
		echo "building $(VERSION) for $$goos/$$goarch"; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$goarch go build -trimpath -tags webui -ldflags "$(DIST_LDFLAGS)" \
			-o $$out/$(BINARY_NAME)$$ext ./cmd/jobpilot || exit 1; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$goarch go build -trimpath -ldflags "$(DIST_LDFLAGS)" \
			-o $$out/$(CLI_BINARY_NAME)$$ext ./cmd/jobpilot-cli || exit 1; \
		cp LICENSE NOTICE THIRD-PARTY-NOTICES.txt $$out/ || exit 1; \
		archive=jobpilot-cli_$(VERSION)_$${goos}_$${goarch}; \
		if [ "$$goos" = windows ]; then \
			(cd $$out && zip -q ../$$archive.zip $(BINARY_NAME)$$ext $(CLI_BINARY_NAME)$$ext LICENSE NOTICE THIRD-PARTY-NOTICES.txt) || exit 1; \
		else \
			tar -czf dist/$$archive.tar.gz -C $$out $(BINARY_NAME) $(CLI_BINARY_NAME) LICENSE NOTICE THIRD-PARTY-NOTICES.txt || exit 1; \
		fi; \
	done

# sidecars builds the daemon and CLI the desktop bundle carries next to the
# app's executable (ADR-0029 §3), named for TARGET as Tauri's externalBin
# expects. A universal macOS build carries universal binaries, made with
# lipo from both architectures; each architecture's own is staged too, since
# Tauri compiles the app once per architecture and each compile looks for
# its own. Without VERSION they say "dev". The daemon carries the interface
# for a browser, as the standalone one does.
sidecars: web
	@rm -rf $(SIDECAR_DIR) && mkdir -p $(SIDECAR_DIR)
	@build() { \
		echo "building the sidecars ($(or $(VERSION),dev)) for $$1/$$2"; \
		CGO_ENABLED=0 GOOS=$$1 GOARCH=$$2 go build -trimpath -tags webui -ldflags "$(SIDECAR_LDFLAGS)" -o $$3/$(BINARY_NAME)$$4 ./cmd/jobpilot && \
		CGO_ENABLED=0 GOOS=$$1 GOARCH=$$2 go build -trimpath -ldflags "$(SIDECAR_LDFLAGS)" -o $$3/$(CLI_BINARY_NAME)$$4 ./cmd/jobpilot-cli; \
	}; \
	place() { mv $$1/$(BINARY_NAME)$$2 $(SIDECAR_DIR)/$(BINARY_NAME)-$(TARGET)$$2 && \
		mv $$1/$(CLI_BINARY_NAME)$$2 $(SIDECAR_DIR)/$(CLI_BINARY_NAME)-$(TARGET)$$2; }; \
	tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	case "$(TARGET)" in \
		aarch64-apple-darwin) build darwin arm64 $$tmp "" && place $$tmp "";; \
		x86_64-apple-darwin) build darwin amd64 $$tmp "" && place $$tmp "";; \
		universal-apple-darwin) \
			build darwin arm64 $$tmp/arm64 "" && build darwin amd64 $$tmp/amd64 "" && \
			for bin in $(BINARY_NAME) $(CLI_BINARY_NAME); do \
				lipo -create -output $(SIDECAR_DIR)/$$bin-$(TARGET) $$tmp/arm64/$$bin $$tmp/amd64/$$bin && \
				cp $$tmp/arm64/$$bin $(SIDECAR_DIR)/$$bin-aarch64-apple-darwin && \
				cp $$tmp/amd64/$$bin $(SIDECAR_DIR)/$$bin-x86_64-apple-darwin || exit 1; \
			done;; \
		x86_64-unknown-linux-gnu) build linux amd64 $$tmp "" && place $$tmp "";; \
		aarch64-unknown-linux-gnu) build linux arm64 $$tmp "" && place $$tmp "";; \
		x86_64-pc-windows-msvc) build windows amd64 $$tmp .exe && place $$tmp .exe;; \
		*) echo "no sidecars for TARGET=$(TARGET)" >&2; exit 1;; \
	esac

# The files a release's version is written in (ADR-0029 §2). Cargo.lock
# carries the desktop crate's version too, and every --locked build fails
# once it disagrees with Cargo.toml.
VERSION_FILES=desktop/src-tauri/tauri.conf.json desktop/src-tauri/Cargo.toml desktop/package.json desktop/src-tauri/Cargo.lock

# set-version writes VERSION into every file a release reads it from.
set-version:
	@case "$(VERSION)" in ""|v*) echo "set-version needs a version without the v, as in: make set-version VERSION=1.2.3" >&2; exit 1;; esac
	perl -pi -e 's/^(  "version": ")[^"]*/$${1}$(VERSION)/' desktop/src-tauri/tauri.conf.json desktop/package.json
	perl -pi -e '$$done++ if !$$done && s/^version = "[^"]*"/version = "$(VERSION)"/' desktop/src-tauri/Cargo.toml
	perl -0pi -e 's/(name = "jobpilot-desktop"\nversion = ")[^"]*/$${1}$(VERSION)/' desktop/src-tauri/Cargo.lock
	@$(MAKE) --no-print-directory version-check VERSION=$(VERSION)

# version-check fails unless every file a release reads its version from
# says VERSION, which the release workflow checks the tag against.
version-check:
	@case "$(VERSION)" in ""|v*) echo "version-check needs a version without the v, as in: make version-check VERSION=1.2.3" >&2; exit 1;; esac
	@bad=0; \
	for file in $(VERSION_FILES); do \
		case "$$file" in \
			*.json) found=$$(perl -ne 'print "$$1\n" if /^  "version": "([^"]*)"/' $$file);; \
			*Cargo.toml) found=$$(perl -ne 'if (/^version = "([^"]*)"/) { print "$$1\n"; exit }' $$file);; \
			*Cargo.lock) found=$$(perl -0ne 'print "$$1\n" if /name = "jobpilot-desktop"\nversion = "([^"]*)"/' $$file);; \
		esac; \
		if [ "$$found" != "$(VERSION)" ]; then echo "$$file says $${found:-nothing}, not $(VERSION)" >&2; bad=1; fi; \
	done; \
	[ $$bad = 0 ] && echo "every version file says $(VERSION)"

# notices writes THIRD-PARTY-NOTICES.txt, the license of every third-party
# component a release compiles in, which every archive and bundle carries
# (internal/tools/gennotices). It builds the desktop's frontend first, whose
# build records which npm packages it bundled. REFRESH_RUST_STD=1 also takes
# Rust's standard library's notices again, after the toolchain changes.
notices:
	cd desktop && bun run build
	go run ./internal/tools/gennotices -go-targets "$(DIST_TARGETS)" -rust-targets "$(RUST_TARGETS)" $(if $(REFRESH_RUST_STD),-refresh-rust-std)

# notices-check fails when the committed notices are not what notices
# writes: a dependency was added, removed or upgraded without them.
notices-check: notices
	@if [ -n "$$(git status --porcelain -- $(NOTICES_FILES))" ]; then \
		git status --short -- $(NOTICES_FILES); \
		echo "the third-party notices are out of date: run make notices and commit what it writes" >&2; \
		exit 1; \
	fi

# bundle builds the desktop's installers for TARGET, carrying the sidecars:
# a .app and .dmg on macOS, the NSIS installer on Windows, .deb and .rpm on
# Linux. The sidecars are named in tauri.bundle.conf.json rather than
# tauri.conf.json, since Tauri refuses to build without them, which every
# development build and check would then need first.
#
# BUNDLE_FLAGS is passed on to `tauri build`, as the release workflow does
# with a --config that signs on Windows once there is a signing command.
bundle: sidecars
	cd desktop && bun tauri build --target $(TARGET) --config src-tauri/tauri.bundle.conf.json $(BUNDLE_FLAGS)

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
	rm -rf bin/ $(WEBUI_DIST)

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

fmt: desktop-deps
	gofmt -w .
	cd desktop && bun run format

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

lint-fix: tools desktop-deps
	./$(GOLANGCI_LINT) run --fix ./...
	cd desktop && bun run fix

# desktop-deps installs the desktop's packages when they are missing, so the
# desktop targets run the Biome that bun.lock pins rather than failing, or
# fetching whichever version is newest.
desktop-deps:
	@test -d desktop/node_modules || (cd desktop && bun install --frozen-lockfile)

# The desktop's TypeScript, CSS and JSON, formatted, linted and with imports
# ordered as desktop/biome.json says. It writes nothing; `make lint-fix`
# applies what can be fixed.
desktop-check: desktop-deps
	cd desktop && bun run check

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
check: check-go desktop-check

# check-go is check without the desktop, for CI's Go job, which has no Bun;
# CI's desktop job runs Biome itself.
check-go: fmt-check vet vet-integration lint lint-integration test test-integration
