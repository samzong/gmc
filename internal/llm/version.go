package llm

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sashabaranov/go-openai"
)

var (
	versionPattern = regexp.MustCompile(`(?i)version:\s*(v?\d+\.\d+\.\d+)`)
	reasonPattern  = regexp.MustCompile(`(?is)reason:\s*(.+)$`)
)

func (c *Client) SuggestVersion(baseVersion string, commits []string, model string) (string, string, error) {
	if len(commits) == 0 {
		return "", "", errors.New("no commits provided for version suggestion")
	}
	resp, err := c.complete(openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem,
				Content: "You are a release manager that recommends the next semantic version. " +
					"Always follow Semantic Versioning rules and respond using VERSION/REASON fields."},
			{Role: openai.ChatMessageRoleUser, Content: buildVersionPrompt(baseVersion, commits)},
		},
	})
	if err != nil {
		return "", "", err
	}
	content, err := firstChoiceContent(resp)
	if err != nil {
		return "", "", err
	}
	version, reason, err := parseVersionSuggestion(content)
	if err != nil {
		return "", "", fmt.Errorf("%w (%w)", err, ErrLLM)
	}
	return version, reason, nil
}

func buildVersionPrompt(baseVersion string, commits []string) string {
	var builder strings.Builder
	for i, commit := range commits {
		fmt.Fprintf(&builder, "%d. %s\n", i+1, strings.TrimSpace(commit))
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
