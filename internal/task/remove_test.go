package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveTask(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-test-archive"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskIntake, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	require.NoError(t, engine.Remove(id, RemoveOptions{Archive: true}))
	rec, err := store.LoadTask(id)
	require.NoError(t, err)
	assert.Equal(t, TaskArchived, rec.State)
}

func TestForceRemoveHumanAttached(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-test-detach-rm"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskNeedsHuman, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID: id + "-a1", TaskID: id, State: AttemptHumanAttached, CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	require.NoError(t, engine.Remove(id, RemoveOptions{Force: true}))
	_, err := os.Stat(filepath.Join(dir, "gmc-tasks", "tasks", id))
	assert.True(t, os.IsNotExist(err))
}

func TestArchiveBlocksHumanAttached(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-test-block"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskNeedsHuman, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID: "attempt-1", TaskID: id, State: AttemptHumanAttached, CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	err := engine.Remove(id, RemoveOptions{Archive: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gmc task detach")
}

func TestForceRemoveTask(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	id := "t-test-force"
	require.NoError(t, store.CreateTask(Record{
		ID: id, State: TaskIntake, Source: "x", CreatedAt: now, UpdatedAt: now,
	}))
	engine := NewEngine(store, nil)
	require.NoError(t, engine.Remove(id, RemoveOptions{Force: true}))
	_, err := os.Stat(filepath.Join(dir, "gmc-tasks", "tasks", id))
	assert.True(t, os.IsNotExist(err))
}
