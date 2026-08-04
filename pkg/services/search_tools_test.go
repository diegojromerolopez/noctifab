package services

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func grepState(t *testing.T) (*domain.State, string) {
	t.Helper()
	dir := t.TempDir()
	return &domain.State{ProjectPath: dir}, dir
}

func TestGrepSearchTool(t *testing.T) {
	tool := &GrepSearchTool{}

	t.Run("when a pattern matches it returns file, line number and content", func(t *testing.T) {
		state, dir := grepState(t)
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha\nneedle here\nomega"), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := tool.Execute(context.Background(), state, map[string]any{"query": "needle"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "a.txt:2: needle here") {
			t.Errorf("expected match line, got %q", out)
		}
	})

	t.Run("when a file exceeds 1MB it is skipped", func(t *testing.T) {
		state, dir := grepState(t)
		big := strings.Repeat("needle padding line\n", (grepMaxFileSize/20)+10)
		if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(big), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := tool.Execute(context.Background(), state, map[string]any{"query": "needle"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "" {
			t.Errorf("expected large file to be skipped, got %d bytes of matches", len(out))
		}
	})

	t.Run("when a file is binary it is skipped", func(t *testing.T) {
		state, dir := grepState(t)
		binContent := append([]byte("needle"), 0x00, 0x01, 0x02)
		if err := os.WriteFile(filepath.Join(dir, "bin.dat"), binContent, 0644); err != nil {
			t.Fatal(err)
		}
		out, err := tool.Execute(context.Background(), state, map[string]any{"query": "needle"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "" {
			t.Errorf("expected binary file to be skipped, got %q", out)
		}
	})

	t.Run("when matches exceed 200 it truncates and appends a marker", func(t *testing.T) {
		state, dir := grepState(t)
		var sb strings.Builder
		for i := 0; i < 300; i++ {
			fmt.Fprintf(&sb, "needle %d\n", i)
		}
		if err := os.WriteFile(filepath.Join(dir, "many.txt"), []byte(sb.String()), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := tool.Execute(context.Background(), state, map[string]any{"query": "needle"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(out, "\n")
		if len(lines) != grepMaxMatches+1 {
			t.Errorf("expected %d lines (200 matches + marker), got %d", grepMaxMatches+1, len(lines))
		}
		if lines[len(lines)-1] != grepTruncationMarker {
			t.Errorf("expected truncation marker as last line, got %q", lines[len(lines)-1])
		}
	})

	t.Run("when a matched line exceeds 500 chars it is capped", func(t *testing.T) {
		state, dir := grepState(t)
		long := "needle " + strings.Repeat("x", 1000)
		if err := os.WriteFile(filepath.Join(dir, "long.txt"), []byte(long), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := tool.Execute(context.Background(), state, map[string]any{"query": "needle"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		parts := strings.SplitN(out, ": ", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected output format: %q", out)
		}
		if len(parts[1]) != grepMaxLineLength {
			t.Errorf("expected matched line capped at %d chars, got %d", grepMaxLineLength, len(parts[1]))
		}
	})

	t.Run("when the query is missing it returns an error", func(t *testing.T) {
		state, _ := grepState(t)
		if _, err := tool.Execute(context.Background(), state, map[string]any{}); err == nil {
			t.Error("expected an error for missing query")
		}
	})

	t.Run("when the regex is invalid it returns an error", func(t *testing.T) {
		state, _ := grepState(t)
		if _, err := tool.Execute(context.Background(), state, map[string]any{"query": "("}); err == nil {
			t.Error("expected an error for invalid regex")
		}
	})
}

func TestIsBinaryContent(t *testing.T) {
	t.Run("when content has a NUL byte in the first 512 bytes it is binary", func(t *testing.T) {
		if !isBinaryContent([]byte{'a', 0x00, 'b'}) {
			t.Error("expected binary detection")
		}
	})

	t.Run("when content is plain text it is not binary", func(t *testing.T) {
		if isBinaryContent([]byte("just text\nwith lines\n")) {
			t.Error("expected non-binary detection")
		}
	})

	t.Run("when a NUL byte appears only after 512 bytes it is treated as text", func(t *testing.T) {
		content := append(bytes.Repeat([]byte{'a'}, 600), 0x00)
		if isBinaryContent(content) {
			t.Error("expected text detection when NUL is past the sniff window")
		}
	})
}
