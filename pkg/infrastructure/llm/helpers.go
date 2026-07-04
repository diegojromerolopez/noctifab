package llm

import (
	"fmt"
	"os"
	"strings"
)

var modelHierarchy = map[string][]string{
	"gemini": {
		"gemini-3.5-flash",
		"gemini-3.1-pro-preview",
		"gemini-3.1-flash-lite",
		"gemini-3-pro-preview",
		"gemini-3-flash-preview",
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
	},
	"openai": {
		"gpt-4o",
		"gpt-4o-mini",
	},
	"mistral": {
		"mistral-large-latest",
		"mistral-medium-latest",
		"mistral-small-latest",
		"open-mistral-7b",
	},
	"deepseek": {
		"deepseek-coder",
		"deepseek-chat",
	},
	"hermes": {
		"hermes-3-llama-3.1-405b",
		"hermes-3-llama-3.1-70b",
		"hermes-3-llama-3.1-8b",
	},
	"anthropic": {
		"claude-3-5-sonnet-latest",
		"claude-3-5-haiku-latest",
	},
	"opencode": {
		"glm-5.2",
		"glm-5.1",
		"kimi-k2.7-code",
		"kimi-k2.6",
		"qwen3.7-max",
		"qwen3.7-plus",
		"minimax-m3",
		"minimax-m2.7",
		"qwen3.6-plus",
		"mimo-v2.5-pro",
		"deepseek-v4-pro",
		"mimo-v2.5",
		"deepseek-v4-flash",
	},
}

func normalizeModel(model string) string {
	trimmed := strings.TrimPrefix(strings.ToLower(model), "models/")
	if trimmed == "" {
		return "gemini-2.5-flash"
	}
	return trimmed
}

func resolveGeminiURL(modelInput, apiKey string) string {
	normModel := normalizeModel(modelInput)
	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", normModel, apiKey)
}

type projectContext struct {
	PackageName            string
	Instructions           string
	TestInstructions       string
	TestWriterInstructions string
	TestTasksInstructions1 string
	TestTasksInstructions2 string
	CliInstructions        string
	ExampleTargetFile      string
}

func getProjectContext() projectContext {
	specBytes, err := os.ReadFile("SPEC.md")
	if err != nil {
		return projectContext{
			PackageName:       "app",
			ExampleTargetFile: "src/main.rs",
		}
	}

	specStr := string(specBytes)
	specLower := strings.ToLower(specStr)

	switch {
	case strings.Contains(specLower, "rust") || strings.Contains(specLower, "cargo") || strings.Contains(specLower, "unsafe_code"):
		return rustContext(specStr)
	case strings.Contains(specLower, "todo") && (strings.Contains(specLower, "cli") || strings.Contains(specLower, "todo.py")):
		return todoCliContext()
	default:
		return pythonPackageContext(specStr)
	}
}

func rustContext(spec string) projectContext {
	return projectContext{
		PackageName:            "rwc",
		Instructions:           "2. All source files MUST be created inside the 'src/' directory following Domain-Driven Design layers: 'src/domain/' for business logic, 'src/application/' for use cases, and 'src/infrastructure/' for I/O. Add '#![deny(unsafe_code)]' to the crate root. Use streaming I/O with BufReader for multi-gigabyte file support.\n13. CRITICAL: The use of `unsafe` blocks is STRICTLY FORBIDDEN anywhere in the codebase, in both source and test files. The crate declares `#![deny(unsafe_code)]`, so any `unsafe { ... }` block is a hard compile error. Never emit `unsafe`, `std::slice::from_raw_parts`, or any raw-pointer API; use safe Rust idioms (slices, iterators, BufReader, `str::from_utf8`) instead. Violations are blocked at write time and will fail the build.",
		TestInstructions:       "3. All tests must use the standard test harness with '#[cfg(test)]' modules. Unit tests go inside each source file, integration tests go in 'tests/' directory. Use 'cargo test' to run the full suite.",
		TestWriterInstructions: "2. Source files live under 'src/' with DDD layering.\n14. CRITICAL: Never use `unsafe` blocks in test files. The crate is `#![deny(unsafe_code)]`; any `unsafe { ... }` in tests fails the build. Use safe Rust alternatives (e.g. `std::io::Cursor` instead of raw buffers).",
		TestTasksInstructions1: "6. CRITICAL: When writing integration tests, use the standard 'tests/' directory. Tests should invoke the binary as a subprocess using 'std::process::Command' to validate CLI output formatting and correctness.",
		TestTasksInstructions2: "",
		CliInstructions:        "",
		ExampleTargetFile:      "src/main.rs",
	}
}

func todoCliContext() projectContext {
	return projectContext{
		PackageName: "todo_app",
		Instructions: "2. The package name is 'todo_app'. All implementation files MUST be created or modified inside the 'todo_app/' directory (e.g., 'todo_app/tasks.py', 'todo_app/storage.py'). The main CLI entry point file is 'todo.py' at the root of the repository. Do NOT create files in 'src/' or 'frontpunch/'.\n" +
			"11. CRITICAL: When opening files (reading or writing), always explicitly specify `encoding='utf-8'` (e.g. `open(path, 'w', encoding='utf-8')`) to ensure compatibility with strict unit test assertions.\n" +
			"12. CRITICAL: When loading tasks or reading JSON, use a try...except FileNotFoundError block and check if the read content is empty rather than calling os.path.exists or os.path.getsize. This prevents test errors where only one of exists/getsize is mocked.\n" +
			"13. CRITICAL: When modifying lists returned by storage utilities (like load_tasks), always copy the list first (e.g. `tasks = list(load_tasks(path))`) before appending or mutating, as mutating the returned list directly causes python unittest mock assertions on call arguments to fail.",
		TestInstructions: "3. All unit/integration tests must be placed in the 'tests/' directory (e.g., 'tests/unit/test_tasks.py', 'tests/e2e/test_todo_e2e.py') and import from 'todo_app' and 'todo' (if testing the CLI). Do not import from 'factory' or 'frontpunch'.",
		TestWriterInstructions: "2. The package name is 'todo_app'. Implementation files exist inside the 'todo_app/' directory and 'todo.py' exists at the root.\n" +
			"11. CRITICAL: When writing mock assertions on file operations, ensure you assert with `encoding='utf-8'` or match the production code pattern. When mocking file existence or sizes, mock both `os.path.exists` and `os.path.getsize` to prevent FileNotFoundError on real disks.",
		TestTasksInstructions1: "6. CRITICAL: When writing integration tests, define any test helper functions inside a 'todo_app' module (e.g. 'todo_app/test_helpers.py') so they are imported consistently.",
		TestTasksInstructions2: "10. CRITICAL: When writing or editing test helper modules like 'todo_app/test_helpers.py', you MUST preserve all existing functions, variables, and comments that may be used by tests from other tasks.",
		CliInstructions:        "8. CRITICAL: When implementing the CLI parser in 'todo.py', make sure to support the subcommands ('add', 'list', 'done', 'rm') and arguments precisely as specified in the SPEC.md.",
		ExampleTargetFile:      "todo.py",
	}
}

func pythonPackageContext(spec string) projectContext {
	packageName := "app"
	if strings.Contains(spec, "frontpunch") {
		packageName = "frontpunch"
	}

	return projectContext{
		PackageName:            packageName,
		Instructions:           fmt.Sprintf("2. The package name is '%s'. All implementation files MUST be created or modified inside the '%s/' directory (e.g., '%s/worker.py', '%s/cli.py', '%s/client.py'). Do NOT create a directory named 'factory' or edit files in 'src/'.", packageName, packageName, packageName, packageName, packageName),
		TestInstructions:       fmt.Sprintf("3. All unit/integration tests must be placed in the 'tests/' directory (e.g., 'tests/unit/test_%s.py', 'tests/unit/test_client.py') and import from '%s'. Do not import from 'factory'.", packageName, packageName),
		TestWriterInstructions: fmt.Sprintf("2. The package name is '%s'. All implementation files exist inside the '%s/' directory.", packageName, packageName),
		TestTasksInstructions1: fmt.Sprintf("6. CRITICAL: When writing integration tests that execute worker tasks, do NOT define the task function inside the test file or under the 'tests/' directory, as this causes Python double-import issues where the test and worker threads use different module namespaces. Instead, define any test task functions inside a '%s' module (e.g. '%s/test_tasks.py') so they are imported consistently.", packageName, packageName),
		TestTasksInstructions2: fmt.Sprintf("10. CRITICAL: When writing or editing test helper modules like '%s/test_tasks.py', you MUST preserve all existing functions, variables, and comments that may be used by tests from other tasks, unless specifically instructed to delete them.", packageName),
		CliInstructions:        fmt.Sprintf("8. CRITICAL: When connecting a CLI to the worker logic in '%s/cli.py', wrap the worker call in a try...except block. This ensures graceful handling when external services are not available.", packageName),
		ExampleTargetFile:      fmt.Sprintf("%s/example.py", packageName),
	}
}
