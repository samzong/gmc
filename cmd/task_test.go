package cmd

import (
	"testing"

	"github.com/samzong/gmc/internal/task"
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
		"attach",
		"create",
		"list",
		"mark",
		"rm",
		"show",
		"start",
	}, names)

	listCmd, _, err := taskCmd.Find([]string{"ls"})
	require.NoError(t, err)
	assert.Equal(t, "list", listCmd.Name())
}

func TestTaskStates(t *testing.T) {
	assert.Equal(t, []string{"new", "plan", "code", "review", "ship"}, task.StateValues())
	assert.True(t, task.ValidState("ship"))
	assert.False(t, task.ValidState("done"))
}

func TestCompleteTaskAgents(t *testing.T) {
	items, directive := completeTaskAgents(nil, nil, "c")
	assert.Contains(t, items, "codex")
	assert.Contains(t, items, "cursor-agent")
	assert.NotContains(t, items, "claude")
	assert.NotContains(t, items, "custom")
	assert.NotZero(t, directive)
}

func TestValidateTaskCreateArgs(t *testing.T) {
	old := taskCreateFile
	t.Cleanup(func() { taskCreateFile = old })

	taskCreateFile = ""
	require.NoError(t, validateTaskCreateArgs(taskCreateCmd, []string{"fix it"}))
	require.Error(t, validateTaskCreateArgs(taskCreateCmd, nil))

	taskCreateFile = "todo.md"
	require.NoError(t, validateTaskCreateArgs(taskCreateCmd, nil))
	err := validateTaskCreateArgs(taskCreateCmd, []string{"fix it"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--file")
}
