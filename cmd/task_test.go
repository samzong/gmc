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
}

func TestTaskStates(t *testing.T) {
	assert.Equal(t, []string{"new", "plan", "code", "review", "ship"}, task.StateValues())
	assert.True(t, task.ValidState("ship"))
	assert.False(t, task.ValidState("done"))
}
