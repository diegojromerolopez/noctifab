package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrioritizeCandidatesByModel(t *testing.T) {
	candidates := []RouterCandidate{
		{Name: "c1", Model: "gemini-2.0-flash"},
		{Name: "c2", Model: "claude-3-5-haiku"},
		{Name: "c3", Model: "gemini-2.5-pro"},
	}

	// Target matching c3
	res := PrioritizeCandidatesByModel(candidates, "2.5-pro")
	assert.Equal(t, "gemini-2.5-pro", res[0].Model)
	assert.Equal(t, "gemini-2.0-flash", res[1].Model)
	assert.Equal(t, "claude-3-5-haiku", res[2].Model)

	// Target not matching anything leaves order untouched
	resEmpty := PrioritizeCandidatesByModel(candidates, "gpt-4o")
	assert.Equal(t, "gemini-2.0-flash", resEmpty[0].Model)
	assert.Equal(t, "claude-3-5-haiku", resEmpty[1].Model)
	assert.Equal(t, "gemini-2.5-pro", resEmpty[2].Model)
}
