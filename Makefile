# Makefile for feed-mcp development tasks
# Run 'make help' to see all available targets

.PHONY: help build test test-verbose test-race test-coverage test-coverage-html lint fmt vet fix run clean install-golangci pre-commit-install dev-setup version deps deps-update tidy check-deps security check-only

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_NAME := feed-mcp
GOLANGCI_LINT_VERSION := v2.4.0
GOPATH := $(shell go env GOPATH)
GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint

# Build information
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -X main.version=$(VERSION)

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)feed-mcp development tasks$(NC)"
	@echo ""
	@echo "$(YELLOW)Usage:$(NC) make <target>"
	@echo ""
	@echo "$(YELLOW)Available targets:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(GREEN)%-18s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## Build targets
build: ## Build all packages
	@echo "$(YELLOW)Building all packages...$(NC)"
	go build -v -ldflags "$(LDFLAGS)" ./...

build-binary: ## Build main binary
	@echo "$(YELLOW)Building $(BINARY_NAME) binary...$(NC)"
	go build -v -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

install: ## Install binary to GOPATH/bin
	@echo "$(YELLOW)Installing $(BINARY_NAME)...$(NC)"
	go install -ldflags "$(LDFLAGS)" .

## Test targets  
test: ## Run all tests (unit + BDD)
	@echo "$(YELLOW)Running all tests...$(NC)"
	go test ./...

test-verbose: ## Run tests with verbose output
	@echo "$(YELLOW)Running tests with verbose output...$(NC)"
	go test -v ./...

test-race: ## Run tests with race detector
	@echo "$(YELLOW)Running tests with race detector...$(NC)"
	go test -race ./...

test-coverage: ## Run tests with coverage
	@echo "$(YELLOW)Running tests with coverage...$(NC)"
	go test -cover ./...

test-coverage-html: ## Generate HTML coverage report
	@echo "$(YELLOW)Generating HTML coverage report...$(NC)"
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

test-specific: ## Run specific test (usage: make test-specific TEST=TestName PACKAGE=./package)
	@if [ -z "$(TEST)" ] || [ -z "$(PACKAGE)" ]; then \
		echo "$(RED)Error: Both TEST and PACKAGE variables must be set.$(NC)"; \
		echo "$(YELLOW)Usage: make test-specific TEST=TestName PACKAGE=./package$(NC)"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Running specific test...$(NC)"
	go test -run $(TEST) $(PACKAGE)

## Linting and formatting targets
fmt: ## Format code
	@echo "$(YELLOW)Formatting code...$(NC)"
	go fmt ./...

vet: ## Run go vet
	@echo "$(YELLOW)Running go vet...$(NC)"
	go vet ./...

lint: $(GOLANGCI_LINT) ## Run comprehensive linting
	@echo "$(YELLOW)Running golangci-lint...$(NC)"
	$(GOLANGCI_LINT) run

lint-fix: $(GOLANGCI_LINT) ## Run linting with auto-fix
	@echo "$(YELLOW)Running golangci-lint with auto-fix...$(NC)"
	$(GOLANGCI_LINT) run --fix

$(GOLANGCI_LINT): ## Install golangci-lint if not present
	@echo "$(YELLOW)Installing golangci-lint $(GOLANGCI_LINT_VERSION)...$(NC)"
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin $(GOLANGCI_LINT_VERSION)

install-golangci: $(GOLANGCI_LINT) ## Install golangci-lint

## Development targets
run: build-binary ## Build and run with example feeds
	@echo "$(YELLOW)Running feed-mcp with example feeds...$(NC)"
	./$(BINARY_NAME) run https://techcrunch.com/feed/ https://www.wired.com/feed/rss

run-local: build-binary ## Build and run with local test feeds
	@echo "$(YELLOW)Running feed-mcp with local test feeds (requires --allow-private-ips)...$(NC)"
	./$(BINARY_NAME) run --allow-private-ips http://localhost:8080/feed.xml

run-security: build-binary ## Run with security feeds
	@echo "$(YELLOW)Running feed-mcp with security feeds...$(NC)"
	./$(BINARY_NAME) run https://krebsonsecurity.com/feed/ https://www.schneier.com/blog/atom.xml

run-reddit: build-binary ## Run with Reddit feeds
	@echo "$(YELLOW)Running feed-mcp with Reddit feeds...$(NC)"
	./$(BINARY_NAME) run https://www.reddit.com/r/golang/.rss https://www.reddit.com/r/mcp/.rss

dev-setup: install-golangci ## Set up development environment
	@echo "$(YELLOW)Setting up development environment...$(NC)"
	go mod tidy
	go mod download
	@echo "$(GREEN)Development environment ready!$(NC)"

## Dependency management
deps: ## Download dependencies
	@echo "$(YELLOW)Downloading dependencies...$(NC)"
	go mod download

deps-update: ## Update dependencies
	@echo "$(YELLOW)Updating dependencies...$(NC)"
	go get -u ./...
	go mod tidy

tidy: ## Tidy up go.mod
	@echo "$(YELLOW)Tidying go.mod...$(NC)"
	go mod tidy

check-deps: ## Check for outdated dependencies
	@echo "$(YELLOW)Checking for outdated dependencies...$(NC)"
	go list -u -m all

## Quality and security
security: ## Run security checks
	@echo "$(YELLOW)Running security checks...$(NC)"
	@if command -v gosec > /dev/null; then \
		gosec ./...; \
	else \
		echo "$(YELLOW)gosec not installed. Install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest$(NC)"; \
	fi

check-only: vet lint test ## Run validation checks without code modification
	@echo "$(GREEN)All checks completed successfully!$(NC)"

check: fmt vet lint test ## Run all checks (format, vet, lint, test)

fix: fmt lint-fix ## Format code and fix linting issues

## Pre-commit hooks setup
pre-commit-install: ## Install pre-commit hooks (requires Python and pre-commit)
	@echo "$(YELLOW)Setting up pre-commit hooks...$(NC)"
	@if ! command -v pre-commit > /dev/null; then \
		echo "$(RED)Error: pre-commit not found. Install with: pip install pre-commit$(NC)"; \
		exit 1; \
	fi
	@echo "repos:" > .pre-commit-config.yaml
	@echo "  - repo: local" >> .pre-commit-config.yaml
	@echo "    hooks:" >> .pre-commit-config.yaml
	@echo "      - id: go-fmt" >> .pre-commit-config.yaml
	@echo "        name: go fmt" >> .pre-commit-config.yaml
	@echo "        language: system" >> .pre-commit-config.yaml
	@echo "        entry: go fmt ./..." >> .pre-commit-config.yaml
	@echo "        files: \.go$$" >> .pre-commit-config.yaml
	@echo "      - id: golangci-lint" >> .pre-commit-config.yaml  
	@echo "        name: golangci-lint" >> .pre-commit-config.yaml
	@echo "        language: system" >> .pre-commit-config.yaml
	@echo "        entry: $(GOLANGCI_LINT) run --fix" >> .pre-commit-config.yaml
	@echo "        files: \.go$$" >> .pre-commit-config.yaml
	@echo "        pass_filenames: false" >> .pre-commit-config.yaml
	pre-commit install
	@echo "$(GREEN)Pre-commit hooks installed!$(NC)"

## Cleanup targets
clean: ## Clean build artifacts and cache
	@echo "$(YELLOW)Cleaning build artifacts...$(NC)"
	go clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	rm -rf dist/

clean-all: clean ## Clean all build artifacts and cache

## Utility targets
version: ## Show version information
	@echo "$(YELLOW)Version Information:$(NC)"
	@echo "Git Version: $(VERSION)"
	@echo "Go Version: $(shell go version)"
	@if [ -f "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint: $(shell $(GOLANGCI_LINT) version 2>/dev/null | head -n1)"; \
	else \
		echo "golangci-lint: not installed"; \
	fi

env: ## Show environment information
	@echo "$(YELLOW)Environment Information:$(NC)"
	@echo "GOPATH: $(GOPATH)"
	@echo "GOROOT: $(shell go env GOROOT)"
	@echo "GOOS: $(shell go env GOOS)"
	@echo "GOARCH: $(shell go env GOARCH)"
	@echo "GO111MODULE: $(shell go env GO111MODULE)"

## CI/Development workflow targets
ci: deps check ## Run CI pipeline (deps, format, vet, lint, test)
	@echo "$(GREEN)CI pipeline completed successfully!$(NC)"

dev: deps fmt fix test ## Development workflow (deps, format, fix, test)
	@echo "$(GREEN)Development workflow completed successfully!$(NC)"

release-check: clean ci ## Check if ready for release
	@echo "$(GREEN)Release check completed successfully!$(NC)"