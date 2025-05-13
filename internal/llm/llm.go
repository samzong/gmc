package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/samzong/gma/internal/config"
	"github.com/sashabaranov/go-openai"
)

func GenerateCommitMessage(prompt string, model string) (string, error) {
	cfg := config.GetConfig()
	
	if cfg.APIKey == "" {
		return "", errors.New("API key not set, please set the API key first: gma config set apikey YOUR_API_KEY")
	}

	clientConfig := openai.DefaultConfig(cfg.APIKey)
	
	if cfg.APIBase != "" {
		clientConfig.BaseURL = cfg.APIBase
	}
	
	client := openai.NewClientWithConfig(clientConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if model == "" {
		model = cfg.Model
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a professional Git commit message generator, helping developers generate commit messages that comply with the Conventional Commits specification.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	resp, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:    model,
			Messages: messages,
		},
	)

	if err != nil {
		return "", fmt.Errorf("Failed to call LLM: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("LLM returned empty response")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
} 