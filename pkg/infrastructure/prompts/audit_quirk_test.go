package prompts

import (
	"strings"
	"testing"
)

// TestProductManagerAuditPrefixQuirk pins a deliberate legacy quirk of the
// product_manager templates that the byte-identical golden tests only cover
// implicitly (via the legacy replica in golden_legacy_test.go, which is a
// transitional artifact and may eventually be deleted).
//
// The quirk: in the legacy prefix-dispatch pipeline
// (pkg/infrastructure/llm/prompt_templates.go, deleted in v0.28.0),
// preprocessPrompt TRIMMED the "Generate detailed user stories from
// specification:" prefix before injecting the remainder into the role body's
// INPUT CONTEXT slot — but for audit prompts it kept the WHOLE raw prompt,
// so the audit instruction line ("Audit and refine existing user stories...")
// plus its "Specification:" / "Existing User Stories:" scaffolding ended up
// INSIDE the INPUT CONTEXT section.
//
// The embedded defaults preserve this asymmetry on purpose (byte-identical
// extraction). Do NOT "clean it up" by removing the instruction line from
// audit.tmpl or adding one to generate.tmpl: that would change the prompt
// bytes sent to the model and break golden byte-identity with the legacy
// assembly. If a future revision deliberately re-designs these prompts, this
// test (and the golden files) must be updated in the same change, with the
// behavior change called out in the CHANGELOG.
func TestProductManagerAuditPrefixQuirk(t *testing.T) {
	const auditInstruction = "Audit and refine existing user stories to ensure complete Definition of Done (DoD), edge cases, and interface contracts:"

	t.Run("when reading the audit default template the audit instruction is retained inside INPUT CONTEXT", func(t *testing.T) {
		text, err := DefaultTemplate(AgentProductManager, "audit")
		if err != nil {
			t.Fatal(err)
		}
		idxCtx := strings.Index(text, "INPUT CONTEXT:")
		idxInstr := strings.Index(text, auditInstruction)
		idxMandate := strings.Index(text, "REFINEMENT & AUDIT MANDATE:")
		if idxCtx < 0 || idxInstr < 0 || idxMandate < 0 {
			t.Fatal("audit template missing expected sections")
		}
		if idxCtx >= idxInstr || idxInstr >= idxMandate {
			t.Error("audit instruction must sit inside the INPUT CONTEXT section (legacy no-trim quirk)")
		}
		if !strings.Contains(text, "INPUT CONTEXT:\n"+auditInstruction) {
			t.Error("audit instruction must immediately follow the INPUT CONTEXT: header, exactly as the legacy untrimmed prompt did")
		}
		for _, scaffold := range []string{"Specification:\n{{.Spec}}", "Existing User Stories:\n{{.ExistingStories}}{{.LegacyFiles}}"} {
			if !strings.Contains(text, scaffold) {
				t.Errorf("audit template missing legacy scaffolding %q", scaffold)
			}
		}
	})

	t.Run("when reading the generate default template no instruction line leaks into INPUT CONTEXT", func(t *testing.T) {
		text, err := DefaultTemplate(AgentProductManager, "generate")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, auditInstruction) {
			t.Error("generate template must not contain the audit instruction")
		}
		if strings.Contains(text, "Generate detailed user stories from specification:") {
			t.Error("generate template must not retain its legacy dispatch prefix (it was trimmed by the legacy pipeline)")
		}
		// The trimmed prefix left "\n\n" behind, so INPUT CONTEXT is followed
		// by two blank lines and then the spec placeholder directly.
		if !strings.Contains(text, "INPUT CONTEXT:\n\n\n{{.Spec}}{{.LegacyFiles}}") {
			t.Error("generate template must place the spec directly in INPUT CONTEXT (with the legacy double blank line from prefix trimming)")
		}
	})

	t.Run("when rendering both actions the quirk shows up only in the audit prompt", func(t *testing.T) {
		r := NewDefaultRenderer()
		data := ProductManagerPromptData{Spec: "SPEC-BODY", ExistingStories: "STORY-BODY"}

		auditOut, err := r.Render(AgentProductManager, "audit", data)
		if err != nil {
			t.Fatal(err)
		}
		generateOut, err := r.Render(AgentProductManager, "generate", data)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(auditOut.Body, "INPUT CONTEXT:\n"+auditInstruction) {
			t.Error("rendered audit prompt must retain the instruction inside INPUT CONTEXT")
		}
		if !strings.Contains(auditOut.Body, "Existing User Stories:\nSTORY-BODY") {
			t.Error("rendered audit prompt must include the existing stories scaffolding")
		}
		if strings.Contains(generateOut.Body, auditInstruction) {
			t.Error("rendered generate prompt must not contain the audit instruction")
		}
		if !strings.Contains(generateOut.Body, "INPUT CONTEXT:\n\n\nSPEC-BODY") {
			t.Error("rendered generate prompt must place the spec directly after INPUT CONTEXT with the legacy double blank line")
		}
	})
}
