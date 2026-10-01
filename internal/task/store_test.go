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
	assert.FileExists(t, filepath.Join(store.root, "tasks", "t-demo", "task.yaml"))
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

func TestStoreRejectsTaskRefsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(victim, 0o755))
	decoy := filepath.Join(victim, "task.yaml")
	require.NoError(t, os.WriteFile(decoy, []byte("source: OUTSIDE_STORE_ROOT\n"), 0o600))

	escapes := []string{
		"../../victim",
		"..",
		"../victim",
		"a/../../victim",
		".",
		"./",
		"sub/..",
		"anything/..",
	}
	for _, ref := range escapes {
		_, err := store.ResolveTaskID(ref)
		require.Error(t, err, ref)
		assert.ErrorIs(t, err, ErrInvalidTaskID, ref)

		_, err = store.LoadTask(ref)
		assert.ErrorIs(t, err, ErrInvalidTaskID, ref)

		assert.ErrorIs(t, store.RemoveTask(ref), ErrInvalidTaskID, ref)
	}

	_, err := store.ResolveTaskID("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
	assert.ErrorIs(t, store.RemoveTask(""), ErrInvalidTaskID)

	assert.FileExists(t, decoy, "a traversal reference must not delete outside the store")
	assert.DirExists(t, victim)
}

func TestStoreRejectsTaskRootItself(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	taskRoot := filepath.Join(root, "gmc-tasks", "tasks")
	require.NoError(t, os.MkdirAll(taskRoot, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(taskRoot, "task.yaml"), []byte("id: .\n"), 0o600))
	require.NoError(t, store.CreateTask(Record{ID: "t-one", State: TaskNew, Source: "x", CreatedAt: time.Now().UTC()}))

	for _, ref := range []string{"", ".", "./", "sub/.."} {
		assert.ErrorIs(t, store.RemoveTask(ref), ErrInvalidTaskID, ref)
	}

	assert.DirExists(t, taskRoot, "the task root must survive")
	assert.FileExists(t, filepath.Join(taskRoot, "t-one", "task.yaml"))
}

func TestStoreAllowsNestedLookingRefsInsideRoot(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.CreateTask(Record{ID: "t-demo", State: TaskNew, Source: "d", CreatedAt: time.Now().UTC()}))

	id, err := store.ResolveTaskID("t-demo")
	require.NoError(t, err)
	assert.Equal(t, "t-demo", id)

	_, err = store.LoadTask("sub/../t-demo")
	require.NoError(t, err)
}

func TestStoreEventsRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.CreateTask(Record{ID: "t-ev", State: TaskNew, Source: "x", CreatedAt: time.Now().UTC()}))

	events, err := store.LoadEvents("t-ev")
	require.NoError(t, err)
	assert.Empty(t, events)

	require.NoError(t, store.AppendEvent(EventRecord{Type: EventTaskCreated, TaskID: "t-ev"}))
	require.NoError(t, store.AppendEvent(EventRecord{Type: EventRunStarted, TaskID: "t-ev", RunID: "r-1", Node: "plan"}))

	events, err = store.LoadEvents("t-ev")
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, EventTaskCreated, events[0].Type)
	assert.Equal(t, EventRunStarted, events[1].Type)
	assert.Equal(t, "r-1", events[1].RunID)
	assert.False(t, events[0].Time.IsZero())
}

func TestStoreEventsMissingTaskDir(t *testing.T) {
	store := NewStore(t.TempDir())
	events, err := store.LoadEvents("t-missing")
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestStoreRunsRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.CreateTask(Record{ID: "t-runs", State: TaskNew, Source: "x", CreatedAt: time.Now().UTC()}))

	runs, err := store.LoadRuns("t-runs")
	require.NoError(t, err)
	assert.Empty(t, runs)

	base := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.SaveRun(RunRecord{
		ID: "r-b", TaskID: "t-runs", Status: RunStatusPassed, StartedAt: base.Add(time.Minute),
	}))
	require.NoError(t, store.SaveRun(RunRecord{ID: "r-c", TaskID: "t-runs", Status: RunStatusFailed, StartedAt: base}))
	require.NoError(t, store.SaveRun(RunRecord{ID: "r-a", TaskID: "t-runs", Status: RunStatusRunning, StartedAt: base}))

	runs, err = store.LoadRuns("t-runs")
	require.NoError(t, err)
	require.Len(t, runs, 3)
	assert.Equal(t, "r-a", runs[0].ID)
	assert.Equal(t, "r-c", runs[1].ID)
	assert.Equal(t, "r-b", runs[2].ID)
	assert.Equal(t, RunStatusFailed, runs[1].Status)

	sum, err := store.LoadSummary("t-runs")
	require.NoError(t, err)
	require.Len(t, sum.Runs, 3)
	assert.Equal(t, "r-a", sum.Runs[0].ID)
}

func TestStoreSummaryWithoutRunsDir(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.CreateTask(Record{ID: "t-legacy", State: TaskNew, Source: "x", CreatedAt: time.Now().UTC()}))
	require.NoError(t, store.SaveAttempt(AttemptRecord{ID: "attempt-1", TaskID: "t-legacy"}))

	sum, err := store.LoadSummary("t-legacy")
	require.NoError(t, err)
	assert.Empty(t, sum.Runs)
	require.NotNil(t, sum.Attempt)
}

func TestStoreLoadRunsSkipsCorruptFiles(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.CreateTask(Record{ID: "t-corrupt", State: TaskNew, Source: "x", CreatedAt: time.Now().UTC()}))
	require.NoError(t, store.SaveRun(RunRecord{
		ID: "r-ok", TaskID: "t-corrupt", Status: RunStatusPassed, StartedAt: time.Now().UTC(),
	}))
	runsDir := filepath.Join(store.root, "tasks", "t-corrupt", "runs")
	require.NoError(t, os.WriteFile(filepath.Join(runsDir, "r-bad.yaml"), []byte("{{{not yaml"), 0o644))

	runs, err := store.LoadRuns("t-corrupt")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, "r-ok", runs[0].ID)

	sum, err := store.LoadSummary("t-corrupt")
	require.NoError(t, err)
	require.Len(t, sum.Runs, 1)
	require.NotEmpty(t, sum.Warnings)
	assert.Contains(t, sum.Warnings[0], "r-bad.yaml")
}

func TestStoreRunLogPath(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.CreateTask(Record{ID: "t-log", State: TaskNew, Source: "x", CreatedAt: time.Now().UTC()}))

	path, err := store.RunLogPath("t-log", RunLogRelPath("r-1", "stdout"))
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.Equal(t, filepath.Join("logs", "r-1.stdout.log"), filepath.FromSlash(RunLogRelPath("r-1", "stdout")))
	assert.Equal(t, filepath.Join(store.root, "tasks", "t-log", "logs", "r-1.stdout.log"), path)
}

func TestEngineCreateTaskFromFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "todo.md")
	require.NoError(t, os.WriteFile(source, []byte("# Todo\n\nShip it."), 0o644))

	engine := NewEngine(NewStore(dir), nil)
	rec, _, err := engine.CreateTask(source)
	require.NoError(t, err)
	assert.Equal(t, TaskNew, rec.State)
	assert.Equal(t, source, rec.SourceFile)
	assert.Contains(t, rec.Source, "Ship it")
}

func TestEngineAdvanceBeforeStart(t *testing.T) {
	engine := NewEngine(NewStore(t.TempDir()), nil)
	rec, _, err := engine.CreateTask("demo")
	require.NoError(t, err)

	_, err = engine.Advance(AdvanceOptions{TaskID: rec.ID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has not started a workflow")
}

func TestEngineRemoveTaskWithoutAttempt(t *testing.T) {
	store := NewStore(t.TempDir())
	engine := NewEngine(store, nil)
	rec, _, err := engine.CreateTask("demo")
	require.NoError(t, err)

	require.NoError(t, engine.Remove(rec.ID, RemoveOptions{}))
	_, err = store.LoadTask(rec.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestEngineRemoveTaskWithMissingWorktree(t *testing.T) {
	engine, store := newTestEngineWithGit(t)
	rec, _, err := engine.CreateTask("orphan worktree")
	require.NoError(t, err)
	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID:       "attempt-1",
		TaskID:   rec.ID,
		Worktree: filepath.Join(t.TempDir(), "missing-worktree"),
		Branch:   "_task/" + rec.ID + "/1",
	}))

	require.NoError(t, engine.Remove(rec.ID, RemoveOptions{Force: true}))
	_, err = store.LoadTask(rec.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestWriteYAMLPreservesPermissions(t *testing.T) {
	store := NewStore(t.TempDir())
	require.NoError(t, store.CreateTask(Record{ID: "t-x", State: TaskNew, Source: "x"}))
	require.NoError(t, store.SaveRun(RunRecord{
		ID: "r-1", TaskID: "t-x", Kind: RunKindCommand, Status: RunStatusRunning,
	}))

	dir, err := store.taskDir("t-x")
	require.NoError(t, err)
	runPath := filepath.Join(dir, "runs", "r-1.yaml")
	info, err := os.Stat(runPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	require.NoError(t, os.Chmod(runPath, 0o600))
	require.NoError(t, store.SaveRun(RunRecord{
		ID: "r-1", TaskID: "t-x", Kind: RunKindCommand, Status: RunStatusPassed,
	}))
	info, err = os.Stat(runPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
