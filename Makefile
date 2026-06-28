# Makefile for noctifab project

BINARY_NAME=noctifab
DIST_DIR=dist
GOFLAGS=-v

.PHONY: all build clean test lint help

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
	@echo "  verify-autonomy - Run autonomous E2E checks inside Docker (isolated)"

verify-autonomy:
	@if [ -z "$$GEMINI_API_KEY" ] && [ -f "/Users/diegoj/repos/frontpunch/.noctifab/secrets.yaml" ]; then \
		GEMINI_KEY=$$(grep "GEMINI_API_KEY:" /Users/diegoj/repos/frontpunch/.noctifab/secrets.yaml | awk -F'"' '{print $$2}'); \
	else \
		GEMINI_KEY=$$GEMINI_API_KEY; \
	fi; \
	if [ -z "$$GEMINI_KEY" ]; then \
		echo "Error: GEMINI_API_KEY is not set."; \
		exit 1; \
	fi; \
	rm -rf validation/data; \
	mkdir -p validation/data/frontpunch; \
	cp -R /Users/diegoj/repos/frontpunch/roadmap validation/data/frontpunch/roadmap; \
	cp -R /Users/diegoj/repos/frontpunch/.noctifab validation/data/frontpunch/.noctifab; \
	rm -f validation/data/frontpunch/.noctifab/data/noctifab.db; \
	cd validation/data/frontpunch && git init && git checkout -b main && git config user.name "Noctifab Tester" && git config user.email "tester@noctifab.local" && git add . && git commit -m "initial frontpunch structures"; \
	docker build -t noctifab-verify -f validation/Dockerfile.validation .; \
	rm -rf validation/data; \
	docker run --rm -e GEMINI_API_KEY="$$GEMINI_KEY" -e GITHUB_TOKEN="dummy-token" noctifab-verify
