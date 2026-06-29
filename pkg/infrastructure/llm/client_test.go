package llm

import (
	"testing"
)

func TestResolveGeminiURL(t *testing.T) {
	tests := []struct {
		name       string
		modelInput string
		apiKey     string
		wantURL    string
	}{
		{
			name:       "empty model input maps to gemini-2.5-pro",
			modelInput: "",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=testkey",
		},
		{
			name:       "gemini-1.5-pro maps to gemini-1.5-pro-latest",
			modelInput: "gemini-1.5-pro",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-pro-latest:generateContent?key=testkey",
		},
		{
			name:       "models/gemini-1.5-pro maps to gemini-1.5-pro-latest",
			modelInput: "models/gemini-1.5-pro",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-pro-latest:generateContent?key=testkey",
		},
		{
			name:       "gemini-2.5-flash remains gemini-2.5-flash",
			modelInput: "gemini-2.5-flash",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=testkey",
		},
		{
			name:       "models/gemini-2.5-flash remains gemini-2.5-flash",
			modelInput: "models/gemini-2.5-flash",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=testkey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL := resolveGeminiURL(tt.modelInput, tt.apiKey)
			if gotURL != tt.wantURL {
				t.Errorf("resolveGeminiURL(%q, %q) = %q; want %q", tt.modelInput, tt.apiKey, gotURL, tt.wantURL)
			}
		})
	}
}

func TestGetNextLowerModel(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		currentModel string
		wantModel    string
	}{
		{
			name:         "gemini-2.5-pro falls back to gemini-1.5-pro-latest",
			provider:     "gemini",
			currentModel: "gemini-2.5-pro",
			wantModel:    "gemini-1.5-pro-latest",
		},
		{
			name:         "models/gemini-2.5-pro falls back to gemini-1.5-pro-latest",
			provider:     "Gemini",
			currentModel: "models/gemini-2.5-pro",
			wantModel:    "gemini-1.5-pro-latest",
		},
		{
			name:         "gemini-1.5-pro-latest falls back to gemini-2.5-flash",
			provider:     "gemini",
			currentModel: "gemini-1.5-pro-latest",
			wantModel:    "gemini-2.5-flash",
		},
		{
			name:         "gemini-1.5-flash-latest is lowest and returns empty",
			provider:     "gemini",
			currentModel: "gemini-1.5-flash-latest",
			wantModel:    "",
		},
		{
			name:         "openai gpt-4o falls back to gpt-4o-mini",
			provider:     "openai",
			currentModel: "gpt-4o",
			wantModel:    "gpt-4o-mini",
		},
		{
			name:         "unknown provider returns empty",
			provider:     "unknown",
			currentModel: "gpt-4o",
			wantModel:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getNextLowerModel(tt.provider, tt.currentModel)
			if got != tt.wantModel {
				t.Errorf("getNextLowerModel(%q, %q) = %q; want %q", tt.provider, tt.currentModel, got, tt.wantModel)
			}
		})
	}
}
