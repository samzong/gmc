package task

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	now := time.Now().UTC()
	rec := Record{
		ID:        "demo-task",
		State:     TaskIntake,
		Source:    "demo",
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, store.CreateTask(rec))
	loaded, err := store.LoadTask("demo-task")
	require.NoError(t, err)
	assert.Equal(t, TaskIntake, loaded.State)
	want := filepath.Join(dir, "gmc-tasks", "tasks", "demo-task", "task.yaml")
	assert.FileExists(t, want)
}

func TestStoreAppendEvent(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	rec := Record{ID: "evt-task", State: TaskIntake, Source: "x", CreatedAt: time.Now().UTC()}
	require.NoError(t, store.CreateTask(rec))
	require.NoError(t, store.appendEvent("evt-task", Event{Type: EventTaskCreated}))
	events, err := store.listEvents("evt-task")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, EventTaskCreated, events[0].Type)
}
