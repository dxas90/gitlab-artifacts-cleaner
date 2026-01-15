# Makefile for GitLab Artifacts Cleaner

.PHONY: all build test clean install fmt lint help

# Variables
BINARY_NAME=gitlab-artifacts-cleaner
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-s -w"

all: fmt lint test build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) .

## test: Run tests
test:
	@echo "Running tests..."
	$(GO) test $(GOFLAGS) ./...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	rm -f artifact-cleaner.log

## install: Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

## lint: Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, skipping..."; \
		echo "Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

## mod: Tidy and verify dependencies
mod:
	@echo "Tidying dependencies..."
	$(GO) mod tidy
	$(GO) mod verify

## run: Run the application (requires environment variables)
run: build
	./$(BINARY_NAME)

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
