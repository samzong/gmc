package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/samzong/gmc/internal/exitcode"
	"github.com/samzong/gmc/internal/task"
	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
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
		"run",
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

func TestValidateTaskRunArgs(t *testing.T) {
	parse := func(args ...string) error {
		cmd := &cobra.Command{Use: "run"}
		require.NoError(t, cmd.Flags().Parse(args))
		return validateTaskRunArgs(cmd, cmd.Flags().Args())
	}

	require.NoError(t, parse("1", "--", "go", "test", "./..."))
	require.NoError(t, parse("t-abc", "--", "true"))

	err := parse("1", "go", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--")

	err = parse("--", "go", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task id")

	err = parse("1", "--")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command")

	err = parse("a", "b", "--", "go")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")

	require.Error(t, parse())
}

func newTaskRunTestEngine(t *testing.T) *task.Engine {
	t.Helper()
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	store, err := task.OpenStore(worktree.NewClient(worktree.Options{}))
	require.NoError(t, err)
	require.NoError(t, store.CreateTask(task.Record{
		ID: "t-run", State: "code", Source: "x", CurrentNode: "code", CreatedAt: time.Now().UTC(),
	}))
	require.NoError(t, store.SaveAttempt(task.AttemptRecord{
		ID: "attempt-1", TaskID: "t-run", Worktree: repo,
	}))
	return task.NewEngine(store, worktree.NewClient(worktree.Options{}))
}

func TestRunTaskRunPropagatesExitCode(t *testing.T) {
	engine := newTaskRunTestEngine(t)
	out, errw := new(bytes.Buffer), new(bytes.Buffer)
	withWriters(t, out, errw)
	setTestValue(t, &outputFlag.value, "text")

	err := runTaskRun(engine, 1, []string{"t-run", "sh", "-c", "echo hi; exit 3"})
	var ecErr *exitcode.Error
	require.True(t, errors.As(err, &ecErr))
	assert.Equal(t, 3, ecErr.Code)
	assert.Contains(t, ecErr.Message, "run ")
	assert.Contains(t, ecErr.Message, "(exit 3)")
	assert.Contains(t, ecErr.Message, "logs:")
	assert.Contains(t, out.String(), "hi")
	assert.NotContains(t, errw.String(), "Run ")
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRunTaskRunJSONStreamSplit(t *testing.T) {
	engine := newTaskRunTestEngine(t)
	out, errw := new(bytes.Buffer), new(lockedBuffer)
	withWriters(t, out, errw)
	setTestValue(t, &outputFlag.value, "json")

	err := runTaskRun(engine, 1, []string{"t-run", "sh", "-c", "echo out1; echo err1 1>&2"})
	require.NoError(t, err)

	var run task.RunRecord
	require.NoError(t, json.Unmarshal(out.Bytes(), &run))
	assert.Equal(t, task.RunStatusPassed, run.Status)
	assert.Equal(t, task.RunKindCommand, run.Kind)
	assert.Contains(t, errw.String(), "out1")
	assert.Contains(t, errw.String(), "err1")
}

func TestValidateTaskAddArgs(t *testing.T) {
	setTestValue(t, &taskAddFile, "")
	require.NoError(t, validateTaskAddArgs(taskAddCmd, []string{"fix it"}))
	require.Error(t, validateTaskAddArgs(taskAddCmd, nil))

	taskAddFile = "todo.md"
	require.NoError(t, validateTaskAddArgs(taskAddCmd, nil))
	err := validateTaskAddArgs(taskAddCmd, []string{"fix it"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--file")
}
