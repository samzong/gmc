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
	assert.Contains(t, names, "create")
	assert.Contains(t, names, "advance")
	assert.Contains(t, names, "gc")
	assert.Contains(t, names, "rm")
	assert.Contains(t, names, "watch")
	assert.NotContains(t, names, "run")
}

func TestParseSessionTarget(t *testing.T) {
	assert.Equal(t, task.SessionCoding, task.ParseSessionTarget("coding"))
	assert.Equal(t, task.SessionReview, task.ParseSessionTarget("review"))
}
