package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTaskContextFile(t *testing.T) {
	dir := t.TempDir()
	rec := Record{ID: "t-1", State: TaskPlan, Source: "fix the bug", Title: "Fix bug"}
	attempt := AttemptRecord{ID: "attempt-1", Worktree: dir, Branch: "_task/t-1/1", Agent: "codex"}

	path, err := WriteTaskContextFile(dir, rec, attempt)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, TaskContextRelPath), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "Fix bug")
	assert.Contains(t, text, "fix the bug")
	assert.Contains(t, text, "_task/t-1/1")
}

func TestInitialAgentPrompt(t *testing.T) {
	rec := Record{Title: "Fix bug"}
	assert.Contains(t, InitialAgentPrompt(rec), ".gmc/TASK.md")
}

func TestAgentCommandCodex(t *testing.T) {
	args, err := AgentCommand("codex", "gpt-5", "coding", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"codex", "-m", "gpt-5", "do the task"}, args)
}

func TestAgentCommandGrok(t *testing.T) {
	args, err := AgentCommand("grok", "grok-4", "coding", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"grok", "-m", "grok-4", "do the task"}, args)
}

func TestAgentCommandCursorAgent(t *testing.T) {
	args, err := AgentCommand("cursor-agent", "gpt-5", "plan", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"cursor-agent", "--model", "gpt-5", "--mode", "plan", "do the task"}, args)
}

func TestAgentCommandOpencode(t *testing.T) {
	args, err := AgentCommand("opencode", "", "coding", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"opencode", "run", "do the task"}, args)
}

func TestAgentCommandUnsupported(t *testing.T) {
	_, err := AgentCommand("claude", "", "coding", "do the task")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "codex, grok, cursor-agent, or opencode")
}
