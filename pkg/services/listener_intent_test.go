package services_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
)

func TestParseIntentFromLLMResponse_StartStory(t *testing.T) {
	t.Run("when kind is START_STORY, it returns an IntentKindStartStory with the path", func(t *testing.T) {
		resp := services.LLMIntentResponse{Kind: "START_STORY", Path: "roadmap/US-0001.md"}
		intent := services.ParseIntentFromLLMResponse(resp)

		assert.Equal(t, services.IntentKindStartStory, intent.Kind)
		assert.Equal(t, "roadmap/US-0001.md", intent.Path)
	})
}

func TestParseIntentFromLLMResponse_StartDirectory(t *testing.T) {
	t.Run("when kind is START_DIRECTORY, it returns an IntentKindStartDirectory with the directory path", func(t *testing.T) {
		resp := services.LLMIntentResponse{Kind: "START_DIRECTORY", Path: "/home/user/repos/frontpunch/roadmap"}
		intent := services.ParseIntentFromLLMResponse(resp)

		assert.Equal(t, services.IntentKindStartDirectory, intent.Kind)
		assert.Equal(t, "/home/user/repos/frontpunch/roadmap", intent.Path)
	})
}

func TestParseIntentFromLLMResponse_ListStatus(t *testing.T) {
	t.Run("when kind is LIST_STATUS, it returns an IntentKindListStatus", func(t *testing.T) {
		resp := services.LLMIntentResponse{Kind: "LIST_STATUS", Message: "Current tasks: 3 pending"}
		intent := services.ParseIntentFromLLMResponse(resp)

		assert.Equal(t, services.IntentKindListStatus, intent.Kind)
		assert.Equal(t, "Current tasks: 3 pending", intent.Message)
	})
}

func TestParseIntentFromLLMResponse_Unknown(t *testing.T) {
	t.Run("when kind is unknown, it returns IntentKindUnknown with a help message", func(t *testing.T) {
		resp := services.LLMIntentResponse{Kind: "FOOBAR"}
		intent := services.ParseIntentFromLLMResponse(resp)

		assert.Equal(t, services.IntentKindUnknown, intent.Kind)
		assert.Contains(t, intent.Message, "did not understand")
	})
}

func TestParseIntentFromLLMResponse_UnknownWithMessage(t *testing.T) {
	t.Run("when kind is unknown but a message is present, it preserves the message", func(t *testing.T) {
		resp := services.LLMIntentResponse{Kind: "FOOBAR", Message: "Try the start command instead."}
		intent := services.ParseIntentFromLLMResponse(resp)

		assert.Equal(t, services.IntentKindUnknown, intent.Kind)
		assert.Equal(t, "Try the start command instead.", intent.Message)
	})
}
