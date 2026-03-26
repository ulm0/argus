.PHONY: all build web dev clean test release snapshot

BINARY_NAME := argus
GOARCH ?= arm64
GOOS ?= linux
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION) -X github.com/ulm0/argus/cmd/argus/cmd.Version=$(VERSION)

all: web build

# Build Next.js static export
web:
	@echo "Building Next.js frontend..."
	cd web && npm ci && npm run build
	@echo "Frontend build complete -> web/out/"

# Cross-compile Go binary with embedded frontend
build:
	@echo "Building Go binary for $(GOOS)/$(GOARCH)..."
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/argus
	@echo "Binary: bin/$(BINARY_NAME) ($(GOOS)/$(GOARCH))"

# Build for local development (current platform)
build-local:
	@echo "Building Go binary for local platform..."
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME)-local ./cmd/argus
	@echo "Binary: bin/$(BINARY_NAME)-local"

# Development mode: run locally with auto-reload
dev:
	@echo "Starting development server..."
	@echo "Frontend: cd web && npm run dev"
	@echo "Backend:  go run ./cmd/argus run config.yaml"

# Run tests
test:
	go test -v -race ./...

# Run tests with coverage
test-cover:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Lint
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf web/out/
	rm -rf web/.next/
	rm -f coverage.out coverage.html

# Download dependencies
deps:
	go mod download
	cd web && npm ci

# Format code
fmt:
	gofmt -s -w .
	cd web && npm run lint -- --fix 2>/dev/null || true

# Create a GitHub release using GoReleaser (requires a git tag)
release:
	goreleaser release --clean

# Build snapshot release locally without publishing (no tag required)
snapshot:
	goreleaser release --snapshot --clean
