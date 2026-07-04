# Makefile for noctifab project

BINARY_NAME=noctifab
DIST_DIR=dist
GOFLAGS=-v

.PHONY: all build clean test lint help validate

# Default target
all: build

# Build the Go binary in the dist directory
build:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(DIST_DIR)/$(BINARY_NAME) cmd/noctifab/main.go

# Clean build artifacts
clean:
	rm -rf $(DIST_DIR)

# Run unit tests
test:
	CGO_ENABLED=0 go test ./...

# Run static analysis linter as specified in AGENTS.md
lint:
	docker run -t --rm -v $(shell pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run

# Display help information
help:
	@echo "Available targets:"
	@echo "  all             - Builds the binary (default target)"
	@echo "  build           - Compile noctifab binary to dist/ folder"
	@echo "  clean           - Remove build artifacts (dist/ directory)"
	@echo "  test            - Run the Go unit test suite"
	@echo "  lint            - Run static analysis lint checks using Docker"
	@echo "  validate        - Run autonomous E2E checks inside Docker (isolated)"

PROJECT ?= frontpunch

validate:
	@if [ -z "$$GEMINI_API_KEY" ]; then \
		if [ -f "validation/projects/$(PROJECT)/.noctifab/secrets.yaml" ]; then \
			GEMINI_KEY=$$(grep "GEMINI_API_KEY:" validation/projects/$(PROJECT)/.noctifab/secrets.yaml | awk -F'"' '{print $$2}'); \
		elif [ -f "validation/projects/frontpunch/.noctifab/secrets.yaml" ]; then \
			GEMINI_KEY=$$(grep "GEMINI_API_KEY:" validation/projects/frontpunch/.noctifab/secrets.yaml | awk -F'"' '{print $$2}'); \
		fi; \
	else \
		GEMINI_KEY=$$GEMINI_API_KEY; \
	fi; \
	if [ -z "$$GEMINI_KEY" ]; then \
		echo "Error: GEMINI_API_KEY is not set."; \
		exit 1; \
	fi; \
	docker build -t noctifab-verify -f validation/Dockerfile.validation .; \
	docker run --rm -e GEMINI_API_KEY="$$GEMINI_KEY" -e GITHUB_TOKEN="dummy-token" -e PROJECT="$(PROJECT)" -e MODE="$(MODE)" noctifab-verify
