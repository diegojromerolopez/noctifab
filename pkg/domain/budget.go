package domain

import "context"

type BudgetRecord struct {
	Date     string
	Provider string
	CostUSD  float64
}

type BudgetStore interface {
	GetDailyUsage(ctx context.Context, date string, provider string) (float64, error)
	IncrementUsage(ctx context.Context, date string, provider string, costUSD float64) error
}

type pricingTier struct {
	prefix string
	input  float64
	output float64
}

var pricingTable = []pricingTier{
	{prefix: "gpt-4o", input: 0.01, output: 0.03},
	{prefix: "gpt-4", input: 0.03, output: 0.06},
	{prefix: "gpt-3.5", input: 0.0005, output: 0.0015},
	{prefix: "claude-3", input: 0.015, output: 0.075},
	{prefix: "claude-2", input: 0.01102, output: 0.03268},
	{prefix: "gemini-3.5", input: 0.000075, output: 0.0003},
	{prefix: "gemini-3.1", input: 0.000075, output: 0.0003},
	{prefix: "gemini-3-", input: 0.000075, output: 0.0003},
	{prefix: "gemini-2.5", input: 0.000075, output: 0.0003},
	{prefix: "gemini-1.5", input: 0.000125, output: 0.000375},
	{prefix: "gemini-1.0", input: 0.001, output: 0.002},
	{prefix: "gemini-pro-latest", input: 0.00125, output: 0.00375},
	{prefix: "gemini-flash-latest", input: 0.000075, output: 0.0003},
	{prefix: "gemini-flash-lite-latest", input: 0.0000375, output: 0.00015},
	{prefix: "gemini-", input: 0.000125, output: 0.000375},
	{prefix: "deepseek", input: 0.0005, output: 0.0015},
	{prefix: "mistral", input: 0.00015, output: 0.0006},
	{prefix: "llama", input: 0.0005, output: 0.0015},
	{prefix: "command", input: 0.015, output: 0.015},
	{prefix: "davin", input: 0.02, output: 0.02},
}

func CostForTokens(model string, promptTokens, completionTokens int) float64 {
	if promptTokens <= 0 && completionTokens <= 0 {
		return 0
	}
	var rate pricingTier
	for _, t := range pricingTable {
		if len(model) >= len(t.prefix) && model[:len(t.prefix)] == t.prefix {
			rate = t
			break
		}
	}
	if rate.prefix == "" {
		return 0
	}
	inputCost := float64(promptTokens) / 1000 * rate.input
	outputCost := float64(completionTokens) / 1000 * rate.output
	return inputCost + outputCost
}
