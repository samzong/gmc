package llm

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/config"
	"github.com/sashabaranov/go-openai"
)

var (
	versionPattern   = regexp.MustCompile(`(?i)version:\s*(v?\d+\.\d+\.\d+)`)
	reasonPattern    = regexp.MustCompile(`(?is)reason:\s*(.+)$`)
	errMissingAPIKey = errors.New(
		"API key not set, please set the API key first: gmc config set apikey YOUR_API_KEY",
	)
)

func newOpenAIClient(model string) (*openai.Client, context.Context, context.CancelFunc, string, error) {
	cfg := config.GetConfig()

	if cfg.APIKey == "" {
		return nil, nil, nil, "", errMissingAPIKey
	}

	clientConfig := openai.DefaultConfig(cfg.APIKey)

	if cfg.APIBase != "" {
		clientConfig.BaseURL = cfg.APIBase
	}

	client := openai.NewClientWithConfig(clientConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	if model == "" {
		model = cfg.Model
	}

	return client, ctx, cancel, model, nil
}

func GenerateCommitMessage(prompt string, model string) (string, error) {
	client, ctx, cancel, chosenModel, err := newOpenAIClient(model)
	if err != nil {
		return "", err
	}
	defer cancel()

	messages := []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: "You are a professional Git commit message generator, helping developers generate " +
				"commit messages that comply with the Conventional Commits specification.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	resp, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:    chosenModel,
			Messages: messages,
		},
	)

	if err != nil {
		return "", fmt.Errorf("failed to call LLM: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("LLM returned empty response")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func SuggestVersion(baseVersion string, commits []string, model string) (string, string, error) {
	if len(commits) == 0 {
		return "", "", errors.New("no commits provided for version suggestion")
	}

	client, ctx, cancel, chosenModel, err := newOpenAIClient(model)
	if err != nil {
		return "", "", err
	}
	defer cancel()

	prompt := buildVersionPrompt(baseVersion, commits)

	messages := []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: "You are a release manager that recommends the next semantic version. " +
				"Always follow Semantic Versioning rules and respond using VERSION/REASON fields.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	resp, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:    chosenModel,
			Messages: messages,
		},
	)

	if err != nil {
		return "", "", fmt.Errorf("failed to call LLM: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", "", errors.New("LLM returned empty response")
	}

	version, reason, err := parseVersionSuggestion(resp.Choices[0].Message.Content)
	if err != nil {
		return "", "", err
	}

	return version, reason, nil
}

func buildVersionPrompt(baseVersion string, commits []string) string {
	var builder strings.Builder
	for i, commit := range commits {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(commit)))
	}

	return fmt.Sprintf(`Current version: %s

Commits since last release:
%s

Apply semantic versioning:
- Breaking change or incompatible API -> MAJOR
- New feature (backward compatible) -> MINOR
- Fix/perf/refactor/build/ci/revert -> PATCH
- Documentation/style/test/chore alone should keep the version the same unless nothing else applies.

Respond exactly in this format:
VERSION: vX.Y.Z
REASON: <short explanation>

If no release should happen, repeat the current version.`,
		strings.TrimSpace(baseVersion), builder.String())
}

func parseVersionSuggestion(response string) (string, string, error) {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return "", "", errors.New("LLM returned empty response")
	}

	versionMatch := versionPattern.FindStringSubmatch(trimmed)
	if len(versionMatch) < 2 {
		return "", "", errors.New("LLM response missing VERSION line in expected format")
	}

	version := strings.TrimSpace(versionMatch[1])
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	reason := ""
	if reasonMatch := reasonPattern.FindStringSubmatch(trimmed); len(reasonMatch) >= 2 {
		reason = strings.TrimSpace(reasonMatch[1])
	}

	return version, reason, nil
}
