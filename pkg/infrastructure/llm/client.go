package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type Client struct {
	Provider   string
	Model      string
	APIKey     string
	MaxRetries int
	Backoff    time.Duration
	URL        string
}

var _ domain.LLMClient = (*Client)(nil)

func NewClient(provider, model, apiKey string, maxRetries int, backoff time.Duration, url string) *Client {
	return &Client{
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		MaxRetries: maxRetries,
		Backoff:    backoff,
		URL:        url,
	}
}

func (c *Client) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	proj := getProjectContext()

	// Preprocess prompt to inject system instructions and schemas based on the target action type
	if strings.HasPrefix(prompt, "Decompose specification into tasks:") {
		specStr := strings.TrimPrefix(prompt, "Decompose specification into tasks:")
		prompt = fmt.Sprintf(`You are a software factory automation agent operating in a restricted workspace sandbox.
You must respond ONLY with a single JSON block. Do not include conversational markdown text or code fences (like `+"`"+`json or `+"`"+`) outside the JSON. All keys and string values in the JSON MUST be enclosed in double quotes (\"); never use single quotes (') for JSON strings or keys.

You are acting as the Planner Agent.
Your task is to decompose the following specification into a Directed Acyclic Graph (DAG) of small, testable tasks.

Specification:
%s

You may only use the 'add_task' tool to define the tasks.
'add_task' tool arguments:
- title: Short, unique title for the task (string)
- description: Detailed instructions of what needs to be implemented (string)
- change_type: Type of modification (string: "FEATURE", "FIX", or "BREAKING")
- depends_on: Array of parent task titles or IDs that must complete first (array of strings)
- target_files: Array of relative file paths in the workspace that this task targets or creates (array of strings)

CRITICAL:
1. You must always specify 'target_files' for each task to inform downstream generator agents of which files they need to work on.
2. The planned tasks must include enough detail so generator agents have all the instructions they need.

Return format:
{
  "reasoning": "Detailed technical rationale explaining your next step",
  "actions": [
    {
      "tool": "add_task",
      "args": {
        "title": "Task title",
        "description": "Task description...",
        "change_type": "FEATURE",
        "depends_on": [],
        "target_files": ["%s"]
      }
    }
  ]
}
`, specStr, proj.ExampleTargetFile)
	} else if strings.HasPrefix(prompt, "Write tests for task:") || strings.HasPrefix(prompt, "Fix the tests for task:") {
		var taskDetails string
		if strings.HasPrefix(prompt, "Write tests for task:") {
			taskDetails = strings.TrimPrefix(prompt, "Write tests for task:")
		} else {
			taskDetails = strings.TrimPrefix(prompt, "Fix the tests for task:")
		}
		prompt = fmt.Sprintf(`You are a software factory automation agent operating in a restricted workspace sandbox.
You must respond ONLY with a single JSON block. Do not include conversational markdown text or code fences (like `+"`"+`json`+"`"+` or `+"`"+`) outside the JSON. All keys and string values in the JSON MUST be enclosed in double quotes (\"); never use single quotes (') for JSON strings or keys.

You are acting as the Tester Agent.
Your task is to write or fix tests that verify the implementation of the specified task.

Task Details:
%s

CRITICAL:
1. You only have ONE single turn to complete this task. You must write/edit test files immediately in your response actions. Do NOT run read_file, find_files, grep_search, or list_directory first, as you will not get another turn.
%s
3. You must write tests according to the following guidelines:
   - Happy paths MUST be verified using end-to-end (e2e) tests as much as possible. Place these under 'tests/e2e/' or similar. E2E tests guideline: do not use mocks, use mock docker services. Check the main flows.
   - Input validations and simple edge cases MUST be verified using unit tests. Place these under 'tests/unit/' or similar. Unit tests guideline: all mock calls need to be asserted, all return values need to be checked. Do not write trivial tests or mocks.
   - Complex edge-cases, internal validation flows, and multi-component interactions MUST be verified using integration tests. Place these under 'tests/integration/' or similar. Integration tests guideline: only mock the lowest level possible (I/O mainly), so for example if your Python package depends on an external library that does HTTP requests, mock that library only. The project code must be tested fully.
4. For all Python test files, use the standard library 'unittest' and 'unittest.mock'. Do NOT import or use 'pytest' under any circumstance, as it is not installed in the sandbox environment.
5. NEVER use or create the 'tests/holdout' directory under any circumstance.
%s
7. Do not pass multiprocessing Manager proxy objects (like ListProxy) inside a JSON payload in integration tests, as they are not JSON serializable. Instead, use a thread-safe Queue, write to a temp file, or mock a method that records the result.
8. When asserting mock calls in unit tests, be careful to match the actual call count. For example, if a mock has a side-effect that returns a value and then raises an exception to break a loop, it is called twice, so calling assert_called_once() will fail. Use assertEqual(mock.call_count, 2) or assert_has_calls() instead.
9. When using side-effects to break the worker's run loop in tests, do NOT raise Exception subclasses (like StopIteration or ValueError), because the worker catches all Exception types to prevent crashing. Instead, raise a BaseException type (like KeyboardInterrupt or a custom subclass of BaseException) so the exception propagates out of the loop and terminates it.
%s
11. Do NOT mock or patch the entire 'threading' module (e.g. @patch('frontpunch.worker.threading')), as this mocks threading.Event and threading.Thread, breaking internal synchronization in the worker. Instead, patch only the specific functions you need (e.g. @patch('frontpunch.worker.threading.current_thread')).
12. When asserting calls on a mock method directly (e.g. mock_signal.signal.assert_has_calls), you must use call(args) rather than call.method(args) (e.g. call(SIGINT, ...) instead of call.signal(SIGINT, ...)), because mock_signal.signal is already the method itself.
13. Do NOT use 'logging.disable(logging.CRITICAL)' in test setups if you use 'self.assertLogs' or expect logger output, as it globally disables all logging, causing 'assertLogs' to fail. Instead, mock the worker's logger or adjust the logger's level/propagate attribute individually to suppress stdout noise.
14. Do NOT import or use external mock libraries like 'fakeredis' under any circumstance, as they are not installed in the sandbox environment. Instead, mock the Redis/Valkey client instance using unittest.mock.MagicMock or use the real client object when a server is running.
15. When patching a module containing constants used in function calls (e.g. @patch('frontpunch.worker.signal')), any constants (like signal.SIGINT) inside the code under test will resolve to mock attributes. Your assertions must expect the mock attributes (e.g. mock_signal.SIGINT) rather than the real constants.
16. Do NOT write tests that send real OS signals (like os.kill(pid, SIGTERM)) or block indefinitely on threading.Event objects without setting them, as they will cause the test suite to hang and fail validation due to timeouts. Always ensure any blocking mock side-effects have a reasonable timeout or are explicitly unblocked/set during the test.
17. When testing signal registration (checking if signal.signal is called in the main thread), do NOT patch the 'threading' module at all. Since the test runs in the main thread of the test process, you can just call worker.run() directly (with an exception/mock to break the loop) and signal.signal will naturally be called, or patch only 'threading.current_thread' to return threading.main_thread() if you are simulating the main thread in other contexts.
18. When writing the integration test's mock 'brpop' side-effect, do NOT use a long 'time.sleep(N)' to simulate blocking/empty queue, as this causes the worker thread to remain blocked when the test thread tries to join it, causing timing failures or hangs. Instead, use a threading.Event object (e.g. 'unblock_event.wait(timeout=1)') and set/signal that event immediately after calling worker._handle_shutdown() in the test, so the worker is unblocked and can terminate and join promptly.
19. If any existing tests disable logging globally at the module level (e.g. using 'logging.disable(logging.CRITICAL)'), this persists and silences log assertions in other tests run in the same process. To prevent this, if you use 'assertLogs', you MUST call 'logging.disable(logging.NOTSET)' in your test's 'setUp' (and optionally restore it in 'tearDown') to ensure the logging system is active during your test.
20. Do NOT modify global state or mutate global variables in unit or integration tests. Changing shared global configuration (like globally disabling logging) causes state pollution across tests and leads to validation failures. It is better to avoid testing a specific piece of code than to rely on modifying global state.
21. When writing integration tests that connect to a Redis/Valkey server, you MUST check if the server is available in setUp() and skip the test (using self.skipTest() or unittest.skip/skipIf) if the connection fails or if the server is not running on localhost. Do NOT fail the test on connection errors, as the test environment may not have a running Redis/Valkey service.


You may use the following tools:
- read_file: read the contents of a file. Args: {"path": "relative/path/to/file"}
- write_file: create a new file or overwrite an existing one. Args: {"path": "relative/path/to/file", "content": "file content"}
- edit_file: modify an existing file. Args: {"path": "relative/path/to/file", "target_content": "exact code block to replace", "replacement_content": "new code block"}
- list_directory: list directory contents. Args: {"path": "relative/path/to/dir"}
- find_files: search for files. Args: {"pattern": "*.py"}
- grep_search: search for a pattern in files. Args: {"query": "search_term"}
- run_tests: run the project's tests to verify correctness. Args: {}
- noop: call this when the tests have been successfully written. Args: {}

Return format:
{
  "reasoning": "Detailed technical rationale explaining your next step",
  "actions": [
    {
      "tool": "tool_name",
      "args": {
         "arg_name": "value"
      }
    }
  ]
}
`, taskDetails, proj.TestWriterInstructions, proj.TestTasksInstructions1, proj.TestTasksInstructions2)
	} else if strings.HasPrefix(prompt, "Execute task:") {
		taskDetails := strings.TrimPrefix(prompt, "Execute task:")
		prompt = fmt.Sprintf(`You are a software factory automation agent operating in a restricted workspace sandbox.
You must respond ONLY with a single JSON block. Do not include conversational markdown text or code fences (like `+"`"+`json`+"`"+` or `+"`"+`) outside the JSON. All keys and string values in the JSON MUST be enclosed in double quotes (\"); never use single quotes (') for JSON strings or keys.

You are acting as the Generator Agent.
Your task is to implement the specified task. Note that the tests for this task have already been written by the Test Writer Agent. Your job is to implement the functionality to make all tests pass successfully.

Task Details:
%s

CRITICAL:
1. You only have ONE single turn to complete this task. You must write/edit files and run tests immediately in your response actions. Do NOT run read_file, find_files, grep_search, or list_directory first, as you will not get another turn.
%s
%s
4. For all Python test files, use the standard library 'unittest' and 'unittest.mock'. Do NOT import or use 'pytest' under any circumstance, as it is not installed in the sandbox environment.
5. NEVER use or create the 'tests/holdout' directory under any circumstance.
6. CRITICAL: When executing worker tasks, the task arguments (args) in the JSON payload may be provided either as a list of positional arguments (e.g. [1, 2]) or as a dictionary of keyword arguments (e.g. {"x": 1, "y": 2}). Your implementation of the job execution method MUST dynamically check the type of the arguments and unpack them correctly (using *args for lists, and **args for dicts) to satisfy all test cases.
7. CRITICAL: When implementing graceful shutdown using ThreadPoolExecutor, you MUST keep the 'with ThreadPoolExecutor(...) as executor' context manager pattern. Do NOT replace it with 'executor = ThreadPoolExecutor(...)' or 'try...finally' around the executor instantiation, as this breaks backward-compatibility with existing tests (e.g. TestWorkerRun) that mock and assert on the context manager enter/exit methods. You must write the graceful shutdown logic inside the 'with' block, wrapping the loop in a try...finally block if you want to explicitly call 'executor.shutdown(wait=True)' or log messages. Your implementation of the run() loop MUST NOT swallow or catch StopIteration, as existing unit tests raise StopIteration to terminate the run loop and expect it to propagate. If you use a try...except block, ensure StopIteration is re-raised.
%s
9. CRITICAL: Loggers must be dependency injected (either via class constructor argument or setter method). This allows tests to pass mock logging objects or custom loggers and verify log calls without relying on or modifying global logger configuration.
10. CRITICAL: When modifying a file that already exists and contains business logic, do NOT overwrite it wholesale with 'write_file'. Instead, use 'edit_file' (or 'multi_replace_file_content') to surgically merge your changes into the existing file, preserving the original structure, functions, docstrings, and behaviors.
11. CRITICAL: Before writing any code, always check if any dependencies or infrastructure configurations (such as Docker database services) are missing from the project manifests (e.g. 'pyproject.toml', 'requirements.txt', 'docker-compose.yml'). If a dependency or service is required by the SPEC, you MUST create or update these manifests first to include them.
12. CRITICAL: If a test failure is caused by a bug or incorrect expectation in the test code itself, do NOT try to adjust the implementation to match the broken tests. Instead, call the 'request_test_fix' block to explain the bug and trigger a test fix by the Tester Agent.


You may use the following tools:
- read_file: read the contents of a file. Args: {"path": "relative/path/to/file"}
- write_file: create a new file or overwrite an existing one. Args: {"path": "relative/path/to/file", "content": "file content"}
- edit_file: modify an existing file. Args: {"path": "relative/path/to/file", "target_content": "exact code block to replace", "replacement_content": "new code block"}
- list_directory: list directory contents. Args: {"path": "relative/path/to/dir"}
- find_files: search for files. Args: {"pattern": "*.py"}
- grep_search: search for a pattern in files. Args: {"query": "search_term"}
- run_tests: run the project's tests to verify correctness. Args: {}
- request_test_fix: call this tool if a test failure is caused by a bug in the test code itself (e.g. incorrect assertion, invalid mock) rather than the implementation. Args: {"feedback": "Detailed description of the bug in the test code and how to fix it."}
- noop: call this when the implementation is fully complete and all tests pass. Args: {}

Return format:
{
  "reasoning": "Detailed technical rationale explaining your next step",
  "actions": [
    {
      "tool": "tool_name",
      "args": {
         "arg_name": "value"
      }
    }
  ]
}
`, taskDetails, proj.Instructions, proj.TestInstructions, proj.CliInstructions)
	}

	apiKey := c.APIKey
	if apiKey == "" {
		switch c.Provider {
		case "gemini":
			apiKey = os.Getenv("GEMINI_API_KEY")
		case "openai":
			apiKey = os.Getenv("OPENAI_API_KEY")
		case "anthropic":
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}

	if apiKey == "" {
		return nil, errors.New("missing API key for LLM provider")
	}

	originalModel := c.Model
	defer func() {
		c.Model = originalModel
	}()

	for {
		var responseBody []byte
		var err error

		maxRetries := c.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 5
		}
		backoff := c.Backoff
		if backoff <= 0 {
			backoff = 100 * time.Millisecond
		}

		var pClient ProviderClient
		switch strings.ToLower(c.Provider) {
		case "openai", "hermes", "huggingface", "mistral", "deepseek", "ollama":
			pClient = NewOpenAIProviderClient(c.Provider)
		case "gemini":
			pClient = NewGeminiProviderClient()
		case "anthropic":
			pClient = NewAnthropicProviderClient()
		default:
			return nil, fmt.Errorf("unsupported LLM provider: %s", c.Provider)
		}

		for attempt := 0; attempt <= maxRetries; attempt++ {
			responseBody, err = pClient.Call(ctx, c.Model, apiKey, prompt, c.URL)
			if err == nil {
				break
			}

			fmt.Fprintf(os.Stderr, "⚠ LLM API error: %v (attempt %d/%d). Retrying...\n", err, attempt+1, maxRetries+1)
			if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") || strings.Contains(err.Error(), "Quota exceeded") || strings.Contains(err.Error(), "quota") {
				fmt.Fprintln(os.Stderr, "⚠ Warning: You have exceeded your LLM API quota (HTTP 429). Please check your plan and billing details.")
			}

			if attempt == maxRetries {
				break
			}

			// Exponential backoff with jitter
			jitter := time.Duration(float64(backoff) * (1.0 + rand.Float64()))
			if delay, ok := parseRetryDelay(err); ok {
				jitter = delay
				fmt.Fprintf(os.Stderr, "⚠ Rate limited. Backing off for %v as requested by the API.\n", delay)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(jitter):
			}
			backoff *= 2
		}

		if err == nil {
			extracted, err := ExtractJSONBlock(string(responseBody))
			if err != nil {
				return nil, err
			}
			return LenientUnmarshal(extracted)
		}

		isFallbackError := strings.Contains(err.Error(), "HTTP error 503") ||
			strings.Contains(err.Error(), "503 Service Unavailable") ||
			strings.Contains(err.Error(), "HTTP error 429") ||
			strings.Contains(err.Error(), "429 Too Many Requests") ||
			strings.Contains(err.Error(), "RESOURCE_EXHAUSTED")

		if isFallbackError {
			nextModel := c.getNextLowerModel(ctx, apiKey)
			if nextModel != "" {
				fmt.Fprintf(os.Stderr, "⚠ Model %s returned transient/quota error. Falling back to lower model: %s...\n", c.Model, nextModel)
				c.Model = nextModel
				continue
			}
		}

		return nil, fmt.Errorf("LLM completion failed after %d retries: %w", maxRetries, err)
	}
}

func (c *Client) getNextLowerModel(ctx context.Context, apiKey string) string {
	list, ok := modelHierarchy[strings.ToLower(c.Provider)]
	if !ok || len(list) <= 1 {
		return ""
	}

	var pClient ProviderClient
	switch strings.ToLower(c.Provider) {
	case "openai", "hermes", "huggingface", "mistral", "deepseek", "ollama":
		pClient = NewOpenAIProviderClient(c.Provider)
	case "gemini":
		pClient = NewGeminiProviderClient()
	case "anthropic":
		pClient = NewAnthropicProviderClient()
	default:
		return ""
	}

	available, err := pClient.GetAvailableModels(ctx, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Warning: failed to query available models from %s: %v. Using static fallback hierarchy.\n", c.Provider, err)
		available = list
	}

	var filteredHierarchy []string
	for _, rankedModel := range list {
		rankedNorm := strings.TrimPrefix(strings.ToLower(rankedModel), "models/")
		isAvailable := false
		for _, availModel := range available {
			availNorm := strings.TrimPrefix(strings.ToLower(availModel), "models/")
			if strings.Contains(availNorm, rankedNorm) || strings.Contains(rankedNorm, availNorm) {
				isAvailable = true
				break
			}
		}
		if isAvailable {
			filteredHierarchy = append(filteredHierarchy, rankedModel)
		}
	}

	if len(filteredHierarchy) == 0 {
		filteredHierarchy = list
	}

	normCurrent := normalizeModel(c.Model)
	normCurrent = strings.ToLower(normCurrent)

	idx := -1
	for i, m := range filteredHierarchy {
		normM := strings.TrimPrefix(m, "models/")
		normM = strings.ToLower(normM)
		if strings.Contains(normCurrent, normM) || strings.Contains(normM, normCurrent) {
			idx = i
			break
		}
	}

	if idx != -1 && idx+1 < len(filteredHierarchy) {
		return filteredHierarchy[idx+1]
	}
	return ""
}

type geminiErrorResponse struct {
	Error struct {
		Details []struct {
			Type       string `json:"@type"`
			RetryDelay string `json:"retryDelay"`
		} `json:"details"`
	} `json:"error"`
}

func parseRetryDelay(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	errStr := err.Error()
	firstBrace := strings.Index(errStr, "{")
	if firstBrace == -1 {
		return 0, false
	}
	jsonStr := errStr[firstBrace:]
	var resp geminiErrorResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return 0, false
	}
	for _, detail := range resp.Error.Details {
		if detail.RetryDelay != "" {
			delayStr := detail.RetryDelay
			if strings.HasSuffix(delayStr, "s") {
				d, err := time.ParseDuration(delayStr)
				if err == nil {
					return d, true
				}
			} else {
				var sec float64
				if _, err := fmt.Sscanf(delayStr, "%f", &sec); err == nil {
					return time.Duration(sec * float64(time.Second)), true
				}
			}
		}
	}
	return 0, false
}
