package services_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/services"
)

func TestSliceSpecForRoadmap_SmallSpecUnchanged(t *testing.T) {
	spec := "# Short Spec\nThis is under 10000 chars.\n"
	sliced := services.SliceSpecForRoadmap(spec)
	assert.Equal(t, spec, sliced)
}

func TestSliceSpecForRoadmap_LargeSpecSlicing(t *testing.T) {
	// Build a spec > 10,000 chars with an exhaustive test matrix and a large code block
	var sb strings.Builder
	sb.WriteString("# Project Specification\n\n")
	sb.WriteString("## 1. Overview\n")
	sb.WriteString(strings.Repeat("Core architecture and domain details.\n", 200)) // ~7.6KB

	sb.WriteString("\n## 2. Exhaustive Conformance Testset Matrix\n")
	sb.WriteString("### 2.1 Unit Test Details\n")
	sb.WriteString(strings.Repeat("- Assert command X behaves like Y with exact return codes.\n", 150)) // ~8KB
	sb.WriteString("### 2.2 Integration Test Catalog\n")
	sb.WriteString(strings.Repeat("- Test client pipeline concurrency under high load.\n", 150))

	sb.WriteString("\n## 3. Deployment & Execution\n")
	sb.WriteString("```yaml\n")
	for i := 0; i < 100; i++ {
		sb.WriteString("  line_of_yaml_config: value\n")
	}
	sb.WriteString("```\n")

	sb.WriteString("\n## 4. Definition of Done\n")
	sb.WriteString("All quality gates passed.\n")

	rawSpec := sb.String()
	require.Greater(t, len(rawSpec), 10000)

	sliced := services.SliceSpecForRoadmap(rawSpec)

	// Ensure the result is significantly smaller than the raw spec
	assert.Less(t, len(sliced), len(rawSpec))

	// Ensure exhaustive test details are omitted
	assert.NotContains(t, sliced, "Assert command X behaves like Y")
	assert.NotContains(t, sliced, "Test client pipeline concurrency")

	// Ensure the header and note are present
	assert.Contains(t, sliced, "## 2. Exhaustive Conformance Testset Matrix")
	assert.Contains(t, sliced, "Exhaustive per-test matrices and assertions are preserved in SPEC.md")

	// Ensure subsequent sections are retained
	assert.Contains(t, sliced, "## 3. Deployment & Execution")
	assert.Contains(t, sliced, "## 4. Definition of Done")
	assert.Contains(t, sliced, "All quality gates passed.")

	// Ensure large code block was truncated
	assert.Contains(t, sliced, "# ... [listing truncated for roadmap decomposition] ...")
}
