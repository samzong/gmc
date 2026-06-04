package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitialAgentPrompt(t *testing.T) {
	task := Record{
		ID:     "t-1",
		Title:  "Review PR #510 (org/repo)",
		Source: "review pr https://github.com/org/repo/pull/510",
		PR:     "510",
	}
	prompt := InitialAgentPrompt(task)
	assert.Contains(t, prompt, "Review PR #510")
	assert.Contains(t, prompt, ".gmc/TASK.md")
}

func TestWriteTaskContextFile(t *testing.T) {
	dir := t.TempDir()
	task := Record{ID: "t-1", Source: "fix the bug", Title: "fix the bug"}
	attempt := AttemptRecord{ID: "attempt-1", Agent: "codex", Mode: "coding"}
	path, err := WriteTaskContextFile(dir, task, attempt)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "fix the bug")
	assert.Equal(t, filepath.Join(dir, TaskContextRelPath), path)
}

func TestAgentCommandCodexWithPrompt(t *testing.T) {
	args, err := AgentCommand("codex", "", "coding", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"codex", "do the task"}, args)
}
