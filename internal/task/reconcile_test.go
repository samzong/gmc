package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskStateFromAttempts(t *testing.T) {
	attempts := []AttemptRecord{{State: AttemptRunning}}
	assert.Equal(t, TaskRunning, taskStateFromAttempts(TaskNeedsHuman, attempts))
	assert.Equal(t, TaskReviewing, taskStateFromAttempts(TaskReviewing, attempts))

	attempts[0].State = AttemptHumanAttached
	assert.Equal(t, TaskNeedsHuman, taskStateFromAttempts(TaskRunning, attempts))
}

func TestDetachClearsNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-test-detach-state"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskNeedsHuman, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID: "a1", TaskID: id, State: AttemptRunning, CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	require.NoError(t, engine.refreshTaskState(id))
	rec, err := store.LoadTask(id)
	require.NoError(t, err)
	assert.Equal(t, TaskRunning, rec.State)
}

func TestDetachHumanAttachedClearsNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-test-detach-human"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskNeedsHuman, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID: "a1", TaskID: id, State: AttemptHumanAttached, CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	require.NoError(t, engine.Detach(id, ""))
	rec, err := store.LoadTask(id)
	require.NoError(t, err)
	assert.Equal(t, TaskRunning, rec.State)
	attempt, err := store.LoadAttempt(id, "a1")
	require.NoError(t, err)
	assert.Equal(t, AttemptRunning, attempt.State)
}
