package notifier

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookNotifier_SuccessfulDispatch(t *testing.T) {
	var receivedURL string
	var receivedBody string

	mockDo := func(req *http.Request) (*http.Response, error) {
		receivedURL = req.URL.String()
		b, _ := io.ReadAll(req.Body)
		receivedBody = string(b)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	}

	w := &WebhookNotifier{
		WebhookURL: "https://hooks.slack.com/services/test",
		DoHTTP:     mockDo,
	}

	err := w.Notify(context.Background(), NotifyStoryCompleted, "Story US-001", "Build succeeded")
	require.NoError(t, err)

	assert.Equal(t, "https://hooks.slack.com/services/test", receivedURL)
	assert.Contains(t, receivedBody, "STORY_COMPLETED")
	assert.Contains(t, receivedBody, "Story US-001")
	assert.Contains(t, receivedBody, "Build succeeded")
}

func TestWebhookNotifier_EmptyURLNoop(t *testing.T) {
	called := false
	mockDo := func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	}

	w := &WebhookNotifier{
		WebhookURL: "",
		DoHTTP:     mockDo,
	}

	err := w.Notify(context.Background(), NotifyStoryCompleted, "Title", "Message")
	require.NoError(t, err)
	assert.False(t, called)
}

func TestWebhookNotifier_HTTPErrorStatus(t *testing.T) {
	mockDo := func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("server error")),
		}, nil
	}

	w := &WebhookNotifier{
		WebhookURL: "https://example.com/webhook",
		DoHTTP:     mockDo,
	}

	err := w.Notify(context.Background(), NotifyBuildFailed, "Build Failed", "Compilation error")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status: 500")
}

func TestWebhookNotifier_NetworkFailure(t *testing.T) {
	mockDo := func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}

	w := &WebhookNotifier{
		WebhookURL: "https://example.com/webhook",
		DoHTTP:     mockDo,
	}

	err := w.Notify(context.Background(), NotifyClarificationNeed, "Question", "Please answer")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}
