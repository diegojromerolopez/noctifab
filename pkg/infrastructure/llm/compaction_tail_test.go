package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestCompactPromptUncompactableTail(t *testing.T) {
	// A tail that compaction WOULD rewrite if it were allowed to touch it:
	// caveman strips "---" divider lines and "Please note that" prefixes.
	tail := "\nReturn format:\n---\nPlease note that this schema is mandatory.\n{\"reasoning\": \"...\"}\n"
	body := "Please note that this body is compactable.\n---\nDo the task.\n"

	t.Run("when the context marks a non-compactable tail the tail survives caveman compaction verbatim", func(t *testing.T) {
		c := &Client{Compaction: "caveman"}
		ctx := domain.WithUncompactableTail(context.Background(), len(tail))
		got := c.compactPrompt(ctx, body+tail)
		if !strings.HasSuffix(got, tail) {
			t.Errorf("expected the tail to survive compaction verbatim, got:\n%s", got)
		}
		if strings.Contains(got, "Please note that this body is compactable.") {
			t.Error("expected the body to be compacted")
		}
	})

	t.Run("when no tail is marked the whole prompt is compacted", func(t *testing.T) {
		c := &Client{Compaction: "caveman"}
		got := c.compactPrompt(context.Background(), body+tail)
		if strings.HasSuffix(got, tail) {
			t.Error("expected the unmarked tail to be compacted like the rest")
		}
	})

	t.Run("when compaction is disabled the prompt is unchanged", func(t *testing.T) {
		c := &Client{}
		ctx := domain.WithUncompactableTail(context.Background(), len(tail))
		got := c.compactPrompt(ctx, body+tail)
		if got != body+tail {
			t.Error("expected the prompt to pass through unchanged")
		}
	})

	t.Run("when the tail length is out of range the whole prompt is compacted safely", func(t *testing.T) {
		c := &Client{Compaction: "caveman"}
		ctx := domain.WithUncompactableTail(context.Background(), len(body+tail)+100)
		got := c.compactPrompt(ctx, body+tail)
		if got == "" {
			t.Error("expected non-empty output")
		}
	})

	t.Run("when using simple_english mode the tail also survives verbatim", func(t *testing.T) {
		seTail := "\nReturn format: utilize double quotes.\n"
		c := &Client{Compaction: "simple_english"}
		ctx := domain.WithUncompactableTail(context.Background(), len(seTail))
		got := c.compactPrompt(ctx, "We utilize this component.\n"+seTail)
		if !strings.HasSuffix(got, seTail) {
			t.Errorf("expected simple_english to leave the tail untouched, got:\n%s", got)
		}
		if strings.Contains(strings.TrimSuffix(got, seTail), "utilize") {
			t.Error("expected the body vocabulary to be simplified")
		}
	})
}

func TestUncompactableTailContext(t *testing.T) {
	t.Run("when no tail is recorded it returns zero", func(t *testing.T) {
		if domain.UncompactableTailLen(context.Background()) != 0 {
			t.Error("expected 0 for unset context")
		}
	})

	t.Run("when a tail is recorded it round-trips", func(t *testing.T) {
		ctx := domain.WithUncompactableTail(context.Background(), 42)
		if domain.UncompactableTailLen(ctx) != 42 {
			t.Error("expected 42")
		}
	})

	t.Run("when a non-positive tail is recorded it returns zero", func(t *testing.T) {
		ctx := domain.WithUncompactableTail(context.Background(), -5)
		if domain.UncompactableTailLen(ctx) != 0 {
			t.Error("expected 0 for negative tail length")
		}
	})
}
