# Build flags
BINARY_NAME=job-agent
CLI_BINARY_NAME=job-agent-cli

.PHONY: all build clean run

all: build

build:
	go build -o bin/$(BINARY_NAME) ./cmd/job-agent
	go build -o bin/$(CLI_BINARY_NAME) ./cmd/job-agent-cli

run:
	go run ./cmd/job-agent

clean:
	rm -rf bin/
