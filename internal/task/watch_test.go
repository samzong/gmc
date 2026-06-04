package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchNoSession(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-watch"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskRunning, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID: "attempt-1", TaskID: id, State: AttemptRunning, CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	err := engine.Watch(id, "", 0, SessionCoding)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tmux session")
}
