# Makefile for noctifab project

BINARY_NAME=noctifab
DIST_DIR=dist
GOFLAGS=-v

.PHONY: all build clean test lint help validate validate-all validate-images

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

# Run containerized E2E tests
test-e2e:
	docker compose -f tests/e2e/docker-compose.yml up --build --exit-code-from test-runner

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
	@echo "  test-e2e        - Run the containerized E2E test suite"
	@echo "  lint            - Run static analysis lint checks using Docker"
	@echo "  validate        - Run autonomous E2E check for one or more projects inside Docker (e.g. PROJECT=pyedis,echo)"
	@echo "  validate-all    - Run all validation projects in parallel inside Docker"
	@echo "  validate-images - Build base + all per-project validation Docker images"

# Which project validate-runs by default when no PROJECT is passed.
PROJECT ?= frontpunch

comma := ,
empty :=
space := $(empty) $(empty)

# Validate one or more projects (comma or space separated). Uses validation/run_one.sh
# which builds the per-project image (or reuses it), launches the container,
# captures the log to output/log/<project>.log, and writes <PROJECT>_FEEDBACK.md.
validate:
	@if [ "$(INTERACTIVE)" = "1" ]; then \
		INTERACTIVE_FLAG="-i"; \
	else \
		INTERACTIVE_FLAG=""; \
	fi; \
	for proj in $(subst $(comma),$(space),$(PROJECT)); do \
		if [ "$(SKIP_BUILD)" = "1" ]; then \
			NOCTIFAB_SKIP_BUILD=1 ./validation/run_one.sh $$INTERACTIVE_FLAG "$$proj" || exit 1; \
		else \
			./validation/run_one.sh $$INTERACTIVE_FLAG "$$proj" || exit 1; \
		fi; \
	done

# Validate every project in validation/projects/ in parallel. Each project
# runs in its own container, writes its own <PROJECT>_FEEDBACK.md, and exits
# non-zero on failure; the aggregate target exits non-zero if any project
# failed. Pass SKIP_BUILD=1 to reuse existing noctifab-validation:* images.
validate-all:
	@if [ "$(SKIP_BUILD)" = "1" ]; then \
		./validation/run_all.sh --skip-build; \
	else \
		./validation/run_all.sh; \
	fi

# Build base + all per-project validation Docker images
validate-images:
	@docker build -f validation/Dockerfile.validation -t noctifab-validation:base .
	@for proj in $$(ls validation/projects); do \
		docker build -f validation/projects/$$proj/Dockerfile -t "noctifab-validation:$$proj" . ; \
	done

