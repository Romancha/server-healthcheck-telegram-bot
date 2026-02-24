.PHONY: build test clean all help lint fmt tidy tools test-coverage test-race

# Variables
BINARY_NAME=server-healthcheck
GOOS?=linux
GOARCH?=amd64

# Go tools path
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

GOLANGCI_LINT=$(GOBIN)/golangci-lint

# Default target
all: lint test build

# Help
help:
	@echo "Usage:"
	@echo "  make build          - Build the binary"
	@echo "  make build-local    - Build for local development"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-race      - Run tests with race detection"
	@echo "  make lint           - Run linter"
	@echo "  make fmt            - Format code"
	@echo "  make tidy           - Tidy modules"
	@echo "  make tools          - Install dev tools"
	@echo "  make clean          - Clean build artifacts"

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags='-w -s' -o $(BINARY_NAME) .

# Build for local development
build-local:
	@echo "Building $(BINARY_NAME) for local use..."
	go build -o $(BINARY_NAME) .

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	go test -race -timeout=60s -count 1 ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	go clean

# Lint
lint:
	@echo "Running linter..."
	$(GOLANGCI_LINT) run ./...

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Tidy modules
tidy:
	@echo "Tidying modules..."
	go mod tidy

# Install dev tools
tools:
	@echo "Installing dev tools..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2
