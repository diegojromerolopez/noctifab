package services_test

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validStoryMarkdown() string {
	return "# Story\n\n```noctifab-contract\n" + `{
  "story_id": "US-001",
  "public_contracts": [{
    "id": "cli.invalid-input",
    "interface": "CLI ./dist/example",
    "applicable_path_prefixes": ["./cmd/", "pkg\\cli"],
    "allowed_executables": ["./dist/example"],
    "exit_codes": [2],
    "stdout_contains": [],
    "stderr_prefixes": ["ERROR:"]
  }]
}` + "\n```\n"
}

func TestParseStoryContract(t *testing.T) {
	t.Run("when one valid block is supplied it returns a normalized contract and source hash", func(t *testing.T) {
		markdown := validStoryMarkdown()
		contract, err := services.ParseStoryContract("./roadmap/US-001.md", markdown)
		require.NoError(t, err)
		require.Equal(t, "US-001", contract.StoryID)
		require.Equal(t, "roadmap/US-001.md", contract.SourcePath)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(markdown))), contract.SourceSHA256)
		require.Equal(t, []string{"cmd", "pkg/cli"}, contract.PublicContracts[0].ApplicablePathPrefixes)
		require.Equal(t, []string{"./dist/example"}, contract.PublicContracts[0].AllowedExecutables)
	})

	t.Run("when exit_codes is formatted as a map or string array it unmarshals correctly", func(t *testing.T) {
		markdown := strings.Replace(validStoryMarkdown(), `"exit_codes": [2]`, `"exit_codes": {"0": "success", "1": "error"}`, 1)
		contract, err := services.ParseStoryContract("./roadmap/US-001.md", markdown)
		require.NoError(t, err)
		require.Equal(t, domain.ExitCodeList{0, 1}, contract.PublicContracts[0].ExitCodes)
	})

	tests := []struct {
		name     string
		markdown string
		needle   string
	}{
		{"when the block is absent", "# Story", "exactly one"},
		{"when blocks are duplicated", validStoryMarkdown() + validStoryMarkdown(), "exactly one"},
		{"when JSON has an unknown field", strings.Replace(validStoryMarkdown(), `"story_id": "US-001",`, `"story_id": "US-001", "unknown": true,`, 1), "unknown field"},
		{"when contract IDs repeat", strings.Replace(validStoryMarkdown(), "]\n}", `,{"id":"cli.invalid-input","interface":"CLI","allowed_executables":["dist/example"],"exit_codes":[0]}]}`+"\n", 1), "duplicate"},
		{"when an ID is invalid", strings.Replace(validStoryMarkdown(), "cli.invalid-input", "CLI invalid", 1), "id is invalid"},
		{"when an executable is absolute", strings.Replace(validStoryMarkdown(), "./dist/example", "/bin/example", 2), "repository-relative"},
		{"when a path traverses upward", strings.Replace(validStoryMarkdown(), "./cmd/", "cmd/../secret", 1), "must not contain .."},
		{"when no expectation is declared", "", "no observable expectation"},
	}
	// Build the no-expectation case separately to keep the fixture readable.
	tests[len(tests)-1].markdown = strings.Replace(validStoryMarkdown(), `"exit_codes": [2],`, `"exit_codes": [],`, 1)
	tests[len(tests)-1].markdown = strings.Replace(tests[len(tests)-1].markdown, `"stderr_prefixes": ["ERROR:"]`, `"stderr_prefixes": []`, 1)

	for _, test := range tests {
		t.Run(test.name+" it returns a prefixed error", func(t *testing.T) {
			_, err := services.ParseStoryContract("roadmap/US-001.md", test.markdown)
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), "story contract:"), err.Error())
			require.Contains(t, err.Error(), test.needle)
		})
	}

	t.Run("when HTTP contract with embedded Bruno block is supplied it attaches BrunoBru", func(t *testing.T) {
		httpStory := `# US-002: User Authentication

` + "```noctifab-contract\n" + `{
  "story_id": "US-002",
  "public_contracts": [
    {
      "id": "post-login",
      "interface": "http",
      "http_method": "POST",
      "http_path": "/api/v1/auth/login",
      "expected_status": 200,
      "expected_response_body": "token"
    }
  ]
}
` + "```\n\n" + "```bru\n" + `meta {
  name: Login
  type: http
}
post {
  url: {{baseUrl}}/api/v1/auth/login
}
` + "```\n"

		contract, err := services.ParseStoryContract("roadmap/US-002.md", httpStory)
		require.NoError(t, err)
		assert.Equal(t, "US-002", contract.StoryID)
		require.Len(t, contract.PublicContracts, 1)
		assert.Equal(t, "POST", contract.PublicContracts[0].HTTPMethod)
		assert.Equal(t, 200, contract.PublicContracts[0].ExpectedStatus)
		assert.Contains(t, contract.PublicContracts[0].BrunoBru, "meta {")

		assert.NoError(t, services.ValidateStoryContract("roadmap/US-002.md", httpStory))
	})

	t.Run("valid protocol story with request format, frames, and examples", func(t *testing.T) {
		socketStory := "```noctifab-contract\n" + `{
  "story_id": "US-003",
  "public_contracts": [
    {
      "id": "ping_command",
      "interface": "socket",
      "request_format": "RESP2",
      "request_frames": ["*1\r\n$4\r\nPING\r\n"],
      "request_examples": ["PING"],
      "stdout_contains": ["PONG"]
    }
  ]
}
` + "```\n"

		contract, err := services.ParseStoryContract("roadmap/US-003.md", socketStory)
		require.NoError(t, err)
		assert.Equal(t, "US-003", contract.StoryID)
		require.Len(t, contract.PublicContracts, 1)
		assert.Equal(t, "RESP2", contract.PublicContracts[0].RequestFormat)
		assert.Equal(t, []string{"*1\r\n$4\r\nPING\r\n"}, contract.PublicContracts[0].RequestFrames)
		assert.Equal(t, []string{"PING"}, contract.PublicContracts[0].RequestExamples)
		assert.NoError(t, services.ValidateStoryContract("roadmap/US-003.md", socketStory))
	})
}
