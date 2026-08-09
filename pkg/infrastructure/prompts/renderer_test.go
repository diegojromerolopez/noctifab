package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkspaceTemplate(t *testing.T, workspace, agent, action, suffix, content string) string {
	t.Helper()
	dir := filepath.Join(workspace, ".noctifab", "prompts", agent)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, action+suffix)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderer_ResolutionPrecedence(t *testing.T) {
	data := TaskPromptData{Title: "T", Description: "D"}

	t.Run("when no overrides exist it renders the embedded default", func(t *testing.T) {
		r, err := NewRenderer(t.TempDir(), nil)
		if err != nil {
			t.Fatal(err)
		}
		rendered, err := r.Render("generator", "implement", data)
		if err != nil {
			t.Fatal(err)
		}
		got := rendered.Full()
		if !strings.Contains(got, "You are acting as the Generator Agent.") {
			t.Error("expected embedded default body")
		}
		d, _ := r.Describe("generator", "implement")
		if d.Source != SourceEmbedded {
			t.Errorf("expected embedded source, got %s", d.Source)
		}
	})

	t.Run("when a convention file exists it overrides the embedded default", func(t *testing.T) {
		ws := t.TempDir()
		writeWorkspaceTemplate(t, ws, "generator", "implement", ".tmpl", "CUSTOM BODY {{.Title}}\n")
		r, err := NewRenderer(ws, nil)
		if err != nil {
			t.Fatal(err)
		}
		rendered, err := r.Render("generator", "implement", data)
		if err != nil {
			t.Fatal(err)
		}
		got := rendered.Full()
		if !strings.HasPrefix(got, "CUSTOM BODY T\n") {
			t.Errorf("expected convention override body, got: %q", got[:40])
		}
		if !strings.HasSuffix(got, Contract("generator")) {
			t.Error("expected non-overridable contract appended to override")
		}
		d, _ := r.Describe("generator", "implement")
		if d.Source != SourceConvention {
			t.Errorf("expected convention source, got %s", d.Source)
		}
	})

	t.Run("when a config path exists it wins over the convention file", func(t *testing.T) {
		ws := t.TempDir()
		writeWorkspaceTemplate(t, ws, "generator", "implement", ".tmpl", "CONVENTION BODY\n")
		cfgPath := filepath.Join(ws, "my-implement.tmpl")
		if err := os.WriteFile(cfgPath, []byte("CONFIG BODY {{.Title}}\n"), 0644); err != nil {
			t.Fatal(err)
		}
		r, err := NewRenderer(ws, map[string]map[string]Override{
			"generator": {"implement": {Path: "my-implement.tmpl"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		renderedP, _ := r.Render("generator", "implement", data)
		got := renderedP.Full()
		if !strings.HasPrefix(got, "CONFIG BODY T\n") {
			t.Errorf("expected config override body, got: %q", got[:40])
		}
		d, _ := r.Describe("generator", "implement")
		if d.Source != SourceConfig {
			t.Errorf("expected config source, got %s", d.Source)
		}
	})

	t.Run("when the config path is absolute it is used as-is", func(t *testing.T) {
		ws := t.TempDir()
		abs := filepath.Join(t.TempDir(), "abs.tmpl")
		if err := os.WriteFile(abs, []byte("ABS BODY\n"), 0644); err != nil {
			t.Fatal(err)
		}
		r, err := NewRenderer(ws, map[string]map[string]Override{
			"planner": {"decompose": {Path: abs}},
		})
		if err != nil {
			t.Fatal(err)
		}
		renderedP, _ := r.Render("planner", "decompose", PlannerPromptData{Spec: "s"})
		got := renderedP.Full()
		if !strings.HasPrefix(got, "ABS BODY\n") {
			t.Error("expected absolute-path override body")
		}
	})

	t.Run("when the config path is missing it fails fast at construction", func(t *testing.T) {
		_, err := NewRenderer(t.TempDir(), map[string]map[string]Override{
			"tester": {"write": {Path: "does-not-exist.tmpl"}},
		})
		if err == nil || !strings.Contains(err.Error(), "does-not-exist.tmpl") {
			t.Fatalf("expected file-named error, got: %v", err)
		}
	})

	t.Run("when an override has a template parse error it fails fast naming the key", func(t *testing.T) {
		ws := t.TempDir()
		writeWorkspaceTemplate(t, ws, "tester", "write", ".tmpl", "BROKEN {{.Title\n")
		_, err := NewRenderer(ws, nil)
		if err == nil || !strings.Contains(err.Error(), "tester/write") {
			t.Fatalf("expected key-named parse error, got: %v", err)
		}
	})

	t.Run("when overrides reference an unknown agent or action it fails fast", func(t *testing.T) {
		if _, err := NewRenderer(t.TempDir(), map[string]map[string]Override{
			"architect": {"design": {Append: "x"}},
		}); err == nil {
			t.Fatal("expected error for unknown agent")
		}
		if _, err := NewRenderer(t.TempDir(), map[string]map[string]Override{
			"tester": {"nonexistent": {Append: "x"}},
		}); err == nil {
			t.Fatal("expected error for unknown action")
		}
	})

	t.Run("when rendering an unknown key it returns a validation error", func(t *testing.T) {
		r := NewDefaultRenderer()
		if _, err := r.Render("qa", "review", nil); err == nil {
			t.Fatal("expected error for unknown key")
		}
		if _, err := r.Describe("qa", "review"); err == nil {
			t.Fatal("expected error for unknown key")
		}
	})
}

func TestRenderer_AppendSemantics(t *testing.T) {
	data := TaskPromptData{Title: "T", Description: "D"}

	t.Run("when a config append exists it is appended to the default body before the contract", func(t *testing.T) {
		r, err := NewRenderer(t.TempDir(), map[string]map[string]Override{
			"tester": {"write": {Append: "Prefer table-driven tests."}},
		})
		if err != nil {
			t.Fatal(err)
		}
		renderedP, _ := r.Render("tester", "write", data)
		got := renderedP.Full()
		if !strings.HasSuffix(got, "Prefer table-driven tests."+Contract("tester")) {
			t.Error("expected append between default body and contract")
		}
		d, _ := r.Describe("tester", "write")
		if d.AppendSource != "config" {
			t.Errorf("expected config append source, got %q", d.AppendSource)
		}
	})

	t.Run("when a convention append file exists it is appended to the default body", func(t *testing.T) {
		ws := t.TempDir()
		writeWorkspaceTemplate(t, ws, "tester", "write", ".append.tmpl", "EXTRA RULE\n")
		r, err := NewRenderer(ws, nil)
		if err != nil {
			t.Fatal(err)
		}
		renderedP, _ := r.Render("tester", "write", data)
		got := renderedP.Full()
		if !strings.HasSuffix(got, "EXTRA RULE\n"+Contract("tester")) {
			t.Error("expected append file content before contract")
		}
		d, _ := r.Describe("tester", "write")
		if d.AppendSource != "convention" {
			t.Errorf("expected convention append source, got %q", d.AppendSource)
		}
	})

	t.Run("when both config append and append file exist the config string wins with a warning", func(t *testing.T) {
		ws := t.TempDir()
		writeWorkspaceTemplate(t, ws, "tester", "write", ".append.tmpl", "FILE APPEND\n")
		var warnings []string
		r, err := newRenderer(ws, map[string]map[string]Override{
			"tester": {"write": {Append: "CONFIG APPEND"}},
		}, func(format string, args ...any) {
			warnings = append(warnings, format)
		})
		if err != nil {
			t.Fatal(err)
		}
		renderedP, _ := r.Render("tester", "write", data)
		got := renderedP.Full()
		if !strings.Contains(got, "CONFIG APPEND") || strings.Contains(got, "FILE APPEND") {
			t.Error("expected config append to win over the append file")
		}
		if len(warnings) == 0 {
			t.Error("expected a warning about the ignored append file")
		}
	})

	t.Run("when a full override and an append coexist construction fails fast", func(t *testing.T) {
		ws := t.TempDir()
		writeWorkspaceTemplate(t, ws, "tester", "write", ".tmpl", "OVERRIDE BODY\n")
		writeWorkspaceTemplate(t, ws, "tester", "write", ".append.tmpl", "APPEND CONTENT\n")
		_, err := NewRenderer(ws, nil)
		if err == nil || !strings.Contains(err.Error(), "tester/write") {
			t.Fatalf("expected key-named conflict error, got: %v", err)
		}
	})

	t.Run("when a config path override and a config append coexist construction fails fast", func(t *testing.T) {
		ws := t.TempDir()
		path := filepath.Join(ws, "custom.tmpl")
		if err := os.WriteFile(path, []byte("BODY\n"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := NewRenderer(ws, map[string]map[string]Override{
			"tester": {"write": {Path: path, Append: "extra"}},
		})
		if err == nil || !strings.Contains(err.Error(), "conflict") {
			t.Fatalf("expected conflict error, got: %v", err)
		}
	})
}

func TestRenderer_ContractAlwaysAppended(t *testing.T) {
	t.Run("when rendering every catalog key the output ends with the agent contract", func(t *testing.T) {
		r := NewDefaultRenderer()
		for _, agent := range Agents() {
			for _, action := range Actions(agent) {
				rendered, err := r.Render(agent, action, FixtureData(agent))
				if err != nil {
					t.Fatalf("Render %s/%s failed: %v", agent, action, err)
				}
				if !strings.HasSuffix(rendered.Full(), Contract(agent)) {
					t.Errorf("%s/%s: rendered prompt does not end with the output contract", agent, action)
				}
			}
		}
	})

	t.Run("when rendering an overridden template the contract is still appended", func(t *testing.T) {
		ws := t.TempDir()
		writeWorkspaceTemplate(t, ws, "generator", "fix", ".tmpl", "SHORT CUSTOM BODY\n")
		r, err := NewRenderer(ws, nil)
		if err != nil {
			t.Fatal(err)
		}
		renderedP, _ := r.Render("generator", "fix", TaskPromptData{})
		got := renderedP.Full()
		if got != "SHORT CUSTOM BODY\n"+Contract("generator") {
			t.Error("expected override body + contract, nothing else")
		}
	})
}

func TestDefaultTemplate(t *testing.T) {
	t.Run("when requesting an embedded default it returns the template text", func(t *testing.T) {
		text, err := DefaultTemplate("product_manager", "generate")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "{{.Spec}}") {
			t.Error("expected Spec placeholder in PM generate default")
		}
	})

	t.Run("when requesting an unknown key it errors", func(t *testing.T) {
		if _, err := DefaultTemplate("tester", "nonexistent"); err == nil {
			t.Fatal("expected error")
		}
	})
}
