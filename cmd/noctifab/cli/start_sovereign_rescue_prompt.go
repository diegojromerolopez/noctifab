package cli

import (
	"fmt"
	"strings"
)

func buildSovereignRescuePrompt(specContent string, failedStories, acceptanceGaps []string, failureLog string, turn, maxTurns int, resolvedStrategy string, detectedMissing bool) string {
	var sb strings.Builder
	sb.WriteString("You are Noctifab's Sovereign Omni-Agent.\n")
	sb.WriteString("The standard multi-agent pipeline encountered an unresolvable bottleneck and could not finish.\n")
	sb.WriteString("All role restrictions, story divisions, and architectural boundaries are DISSOLVED.\n")
	sb.WriteString("You have sovereign, direct authority to inspect, create, and modify ANY file in the workspace.\n\n")

	sb.WriteString("=== GROUND TRUTH SPECIFICATION (SPEC.md) ===\n")
	sb.WriteString(specContent)
	sb.WriteString("\n\n")

	if len(acceptanceGaps) > 0 {
		sb.WriteString("=== WHOLE-PROJECT ACCEPTANCE AUDIT GAPS (REQUIRED REMEDIATION) ===\n")
		sb.WriteString("The project failed the Whole-Project Acceptance Audit with the following concrete specification gaps.\n")
		sb.WriteString("You MUST implement all missing files, commands, schemas, and test scenarios enumerated below:\n")
		for _, gap := range acceptanceGaps {
			sb.WriteString("- ")
			sb.WriteString(gap)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(failedStories) > 0 {
		sb.WriteString("=== UNRESOLVED / FAILED ROADMAP STORIES ===\n")
		for _, s := range failedStories {
			sb.WriteString("- ")
			sb.WriteString(s)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "=== CURRENT FAILURE DIAGNOSTICS (Turn %d of %d) ===\n", turn, maxTurns)
	sb.WriteString(failureLog)
	sb.WriteString("\n\n")

	if directive := BuildToolchainFallbackDirective(resolvedStrategy, detectedMissing); directive != "" {
		sb.WriteString(directive)
	}

	sb.WriteString("=== DIRECT MANDATE ===\n")
	sb.WriteString("1. Directly write or modify all source code, headers, and configuration files needed to fulfill SPEC.md.\n")
	sb.WriteString("2. Author genuine unit tests under tests/ directory with real assertions (0 tests or empty tests will FAIL).\n")
	sb.WriteString("3. Ensure the project contains a Makefile with three standard recipes:\n")
	sb.WriteString("   - build: compiles all source binaries cleanly without errors.\n")
	sb.WriteString("   - test: executes unit tests and exits with code 1 if 0 tests are found.\n")
	sb.WriteString("   - e2e: executes end-to-end black-box verification of compiled binaries (e2e is strictly validated; if e2e fails or is missing, the turn is rejected).\n")
	sb.WriteString("4. Fix all compilation errors, missing translation units, or syntax issues reported above.\n")
	sb.WriteString("5. Strictly avoid placeholder stubs, error-masking shell tricks, and tautological tests (e.g., 'TODO: implement', 'pass', empty main functions, '|| true', 'assert True'). The anti-stub validator and non-tautological test auditor strictly reject them.\n")
	sb.WriteString("6. DIAGNOSTIC PROBE SCAFFOLDING & SCRIPT-FIRST DEBUGGING: When investigating service timeouts, port/socket connections (e.g. Redis on 6379, HTTP on 8080), daemon liveness, or mysterious test failures, NEVER guess or speculate. Author an executable diagnostic probe script or test (e.g. in tests/) or invoke check_socket / check_http / validate_manifest. Execute the probe deterministically, observe empirical OS error details (ECONNREFUSED, ETIMEDOUT, exit codes), and run a programmatic debug loop until the code operates correctly.\n")
	sb.WriteString("7. STRUCTURED DATA & SCRIPT-FIRST REFLEX: For any task dealing with structured data (JSON, YAML, TOML, CSV, schemas, ASTs, SQL seed/migrations, tabular data, or graph relationships), ASK YOURSELF if writing and executing an automated script (e.g. in Python or Go) is better, faster, and more reliable than manual string editing. Use scripts to parse, transform, validate, and verify structured data programmatically.\n")
	sb.WriteString("8. You are an autonomous headless dark-factory repair engine. You DO NOT interact with a human user and do not require chat-based interactive tools. The host orchestrator reads your JSON response and executes all file write/edit and diagnostic actions directly onto the workspace filesystem. You MUST provide all source code, tests, Dockerfiles, and configurations directly inside the 'actions' array using 'write_file', 'write_files', 'edit_file', 'check_socket', 'check_http', 'validate_manifest', or 'install_package'. Do NOT emit apologies, refusals, or claims that you lack workspace write tools.\n\n")

	sb.WriteString("=== AVAILABLE TOOLS ===\n")
	sb.WriteString("- write_file: create or overwrite a file. Args: {\"path\": \"relative/path\", \"content\": \"...\"}\n")
	sb.WriteString("- write_files: atomically create or overwrite multiple files. Args: {\"files\": [{\"path\": \"relative/path\", \"content\": \"...\"}]}\n")
	sb.WriteString("- edit_file: modify an existing file. Args: {\"path\": \"relative/path\", \"target_content\": \"exact code block to replace\", \"replacement_content\": \"new code block\"}\n")
	sb.WriteString("- check_socket: test TCP/UDP connection and optional payload ping/pong response. Args: {\"host\": \"127.0.0.1\", \"port\": 6379, \"timeout_seconds\": 2}\n")
	sb.WriteString("- check_http: test HTTP endpoint status, headers, and body response. Args: {\"url\": \"http://127.0.0.1:8080/health\", \"method\": \"GET\", \"expected_status\": 200}\n")
	sb.WriteString("- validate_manifest: validate package manifest dependencies (Cargo.toml, pyproject.toml, go.mod, package.json). Args: {\"manifest_path\": \"Cargo.toml\"}\n")
	sb.WriteString("- install_package: install project dependencies via the configured package manager. Args: {\"package\": \"name\"}\n\n")

	sb.WriteString("=== REQUIRED RESPONSE FORMAT ===\n")
	sb.WriteString("You MUST respond ONLY with a single JSON object matching this schema (do NOT wrap in markdown code fences):\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"reasoning\": \"Explanation of fixes or diagnostic probes applied to unblock the project\",\n")
	sb.WriteString("  \"actions\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"tool\": \"check_socket\",\n")
	sb.WriteString("      \"args\": {\n")
	sb.WriteString("        \"host\": \"127.0.0.1\",\n")
	sb.WriteString("        \"port\": 6379,\n")
	sb.WriteString("        \"timeout_seconds\": 2\n")
	sb.WriteString("      }\n")
	sb.WriteString("    },\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"tool\": \"write_file\",\n")
	sb.WriteString("      \"args\": {\n")
	sb.WriteString("        \"path\": \"src/main.py\",\n")
	sb.WriteString("        \"content\": \"...\"\n")
	sb.WriteString("      }\n")
	sb.WriteString("    },\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"tool\": \"write_files\",\n")
	sb.WriteString("      \"args\": {\n")
	sb.WriteString("        \"files\": [\n")
	sb.WriteString("          {\"path\": \"tests/test_app.py\", \"content\": \"...\"},\n")
	sb.WriteString("          {\"path\": \"Makefile\", \"content\": \"...\"}\n")
	sb.WriteString("        ]\n")
	sb.WriteString("      }\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")

	return sb.String()
}
