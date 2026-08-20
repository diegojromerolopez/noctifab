package services

import (
	"bytes"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func TestSpecRenderer_Formatting(t *testing.T) {
	var buf bytes.Buffer
	renderer := NewCustomSpecRenderer(strings.NewReader(""), &buf)

	renderer.PrintHeader("Test Header")
	assert.Contains(t, buf.String(), "Test Header")

	renderer.PrintSuccess("Success message")
	assert.Contains(t, buf.String(), "✔ Success message")

	renderer.PrintInfo("Info message")
	assert.Contains(t, buf.String(), "ℹ Info message")

	renderer.PrintError("Error message")
	assert.Contains(t, buf.String(), "✖ Error message")

	renderer.PrintApprovalMessage("looks good", "Reason")
	assert.Contains(t, buf.String(), "Approval intent detected")
}

func TestSpecRenderer_Diff(t *testing.T) {
	var buf bytes.Buffer
	renderer := NewCustomSpecRenderer(strings.NewReader(""), &buf)

	oldSpec := "# Title\nSection 1\nSection 2"
	newSpec := "# Title\nSection 1\nSection 2\n+ Added Section 3"

	diff := renderer.CalculateDiff(oldSpec, newSpec)
	assert.Contains(t, diff, "+ + Added Section 3")

	renderer.RenderDiff(diff)
	assert.Contains(t, buf.String(), "Specification Changes Applied")
}

func TestSpecRenderer_InteractivePrompts(t *testing.T) {
	// Feedback prompt
	in1 := strings.NewReader("Add PostgreSQL support\n")
	var out1 bytes.Buffer
	renderer1 := NewCustomSpecRenderer(in1, &out1)
	input, err := renderer1.PromptUserFeedback(1)
	assert.NoError(t, err)
	assert.Equal(t, "Add PostgreSQL support", input)

	// Yes/No prompt - affirmative
	in2 := strings.NewReader("y\n")
	var out2 bytes.Buffer
	renderer2 := NewCustomSpecRenderer(in2, &out2)
	assert.True(t, renderer2.PromptYesNo("Generate roadmap?", false))

	// Yes/No prompt - negative
	in3 := strings.NewReader("n\n")
	var out3 bytes.Buffer
	renderer3 := NewCustomSpecRenderer(in3, &out3)
	assert.False(t, renderer3.PromptYesNo("Generate roadmap?", true))

	// Yes/No prompt - default
	in4 := strings.NewReader("\n")
	var out4 bytes.Buffer
	renderer4 := NewCustomSpecRenderer(in4, &out4)
	assert.True(t, renderer4.PromptYesNo("Generate roadmap?", true))
}

func TestSpecRenderer_HistoryAndRollback(t *testing.T) {
	var buf bytes.Buffer
	renderer := NewCustomSpecRenderer(strings.NewReader(""), &buf)

	revisions := []domain.SpecRevision{
		{Version: 1, Content: "# V1", Prompt: "Initial", Kind: domain.SpecTurnInitial},
		{Version: 2, Content: "# V2\nTLS", Prompt: "Add TLS", Kind: domain.SpecTurnRefine},
	}

	renderer.RenderHistory(revisions, 1)
	assert.Contains(t, buf.String(), "Specification Revision Timeline")
	assert.Contains(t, buf.String(), "v1:")
	assert.Contains(t, buf.String(), "➔ v2:")

	renderer.RenderRollback(1, 10)
	assert.Contains(t, buf.String(), "Rolled back to Revision 1")

	renderer.RenderCheckout(2, 20)
	assert.Contains(t, buf.String(), "Checked out Revision 2")
}
