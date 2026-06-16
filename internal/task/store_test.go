package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateAndLoadSummary(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Now().UTC()
	rec := Record{ID: "t-demo", State: TaskNew, Source: "demo", CreatedAt: now}

	require.NoError(t, store.CreateTask(rec))
	sum, err := store.LoadSummary("t-demo")
	require.NoError(t, err)

	assert.Equal(t, TaskNew, sum.Task.State)
	assert.Nil(t, sum.Attempt)
	assert.FileExists(t, filepath.Join(store.Root(), "tasks", "t-demo", "task.yaml"))
}

func TestStoreResolveTaskID(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Now().UTC()
	require.NoError(t, store.CreateTask(Record{ID: "t-alpha", State: TaskNew, Source: "a", CreatedAt: now}))
	require.NoError(t, store.CreateTask(Record{ID: "t-beta", State: TaskNew, Source: "b", CreatedAt: now}))

	id, err := store.ResolveTaskID("t-al")
	require.NoError(t, err)
	assert.Equal(t, "t-alpha", id)

	id, err = store.ResolveTaskID("1")
	require.NoError(t, err)
	assert.Equal(t, "t-alpha", id)

	id, err = store.ResolveTaskID("2")
	require.NoError(t, err)
	assert.Equal(t, "t-beta", id)

	_, err = store.ResolveTaskID("3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestEngineCreateTaskFromFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "todo.md")
	require.NoError(t, os.WriteFile(source, []byte("# Todo\n\nShip it."), 0o644))

	engine := NewEngine(NewStore(dir), nil)
	rec, err := engine.CreateTask(source)
	require.NoError(t, err)
	assert.Equal(t, TaskNew, rec.State)
	assert.Equal(t, source, rec.SourceFile)
	assert.Contains(t, rec.Source, "Ship it")
}

func TestEngineAdvanceBeforeStart(t *testing.T) {
	engine := NewEngine(NewStore(t.TempDir()), nil)
	rec, err := engine.CreateTask("demo")
	require.NoError(t, err)

	_, err = engine.Advance(AdvanceOptions{TaskID: rec.ID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has not started a workflow")
}

func TestEngineRemoveTaskWithoutAttempt(t *testing.T) {
	store := NewStore(t.TempDir())
	engine := NewEngine(store, nil)
	rec, err := engine.CreateTask("demo")
	require.NoError(t, err)

	require.NoError(t, engine.Remove(rec.ID, RemoveOptions{}))
	_, err = store.LoadTask(rec.ID)
	require.ErrorIs(t, err, ErrNotFound)
}
