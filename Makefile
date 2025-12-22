# kql Makefile

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X github.com/cloudygreybeard/kql/cmd.Version=$(VERSION) \
	-X github.com/cloudygreybeard/kql/cmd.GitCommit=$(COMMIT) \
	-X github.com/cloudygreybeard/kql/cmd.BuildDate=$(DATE)

.PHONY: build test lint clean release-check help

## build: Build the binary
build:
	go build -ldflags "$(LDFLAGS)" -o kql .

## test: Run tests
test:
	go test -v -race ./...

## lint: Run linter
lint:
	golangci-lint run

## clean: Remove build artifacts
clean:
	rm -f kql
	rm -rf dist/

## release-check: Validate goreleaser config
release-check:
	goreleaser check

## release-snapshot: Build a snapshot release (no publish)
release-snapshot:
	goreleaser release --snapshot --clean

## install: Install to GOPATH/bin
install:
	go install -ldflags "$(LDFLAGS)" .

## help: Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'

