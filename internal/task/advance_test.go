package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextTaskState(t *testing.T) {
	next, ok := NextTaskState(TaskRunning)
	assert.True(t, ok)
	assert.Equal(t, TaskReviewing, next)
	_, ok = NextTaskState(TaskIntake)
	assert.False(t, ok)
}

func TestAdvanceReviewingToVerifying(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-advance-r2v"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskReviewing, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	wt := filepath.Join(dir, "wt")
	require.NoError(t, os.MkdirAll(wt, 0o755))
	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID: "a1", TaskID: id, State: AttemptRunning, Worktree: wt,
		Agent: "codex", CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	sum, err := engine.Advance(AdvanceOptions{TaskID: id})
	require.NoError(t, err)
	assert.Equal(t, TaskVerifying, sum.Task.State)
}

func TestStartRequiresIntake(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-start-guard"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskRunning, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	_, err := engine.Start(StartOptions{TaskID: id, Agent: "codex"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intake")
}

func TestSummarizeLog(t *testing.T) {
	s := SummarizeLog("line one\n\nline two\n")
	assert.Contains(t, s, "line one")
	assert.Contains(t, s, "line two")
}
