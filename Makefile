# Build flags
BINARY_NAME=job-agent
CLI_BINARY_NAME=job-agent-cli

.PHONY: all build clean run fmt fmt-check vet lint tidy test check

all: build

build:
	go build -o bin/$(BINARY_NAME) ./cmd/job-agent
	go build -o bin/$(CLI_BINARY_NAME) ./cmd/job-agent-cli

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
