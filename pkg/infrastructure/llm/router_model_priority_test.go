package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrioritizeCandidatesByModel(t *testing.T) {
	candidates := []RouterCandidate{
		{Name: "c1", Model: "gemini-2.0-flash"},
		{Name: "c2", Model: "claude-haiku-5-5"},
		{Name: "c3", Model: "gemini-2.5-pro"},
	}

	// Target matching c3
	res := PrioritizeCandidatesByModel(candidates, "2.5-pro")
	assert.Equal(t, "gemini-2.5-pro", res[0].Model)
	assert.Equal(t, "gemini-2.0-flash", res[1].Model)
	assert.Equal(t, "claude-haiku-5-5", res[2].Model)

	// Target not matching anything leaves order untouched
	resEmpty := PrioritizeCandidatesByModel(candidates, "gpt-4o")
	assert.Equal(t, "gemini-2.0-flash", resEmpty[0].Model)
	assert.Equal(t, "claude-haiku-5-5", resEmpty[1].Model)
	assert.Equal(t, "gemini-2.5-pro", resEmpty[2].Model)
}

func TestPrioritizeCandidatesForQualityStage(t *testing.T) {
	candidates := []RouterCandidate{
		{Name: "claude", Provider: "anthropic", Model: "claude-sonnet-5"},
		{Name: "gemini-flash", Provider: "gemini", Model: "gemini-3.8-flash"},
		{Name: "deepseek-pro", Provider: "qwencloud", Model: "deepseek-v4-pro"},
		{Name: "gemini", Provider: "gemini", Model: "gemini-3.1-pro-preview"},
	}

	// Stage 0 (Draft): Ascending quality (fast draft model first, climbing to frontier)
	stage0 := PrioritizeCandidatesForQualityStage(candidates, 0)
	assert.Equal(t, "gemini-3.8-flash", stage0[0].Model)
	assert.Equal(t, "deepseek-v4-pro", stage0[1].Model)
	assert.Equal(t, "gemini-3.1-pro-preview", stage0[2].Model)
	assert.Equal(t, "claude-sonnet-5", stage0[3].Model)

	// Stage 1 (Refinement / Improve Draft): Steps up past fast draft (score < 70) to top frontier models
	stage1 := PrioritizeCandidatesForQualityStage(candidates, 1)
	assert.Equal(t, "claude-sonnet-5", stage1[0].Model)
	assert.Equal(t, "gemini-3.1-pro-preview", stage1[1].Model)
	assert.Equal(t, "deepseek-v4-pro", stage1[2].Model)
	assert.Equal(t, "gemini-3.8-flash", stage1[3].Model)

	// Stage 2+ (Surgical Repair / Frontier Escalation): Highest quality frontier models first (descending)
	stage2 := PrioritizeCandidatesForQualityStage(candidates, 2)
	assert.Equal(t, "claude-sonnet-5", stage2[0].Model)
	assert.Equal(t, "gemini-3.1-pro-preview", stage2[1].Model)
	assert.Equal(t, "deepseek-v4-pro", stage2[2].Model)
	assert.Equal(t, "gemini-3.8-flash", stage2[3].Model)
}
