package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookPayload represents the notification payload sent to remote webhooks (Slack, Discord, generic).
type WebhookPayload struct {
	Text      string `json:"text,omitempty"`    // Slack format
	Content   string `json:"content,omitempty"` // Discord format
	Event     string `json:"event"`             // Generic event category
	Title     string `json:"title"`             // Alert title
	Message   string `json:"message"`           // Alert description
	Timestamp string `json:"timestamp"`         // ISO8601 timestamp
}

// WebhookNotifier dispatches alerts to remote HTTP webhook endpoints.
type WebhookNotifier struct {
	WebhookURL string
	DoHTTP     func(req *http.Request) (*http.Response, error)
}

// NewWebhookNotifier creates a WebhookNotifier for the target URL.
func NewWebhookNotifier(url string) *WebhookNotifier {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	return &WebhookNotifier{
		WebhookURL: url,
		DoHTTP:     client.Do,
	}
}

// Notify sends the alert JSON payload to the configured webhook endpoint.
func (w *WebhookNotifier) Notify(ctx context.Context, kind NotificationKind, title, message string) error {
	if w.WebhookURL == "" {
		return nil
	}

	summaryText := fmt.Sprintf("[%s] %s: %s", kind, title, message)
	payload := WebhookPayload{
		Text:      summaryText,
		Content:   summaryText,
		Event:     string(kind),
		Title:     title,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.WebhookURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Noctifab-Dark-Factory/1.0")

	resp, err := w.DoHTTP(req)
	if err != nil {
		return fmt.Errorf("failed to dispatch webhook: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with status: %d", resp.StatusCode)
	}

	return nil
}
