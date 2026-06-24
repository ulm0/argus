.PHONY: all build web dev clean test release snapshot

BINARY_NAME := argus
GOARCH ?= arm64
GOOS ?= linux
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION) -X github.com/ulm0/argus/cmd/argus/cmd.Version=$(VERSION)

all: web build

# Build Next.js static export and stage it where the Go binary embeds it
# (cmd/argus/web/out), mirroring the GoReleaser before-hook.
web:
	@echo "Building Next.js frontend..."
	cd web && pnpm install --frozen-lockfile && pnpm run build
	rm -rf cmd/argus/web/out && mkdir -p cmd/argus/web && cp -r web/out cmd/argus/web/out
	@echo "Frontend build complete -> cmd/argus/web/out/"

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
	@echo "Frontend: cd web && pnpm dev"
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
	cd web && pnpm install --frozen-lockfile

# Format code
fmt:
	gofmt -s -w .
	cd web && pnpm run lint --fix 2>/dev/null || true

# Create a GitHub release using GoReleaser (requires a git tag)
release:
	goreleaser release --clean

# Build snapshot release locally without publishing (no tag required)
snapshot:
	goreleaser release --snapshot --clean
