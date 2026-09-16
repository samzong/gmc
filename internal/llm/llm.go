package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/config"
	"github.com/sashabaranov/go-openai"
)

var (
	ErrLLM           = errors.New("LLM error")
	errMissingAPIKey = errors.New("API key not set, please set the API key first: gmc config set apikey YOUR_API_KEY")
)

type Options struct {
	Timeout time.Duration
}

type Client struct {
	timeout time.Duration
}

const defaultTimeout = 30 * time.Second

func NewClient(opts Options) *Client {
	return &Client{timeout: opts.Timeout}
}

func (c *Client) effectiveTimeout() time.Duration {
	if c == nil || c.timeout <= 0 {
		return defaultTimeout
	}
	return c.timeout
}

func (c *Client) complete(request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	if cfg.APIKey == "" {
		return openai.ChatCompletionResponse{}, errMissingAPIKey
	}
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if cfg.APIBase != "" {
		clientConfig.BaseURL = cfg.APIBase
	}
	if request.Model == "" {
		request.Model = cfg.Model
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.effectiveTimeout())
	defer cancel()
	resp, err := openai.NewClientWithConfig(clientConfig).CreateChatCompletion(ctx, request)
	if err != nil {
		return resp, fmt.Errorf("failed to call LLM: %w (%w)", err, ErrLLM)
	}
	if len(resp.Choices) == 0 {
		return resp, fmt.Errorf("LLM returned empty response: %w", ErrLLM)
	}
	return resp, nil
}

func (c *Client) GenerateCommitMessage(prompt string, model string) (string, error) {
	resp, err := c.complete(openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem,
				Content: "You are a professional Git commit message generator, helping developers generate " +
					"commit messages that comply with the Conventional Commits specification."},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})
	if err != nil {
		return "", err
	}
	return firstChoiceContent(resp)
}

func (c *Client) TestConnection(model string) error {
	_, err := c.complete(openai.ChatCompletionRequest{
		Model:       model,
		Messages:    []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, Content: "Reply with OK."}},
		MaxTokens:   1,
		Temperature: 0,
	})
	return err
}

func firstChoiceContent(resp openai.ChatCompletionResponse) (string, error) {
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned empty response: %w", ErrLLM)
	}

	choice := resp.Choices[0]
	if choice.FinishReason == openai.FinishReasonLength {
		return "", fmt.Errorf("LLM response was cut off by the token limit: %w", ErrLLM)
	}

	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		return "", fmt.Errorf("LLM returned an empty message: %w", ErrLLM)
	}

	return content, nil
}
