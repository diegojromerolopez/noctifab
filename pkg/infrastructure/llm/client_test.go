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
			name:       "gemini-1.5-pro maps to gemini-2.5-pro",
			modelInput: "gemini-1.5-pro",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=testkey",
		},
		{
			name:       "models/gemini-1.5-pro maps to gemini-2.5-pro",
			modelInput: "models/gemini-1.5-pro",
			apiKey:     "testkey",
			wantURL:    "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=testkey",
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
