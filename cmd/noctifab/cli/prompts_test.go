package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPromptConfigPath(t *testing.T) {
	t.Run("when config flag is set it takes precedence over the environment", func(t *testing.T) {
		envPath := filepath.Join(t.TempDir(), "env", "config.yaml")
		flagPath := filepath.Join(t.TempDir(), "flag", "config.yaml")
		t.Setenv("NOCTIFAB_CONFIG", envPath)

		cmd := &cobra.Command{}
		cmd.Flags().String("config", ".noctifab/config.yaml", "")
		if err := cmd.Flags().Set("config", flagPath); err != nil {
			t.Fatal(err)
		}

		if got := promptConfigPath(cmd); got != flagPath {
			t.Fatalf("expected flag config path %q, got %q", flagPath, got)
		}
		if got := promptsWorkspace(cmd); got != filepath.Dir(filepath.Dir(flagPath)) {
			t.Fatalf("expected workspace derived from flag path, got %q", got)
		}
	})

	t.Run("when config flag is unset it uses the environment", func(t *testing.T) {
		envDir := filepath.Join(t.TempDir(), "env")
		envPath := filepath.Join(envDir, "config.yaml")
		t.Setenv("NOCTIFAB_CONFIG", envPath)
		if err := os.MkdirAll(envDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(envPath, []byte("prompts:\n  tester:\n    write:\n      append: custom\n"), 0644); err != nil {
			t.Fatal(err)
		}

		cmd := &cobra.Command{}
		cmd.Flags().String("config", ".noctifab/config.yaml", "")

		if got := promptConfigPath(cmd); got != envPath {
			t.Fatalf("expected environment config path %q, got %q", envPath, got)
		}
		overrides := loadPromptOverrides(cmd)
		if got := overrides["tester"]["write"].Append; got != "custom" {
			t.Fatalf("expected overrides loaded from environment config, got %q", got)
		}
	})
}

func runPromptsCmd(t *testing.T, workspace string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetErr(&out)
	defer RootCmd.SetOut(nil)
	defer RootCmd.SetErr(nil)
	full := append([]string{"prompts"}, args...)
	full = append(full, "--config", filepath.Join(workspace, ".noctifab", "config.yaml"))
	RootCmd.SetArgs(full)
	defer RootCmd.SetArgs(nil)
	err := RootCmd.Execute()
	return out.String(), err
}

func TestPromptsListCommand(t *testing.T) {
	t.Run("when no overrides exist it lists all 21 actions as embedded", func(t *testing.T) {
		ws := t.TempDir()
		out, err := runPromptsCmd(t, ws, "list")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, needle := range []string{"product_manager", "planner", "tester", "generator", "qa", "spec", "acceptance", "implement", "write_breadth_first"} {
			if !strings.Contains(out, needle) {
				t.Errorf("expected %q in list output, got:\n%s", needle, out)
			}
		}
		if strings.Count(out, "embedded") != 21 {
			t.Errorf("expected 21 embedded entries, got %d:\n%s", strings.Count(out, "embedded"), out)
		}
	})

	t.Run("when a convention override exists it is reported as convention", func(t *testing.T) {
		ws := t.TempDir()
		dir := filepath.Join(ws, ".noctifab", "prompts", "tester")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "write.tmpl"), []byte("custom {{.Title}}\n"), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := runPromptsCmd(t, ws, "list")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "convention") {
			t.Errorf("expected convention source in output:\n%s", out)
		}
	})
}

func TestPromptsShowCommand(t *testing.T) {
	t.Run("when showing a default action it prints the template and the contract", func(t *testing.T) {
		ws := t.TempDir()
		out, err := runPromptsCmd(t, ws, "show", "generator", "implement")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, needle := range []string{"# Source: embedded", "{{.Title}}", "Non-overridable output contract", "Return format"} {
			if !strings.Contains(out, needle) {
				t.Errorf("expected %q in show output", needle)
			}
		}
	})

	t.Run("when showing an unknown action it fails", func(t *testing.T) {
		ws := t.TempDir()
		if _, err := runPromptsCmd(t, ws, "show", "generator", "nope"); err == nil {
			t.Fatal("expected error for unknown action")
		}
	})
}

func TestPromptsInitCommand(t *testing.T) {
	t.Run("when initializing one action it writes the default template", func(t *testing.T) {
		ws := t.TempDir()
		out, err := runPromptsCmd(t, ws, "init", "tester", "write")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := filepath.Join(ws, ".noctifab", "prompts", "tester", "write.tmpl")
		if !strings.Contains(out, "created "+path) {
			t.Errorf("expected creation message, got:\n%s", out)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "{{.Title}}") {
			t.Error("expected default template content")
		}
	})

	t.Run("when the file already exists it never overwrites", func(t *testing.T) {
		ws := t.TempDir()
		dir := filepath.Join(ws, ".noctifab", "prompts", "tester")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "write.tmpl")
		if err := os.WriteFile(path, []byte("MINE\n"), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := runPromptsCmd(t, ws, "init", "tester", "write")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "skipped") {
			t.Errorf("expected skip message, got:\n%s", out)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "MINE\n" {
			t.Error("existing file must not be overwritten")
		}
	})

	t.Run("when initializing everything it writes all 21 templates", func(t *testing.T) {
		ws := t.TempDir()
		out, err := runPromptsCmd(t, ws, "init")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Count(out, "created ") != 21 {
			t.Errorf("expected 21 created files, got:\n%s", out)
		}
	})

	t.Run("when the agent is unknown it fails", func(t *testing.T) {
		ws := t.TempDir()
		if _, err := runPromptsCmd(t, ws, "init", "architect"); err == nil {
			t.Fatal("expected error for unknown agent")
		}
	})
}

func TestPromptsValidateCommand(t *testing.T) {
	t.Run("when all templates are valid it reports success with exit code 0", func(t *testing.T) {
		ws := t.TempDir()
		out, err := runPromptsCmd(t, ws, "validate")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "All prompt templates are valid.") {
			t.Errorf("expected success message, got:\n%s", out)
		}
	})

	t.Run("when an override has a parse error it fails naming the key", func(t *testing.T) {
		ws := t.TempDir()
		dir := filepath.Join(ws, ".noctifab", "prompts", "generator")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "fix.tmpl"), []byte("{{.Broken\n"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := runPromptsCmd(t, ws, "validate")
		if err == nil || !strings.Contains(err.Error(), "generator/fix") {
			t.Fatalf("expected key-named error, got: %v", err)
		}
	})

	t.Run("when the config declares an unknown agent it fails", func(t *testing.T) {
		ws := t.TempDir()
		cfgDir := filepath.Join(ws, ".noctifab")
		if err := os.MkdirAll(cfgDir, 0755); err != nil {
			t.Fatal(err)
		}
		cfg := "prompts:\n  architect:\n    design:\n      append: \"x\"\n"
		if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := runPromptsCmd(t, ws, "validate"); err == nil {
			t.Fatal("expected error for unknown agent in config")
		}
	})
}
