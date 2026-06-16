package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCommandsRegistered(t *testing.T) {
	require.NotNil(t, taskCmd)
	assert.Equal(t, "task", taskCmd.Use)
	assert.Equal(t, "worktree", taskCmd.GroupID)

	names := make([]string, 0, len(taskCmd.Commands()))
	for _, c := range taskCmd.Commands() {
		names = append(names, c.Name())
	}
	assert.ElementsMatch(t, []string{
		"advance",
		"add",
		"attach",
		"list",
		"rm",
		"show",
		"start",
		"webui",
	}, names)

	listCmd, _, err := taskCmd.Find([]string{"ls"})
	require.NoError(t, err)
	assert.Equal(t, "list", listCmd.Name())
}

func TestCompleteTaskAgents(t *testing.T) {
	items, directive := completeTaskAgents(nil, nil, "c")
	assert.Contains(t, items, "codex")
	assert.Contains(t, items, "cursor-agent")
	assert.NotContains(t, items, "claude")
	assert.NotContains(t, items, "custom")
	assert.NotZero(t, directive)
}

func TestValidateTaskAddArgs(t *testing.T) {
	old := taskAddFile
	t.Cleanup(func() { taskAddFile = old })

	taskAddFile = ""
	require.NoError(t, validateTaskAddArgs(taskAddCmd, []string{"fix it"}))
	require.Error(t, validateTaskAddArgs(taskAddCmd, nil))

	taskAddFile = "todo.md"
	require.NoError(t, validateTaskAddArgs(taskAddCmd, nil))
	err := validateTaskAddArgs(taskAddCmd, []string{"fix it"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--file")
}
