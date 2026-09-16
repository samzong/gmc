package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveLLM(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("api_key", "test-api-key")
	viper.Set("api_base", server.URL+"/v1")
	viper.Set("model", "configured-model")
	return NewClient(Options{Timeout: time.Second})
}

func TestGenerateCommitMessage(t *testing.T) {
	for _, model := range []string{"", "explicit-model"} {
		t.Run(model, func(t *testing.T) {
			client := serveLLM(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/chat/completions", r.URL.Path)
				assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
				var request openai.ChatCompletionRequest
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
					return
				}
				expectedModel := model
				if expectedModel == "" {
					expectedModel = "configured-model"
				}
				assert.Equal(t, expectedModel, request.Model)
				if assert.Len(t, request.Messages, 2) {
					assert.Equal(t, openai.ChatMessageRoleSystem, request.Messages[0].Role)
					assert.Contains(t, request.Messages[0].Content, "Conventional Commits")
					assert.Equal(t, openai.ChatMessageRoleUser, request.Messages[1].Role)
					assert.Equal(t, "Add authentication", request.Messages[1].Content)
				}
				fmt.Fprint(w, `{"choices":[{"message":{"content":"  feat: authenticate\n"},"finish_reason":"stop"}]}`)
			})
			message, err := client.GenerateCommitMessage("Add authentication", model)
			require.NoError(t, err)
			assert.Equal(t, "feat: authenticate", message)
		})
	}
}

func TestSuggestVersion(t *testing.T) {
	client := serveLLM(t, func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
			return
		}
		if assert.Len(t, request.Messages, 2) {
			assert.Contains(t, request.Messages[0].Content, "Semantic Versioning")
			assert.Contains(t, request.Messages[1].Content, "Current version: v1.0.0")
			assert.Contains(t, request.Messages[1].Content, "1. feat: example")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"VERSION: v1.1.0\nREASON: New capability"}}]}`)
	})
	version, reason, err := client.SuggestVersion("v1.0.0", []string{"feat: example"}, "")
	require.NoError(t, err)
	assert.Equal(t, "v1.1.0", version)
	assert.Equal(t, "New capability", reason)
	_, _, err = client.SuggestVersion("v1.0.0", nil, "")
	require.Error(t, err)
}

func TestConnectionAcceptsTokenLimitedResponse(t *testing.T) {
	client := serveLLM(t, func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&request)) {
			return
		}
		assert.Equal(t, 1, request.MaxTokens)
		assert.Zero(t, request.Temperature)
		assert.Equal(t, []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "Reply with OK."},
		}, request.Messages)
		fmt.Fprint(w, `{"choices":[{"message":{"content":""},"finish_reason":"length"}]}`)
	})
	require.NoError(t, client.TestConnection(""))
}

func TestCompletionFailures(t *testing.T) {
	for _, response := range []string{
		`{"choices":[]}`,
		`{"choices":[{"message":{"content":""}}]}`,
		`{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`,
		`{"error":{"message":"denied","type":"invalid_request_error"}}`,
	} {
		t.Run(response, func(t *testing.T) {
			client := serveLLM(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, response) })
			message, err := client.GenerateCommitMessage("", "")
			assert.Empty(t, message)
			require.ErrorIs(t, err, ErrLLM)
		})
	}
}

func TestCompletionTimeout(t *testing.T) {
	client := serveLLM(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	})
	client.timeout = 10 * time.Millisecond
	_, err := client.GenerateCommitMessage("prompt", "")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, ErrLLM)
}

func TestMissingAPIKey(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	client := NewClient(Options{})
	_, err := client.GenerateCommitMessage("prompt", "")
	require.ErrorIs(t, err, errMissingAPIKey)
	_, _, err = client.SuggestVersion("v1.0.0", []string{"fix: bug"}, "")
	require.ErrorIs(t, err, errMissingAPIKey)
	require.ErrorIs(t, client.TestConnection(""), errMissingAPIKey)
}

func TestParseVersionSuggestion(t *testing.T) {
	for _, tt := range []struct {
		input   string
		version string
		reason  string
	}{
		{"VERSION: v1.2.3\nREASON: Minor improvements", "v1.2.3", "Minor improvements"},
		{"Version: 0.2.0\nReason: Feature release", "v0.2.0", "Feature release"},
	} {
		version, reason, err := parseVersionSuggestion(tt.input)
		require.NoError(t, err)
		assert.Equal(t, tt.version, version)
		assert.Equal(t, tt.reason, reason)
	}
	_, _, err := parseVersionSuggestion("Unexpected response")
	require.Error(t, err)
}
