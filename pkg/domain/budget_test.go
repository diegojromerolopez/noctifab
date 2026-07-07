package domain

import (
	"context"
	"testing"
)

func TestCostForTokens(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		promptTokens     int
		completionTokens int
		want             float64
	}{
		{name: "gpt-4o exact", model: "gpt-4o", promptTokens: 1000, completionTokens: 500, want: 0.01*1 + 0.03*0.5},
		{name: "gpt-4o variant", model: "gpt-4o-mini", promptTokens: 1000, completionTokens: 500, want: 0.01*1 + 0.03*0.5},
		{name: "gpt-4", model: "gpt-4", promptTokens: 2000, completionTokens: 1000, want: 0.03*2 + 0.06*1},
		{name: "gpt-3.5-turbo", model: "gpt-3.5-turbo", promptTokens: 10000, completionTokens: 2000, want: 0.0005*10 + 0.0015*2},
		{name: "claude-3-haiku", model: "claude-3-haiku", promptTokens: 1000, completionTokens: 1000, want: 0.015*1 + 0.075*1},
		{name: "gemini-1.5-flash", model: "gemini-1.5-flash", promptTokens: 1000, completionTokens: 1000, want: 0.000125*1 + 0.000375*1},
		{name: "deepseek-coder", model: "deepseek-coder", promptTokens: 1000, completionTokens: 500, want: 0.0005*1 + 0.0015*0.5},
		{name: "mistral-small", model: "mistral-small", promptTokens: 1000, completionTokens: 500, want: 0.00015*1 + 0.0006*0.5},
		{name: "llama-3", model: "llama-3", promptTokens: 1000, completionTokens: 500, want: 0.0005*1 + 0.0015*0.5},
		{name: "command-r", model: "command-r", promptTokens: 1000, completionTokens: 500, want: 0.015*1 + 0.015*0.5},
		{name: "davinci-002", model: "davinci-002", promptTokens: 1000, completionTokens: 500, want: 0.02*1 + 0.02*0.5},
		{name: "unknown model", model: "unknown-model", promptTokens: 1000, completionTokens: 500, want: 0},
		{name: "zero tokens", model: "gpt-4o", promptTokens: 0, completionTokens: 0, want: 0},
		{name: "negative tokens", model: "gpt-4o", promptTokens: -100, completionTokens: -50, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CostForTokens(tt.model, tt.promptTokens, tt.completionTokens)
			if got != tt.want {
				t.Errorf("CostForTokens(%q, %d, %d) = %.6f, want %.6f", tt.model, tt.promptTokens, tt.completionTokens, got, tt.want)
			}
		})
	}
}

type mockBudgetStore struct {
	usage   float64
	records map[string]float64
}

func (m *mockBudgetStore) GetDailyUsage(_ context.Context, date string, provider string) (float64, error) {
	if m.records != nil {
		return m.records[date+"|"+provider], nil
	}
	return m.usage, nil
}

func (m *mockBudgetStore) IncrementUsage(_ context.Context, date string, provider string, costUSD float64) error {
	if m.records == nil {
		m.records = make(map[string]float64)
	}
	m.records[date+"|"+provider] += costUSD
	return nil
}

func TestBudgetStoreInterface(t *testing.T) {
	t.Run("GetDailyUsage returns zero when no records exist", func(t *testing.T) {
		store := &mockBudgetStore{records: make(map[string]float64)}
		usage, err := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage != 0 {
			t.Errorf("expected 0, got %.2f", usage)
		}
	})

	t.Run("IncrementUsage accumulates correctly", func(t *testing.T) {
		store := &mockBudgetStore{records: make(map[string]float64)}
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 1.50)
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 2.50)
		usage, _ := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if usage != 4.00 {
			t.Errorf("expected 4.00, got %.2f", usage)
		}
	})

	t.Run("different providers tracked separately", func(t *testing.T) {
		store := &mockBudgetStore{records: make(map[string]float64)}
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 1.00)
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "claude-3", 2.00)
		gptUsage, _ := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		claudeUsage, _ := store.GetDailyUsage(context.Background(), "2026-07-02", "claude-3")
		if gptUsage != 1.00 {
			t.Errorf("expected gpt-4o usage 1.00, got %.2f", gptUsage)
		}
		if claudeUsage != 2.00 {
			t.Errorf("expected claude-3 usage 2.00, got %.2f", claudeUsage)
		}
	})
}
