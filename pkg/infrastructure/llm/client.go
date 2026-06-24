package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
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
}

var _ domain.LLMClient = (*Client)(nil)

func NewClient(provider, model, apiKey string, maxRetries int, backoff time.Duration) *Client {
	return &Client{
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		MaxRetries: maxRetries,
		Backoff:    backoff,
	}
}

func (c *Client) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	// Preprocess prompt to inject system instructions and schemas based on the target action type
	if strings.HasPrefix(prompt, "Decompose specification into tasks:") {
		specStr := strings.TrimPrefix(prompt, "Decompose specification into tasks:")
		prompt = fmt.Sprintf(`You are a software factory automation agent operating in a restricted workspace sandbox.
You must respond ONLY with a single JSON block. Do not include conversational markdown text or code fences (like `+"`"+`json or `+"`"+`) outside the JSON.

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
        "depends_on": []
      }
    }
  ]
}
`, specStr)
	} else if strings.HasPrefix(prompt, "Execute task:") {
		taskDetails := strings.TrimPrefix(prompt, "Execute task:")
		prompt = fmt.Sprintf(`You are a software factory automation agent operating in a restricted workspace sandbox.
You must respond ONLY with a single JSON block. Do not include conversational markdown text or code fences (like `+"`"+`json or `+"`"+`) outside the JSON.

You are acting as the Generator Agent.
Your task is to implement the specified task.

Task Details:
%s

CRITICAL:
1. You only have ONE single turn to complete this task. You must write/edit files and run tests immediately in your response actions. Do NOT run read_file, find_files, grep_search, or list_directory first, as you will not get another turn.
2. The package name is 'frontpunch'. All implementation files MUST be created or modified inside the 'frontpunch/' directory (e.g., 'frontpunch/worker.py', 'frontpunch/cli.py', 'frontpunch/client.py'). Do NOT create a directory named 'factory' or edit files in 'src/'.
3. All unit/integration tests must be placed in the 'tests/' directory (e.g., 'tests/unit/test_worker.py', 'tests/unit/test_client.py') and import from 'frontpunch'. Do not import from 'factory'.
4. For all Python test files, use the standard library 'unittest' and 'unittest.mock'. Do NOT import or use 'pytest' under any circumstance, as it is not installed in the sandbox environment.

You may use the following tools:
- read_file: read the contents of a file. Args: {"path": "relative/path/to/file"}
- write_file: create a new file or overwrite an existing one. Args: {"path": "relative/path/to/file", "content": "file content"}
- edit_file: modify an existing file. Args: {"path": "relative/path/to/file", "target_content": "exact code block to replace", "replacement_content": "new code block"}
- list_directory: list directory contents. Args: {"path": "relative/path/to/dir"}
- find_files: search for files. Args: {"pattern": "*.py"}
- grep_search: search for a pattern in files. Args: {"query": "search_term"}
- run_tests: run the project's tests to verify correctness. Args: {}
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
`, taskDetails)
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

	var responseBody []byte
	var err error

	maxRetries := c.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 10
	}
	backoff := c.Backoff
	if backoff <= 0 {
		backoff = 100 * time.Millisecond
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		responseBody, err = c.doPost(ctx, apiKey, prompt)
		if err == nil {
			break
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("LLM completion failed after %d retries: %w", maxRetries, err)
		}

		// Exponential backoff with jitter
		jitter := time.Duration(float64(backoff) * (1.0 + rand.Float64()))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(jitter):
		}
		backoff *= 2
	}

	extracted, err := ExtractJSONBlock(string(responseBody))
	if err != nil {
		return nil, err
	}

	return LenientUnmarshal(extracted)
}

func (c *Client) doPost(ctx context.Context, apiKey, prompt string) ([]byte, error) {
	var url string
	var reqBody []byte
	headers := make(map[string]string)

	switch c.Provider {
	case "openai":
		url = "https://api.openai.com/v1/chat/completions"
		headers["Authorization"] = "Bearer " + apiKey
		headers["Content-Type"] = "application/json"
		payload := map[string]any{
			"model": c.Model,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
			"temperature": 0.0,
		}
		reqBody, _ = json.Marshal(payload)

	case "gemini":
		url = resolveGeminiURL(c.Model, apiKey)
		headers["Content-Type"] = "application/json"
		payload := map[string]any{
			"contents": []map[string]any{
				{
					"parts": []map[string]string{
						{"text": prompt},
					},
				},
			},
			"generationConfig": map[string]any{
				"temperature": 0.0,
			},
		}
		reqBody, _ = json.Marshal(payload)

	case "anthropic":
		url = "https://api.anthropic.com/v1/messages"
		headers["X-API-Key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
		headers["Content-Type"] = "application/json"
		payload := map[string]any{
			"model": c.Model,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
			"max_tokens":  4096,
			"temperature": 0.0,
		}
		reqBody, _ = json.Marshal(payload)

	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", c.Provider)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse actual content out of the provider envelope
	switch c.Provider {
	case "openai":
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, err
		}
		choices, ok := result["choices"].([]any)
		if !ok || len(choices) == 0 {
			return nil, fmt.Errorf("unexpected OpenAI response: %s", string(respBody))
		}
		choice := choices[0].(map[string]any)
		msg := choice["message"].(map[string]any)
		content, _ := msg["content"].(string)
		return []byte(content), nil

	case "gemini":
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, err
		}
		candidates, ok := result["candidates"].([]any)
		if !ok || len(candidates) == 0 {
			return nil, fmt.Errorf("unexpected Gemini response: %s", string(respBody))
		}
		candidate := candidates[0].(map[string]any)
		content := candidate["content"].(map[string]any)
		parts := content["parts"].([]any)
		part := parts[0].(map[string]any)
		text, _ := part["text"].(string)
		return []byte(text), nil

	case "anthropic":
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, err
		}
		content, ok := result["content"].([]any)
		if !ok || len(content) == 0 {
			return nil, fmt.Errorf("unexpected Anthropic response: %s", string(respBody))
		}
		item := content[0].(map[string]any)
		text, _ := item["text"].(string)
		return []byte(text), nil
	}

	return respBody, nil
}

func resolveGeminiURL(modelInput, apiKey string) string {
	model := strings.TrimPrefix(modelInput, "models/")
	if model == "" || model == "gemini-1.5-pro" {
		model = "gemini-2.5-pro"
	}
	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
}
