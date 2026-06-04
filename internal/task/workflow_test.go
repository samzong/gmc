package task

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadWorkflowForAgentPerAgentBlock(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("task.agents.codex.review_command", []string{"codex", "review", "--base", "main"})
	viper.Set("task.agents.codex.verify_command", []string{"pnpm", "check"})
	viper.Set("task.agents.grok.review_command", []string{"grok", "review"})
	viper.Set("task.agents.grok.verify_command", []string{"npm", "test"})

	codex := LoadWorkflowForAgent("codex")
	assert.Equal(t, []string{"codex", "review", "--base", "main"}, codex.ReviewCommand)
	assert.Equal(t, []string{"pnpm", "check"}, codex.VerifyCommand)

	grok := LoadWorkflowForAgent("grok")
	assert.Equal(t, []string{"grok", "review"}, grok.ReviewCommand)
	assert.Equal(t, []string{"npm", "test"}, grok.VerifyCommand)
}

func TestLoadWorkflowClaudeCodeAlias(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("task.agents.claude-code.review_command", []string{"claude", "review"})

	wf := LoadWorkflowForAgent("claude-code")
	assert.Equal(t, []string{"claude", "review"}, wf.ReviewCommand)
}

func TestResolveReviewCommandPrefersConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("task.agents.codex.review_command", []string{"codex", "review", "--json"})

	cmd, err := ResolveReviewCommand("codex", "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"codex", "review", "--json"}, cmd)
}

func TestResolveReviewCommandBuiltinCodex(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd, err := ResolveReviewCommand("codex", "gpt-5", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"codex", "review", "--uncommitted", "-m", "gpt-5"}, cmd)
}

func TestNormalizeTaskAgent(t *testing.T) {
	assert.Equal(t, "claude", NormalizeTaskAgent("claude-code"))
	assert.Equal(t, "grok", NormalizeTaskAgent("grok"))
	assert.Equal(t, "codex", NormalizeTaskAgent("codex-cli"))
}
