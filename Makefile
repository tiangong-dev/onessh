.DEFAULT_GOAL := help

GO ?= go
BINARY ?= onessh
CMD_PATH ?= ./cmd/onessh
BUILD_DIR ?= bin
DIST_DIR ?= dist

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: help deps fmt vet build build-release install test test-short test-e2e ci clean

help:
	@echo "Available targets:"
	@echo "  make build         Build local binary to $(BUILD_DIR)/$(BINARY)"
	@echo "  make build-release Build release-style binary to $(DIST_DIR)/$(BINARY)"
	@echo "  make test          Run all Go tests (includes e2e)"
	@echo "  make test-short    Run tests in short mode"
	@echo "  make test-e2e      Run e2e tests only"
	@echo "  make ci            Run vet + full test suite"
	@echo "  make deps          Run go mod tidy"
	@echo "  make fmt           Run go fmt"
	@echo "  make install       Install onessh to GOPATH/bin"
	@echo "  make clean         Remove build artifacts"

deps:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY) $(CMD_PATH)

build-release:
	@mkdir -p $(DIST_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY) $(CMD_PATH)

install:
	$(GO) install $(CMD_PATH)

test:
	$(GO) test ./...

test-short:
	$(GO) test ./... -short

test-e2e:
	$(GO) test ./e2e -v

ci: vet test

clean:
	rm -rf $(BUILD_DIR) $(DIST_DIR)
