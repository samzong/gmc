package llm

import (
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstChoiceContent(t *testing.T) {
	choice := func(content string, reason openai.FinishReason) openai.ChatCompletionResponse {
		return openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{
				Message:      openai.ChatCompletionMessage{Content: content},
				FinishReason: reason,
			}},
		}
	}

	tests := []struct {
		name    string
		resp    openai.ChatCompletionResponse
		want    string
		wantErr bool
	}{
		{
			name: "returns the trimmed message",
			resp: choice("\n  feat: add thing \n", openai.FinishReasonStop),
			want: "feat: add thing",
		},
		{
			name:    "rejects a response with no choices",
			resp:    openai.ChatCompletionResponse{},
			wantErr: true,
		},
		{
			name:    "rejects empty content",
			resp:    choice("", openai.FinishReasonStop),
			wantErr: true,
		},
		{
			name:    "rejects whitespace-only content",
			resp:    choice("   \n\t ", openai.FinishReasonStop),
			wantErr: true,
		},
		{
			name:    "rejects a response cut off by the token limit",
			resp:    choice("feat: partial", openai.FinishReasonLength),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := firstChoiceContent(tt.resp)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrLLM)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
