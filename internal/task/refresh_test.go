package task

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLedgerTestEnv(t *testing.T) (*Engine, *Store, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	runGit(t, "init")
	runGit(t, "config", "user.email", "test@test")
	runGit(t, "config", "user.name", "test")
	runGit(t, "commit", "--allow-empty", "-m", "init")

	wt := worktree.NewClient(worktree.Options{})
	store, err := OpenStore(wt)
	require.NoError(t, err)
	return NewEngine(store, wt), store, dir
}

func stubTmuxRuntime(t *testing.T, alive, attached map[string]bool, live []string) {
	t.Helper()
	oldAlive, oldAttached, oldList, oldUp := tmuxSessionAlive, tmuxSessionAttached, tmuxListSessions, tmuxReachable
	t.Cleanup(func() {
		tmuxSessionAlive = oldAlive
		tmuxSessionAttached = oldAttached
		tmuxListSessions = oldList
		tmuxReachable = oldUp
	})
	tmuxSessionAlive = func(p TmuxProfile) bool { return alive[p.Session] }
	tmuxSessionAttached = func(p TmuxProfile) bool { return attached[p.Session] }
	tmuxListSessions = func(string) ([]string, error) { return live, nil }
	tmuxReachable = func() bool { return true }
}

func stubTmuxKill(t *testing.T, killed *[]string) {
	t.Helper()
	oldKill := killTmuxSession
	t.Cleanup(func() { killTmuxSession = oldKill })
	killTmuxSession = func(p TmuxProfile) error {
		*killed = append(*killed, p.Session)
		return nil
	}
}

func seedTask(t *testing.T, store *Store, rec Record, attempt *AttemptRecord, runs ...RunRecord) {
	t.Helper()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	if rec.Source == "" {
		rec.Source = "x"
	}
	require.NoError(t, store.CreateTask(rec))
	if attempt != nil {
		require.NoError(t, store.SaveAttempt(*attempt))
	}
	for _, run := range runs {
		if run.StartedAt.IsZero() {
			run.StartedAt = time.Now().UTC()
		}
		require.NoError(t, store.SaveRun(run))
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, "-C", dir, "init")
	runGit(t, "-C", dir, "config", "user.email", "test@test")
	runGit(t, "-C", dir, "config", "user.name", "test")
	runGit(t, "-C", dir, "commit", "--allow-empty", "-m", "init")
}

func agentRun(id, taskID, node, session, status string) RunRecord {
	return RunRecord{
		ID:      id,
		TaskID:  taskID,
		Node:    node,
		Kind:    RunKindAgent,
		Runtime: RunRuntimeTmux,
		Command: []string{"codex"},
		Session: session,
		Socket:  gmcTmuxSocket,
		Status:  status,
	}
}

func TestRefreshMarksTmuxRunExited(t *testing.T) {
	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t, map[string]bool{"sess-1": false}, nil, nil)
	seedTask(t, store,
		Record{ID: "t-a", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-a", Worktree: dir},
		agentRun("r-1", "t-a", "code", "sess-1", RunStatusRunning),
	)

	results, err := engine.Refresh("t-a")
	require.NoError(t, err)
	require.Len(t, results, 1)
	res := results[0]
	assert.Equal(t, "t-a", res.TaskID)
	assert.Equal(t, "code", res.State)
	assert.Equal(t, "clean", res.WorktreeStatus)
	assert.Equal(t, "gone", res.Session)
	require.Equal(t, []string{"r-1: running → exited"}, res.Updated)

	runs, err := store.LoadRuns("t-a")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	assert.Equal(t, RunStatusExited, runs[0].Status)
	require.NotNil(t, runs[0].EndedAt)

	events, err := store.LoadEvents("t-a")
	require.NoError(t, err)
	last := events[len(events)-1]
	assert.Equal(t, EventRunExited, last.Type)
	assert.Equal(t, "r-1", last.RunID)
	assert.Equal(t, RunStatusExited, last.Status)
}

func TestRefreshLeavesLiveSessionRun(t *testing.T) {
	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t, map[string]bool{"sess-1": true}, nil, nil)
	seedTask(t, store,
		Record{ID: "t-a", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-a", Worktree: dir},
		agentRun("r-1", "t-a", "code", "sess-1", RunStatusRunning),
	)

	results, err := engine.Refresh("t-a")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "alive", results[0].Session)
	assert.Empty(t, results[0].Updated)

	runs, err := store.LoadRuns("t-a")
	require.NoError(t, err)
	assert.Equal(t, RunStatusRunning, runs[0].Status)
}

func TestRefreshMarksHeadlessRunLost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a finished child pid")
	}
	finished := exec.Command("true")
	require.NoError(t, finished.Run())
	deadPID := finished.Process.Pid

	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t, nil, nil, nil)
	seedTask(t, store,
		Record{ID: "t-a", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-a", Worktree: dir},
		RunRecord{
			ID: "r-dead", TaskID: "t-a", Node: "code", Kind: RunKindCommand,
			Runtime: RunRuntimeHeadless, PID: deadPID, Status: RunStatusRunning,
		},
		RunRecord{
			ID: "r-live", TaskID: "t-a", Node: "code", Kind: RunKindCommand,
			Runtime: RunRuntimeHeadless, PID: os.Getpid(), Status: RunStatusRunning,
		},
	)

	results, err := engine.Refresh("t-a")
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, []string{"r-dead: running → lost"}, results[0].Updated)

	runs, err := store.LoadRuns("t-a")
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.Equal(t, RunStatusLost, runs[0].Status)
	assert.Equal(t, RunStatusRunning, runs[1].Status)

	events, err := store.LoadEvents("t-a")
	require.NoError(t, err)
	last := events[len(events)-1]
	assert.Equal(t, EventRunLost, last.Type)
	assert.Equal(t, "r-dead", last.RunID)
}

func TestRefreshLeavesFinishedRuns(t *testing.T) {
	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t, map[string]bool{}, nil, nil)
	code := 3
	seedTask(t, store,
		Record{ID: "t-a", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-a", Worktree: dir},
		agentRun("r-pass", "t-a", "code", "sess-gone", RunStatusPassed),
		RunRecord{
			ID: "r-fail", TaskID: "t-a", Node: "code", Kind: RunKindCommand,
			Runtime: RunRuntimeHeadless, PID: 999999, Status: RunStatusFailed, ExitCode: &code,
		},
	)

	results, err := engine.Refresh("t-a")
	require.NoError(t, err)
	assert.Empty(t, results[0].Updated)

	runs, err := store.LoadRuns("t-a")
	require.NoError(t, err)
	assert.Equal(t, RunStatusPassed, runs[0].Status)
	assert.Equal(t, RunStatusFailed, runs[1].Status)
}

func TestRefreshWorktreeStatuses(t *testing.T) {
	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t, nil, nil, nil)

	cleanDir := t.TempDir()
	initGitRepo(t, cleanDir)
	missing := filepath.Join(t.TempDir(), "gone")
	seedTask(t, store,
		Record{ID: "t-miss", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-miss", Worktree: missing},
	)
	seedTask(t, store,
		Record{ID: "t-clean", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-clean", Worktree: cleanDir},
	)
	seedTask(t, store,
		Record{ID: "t-none", State: TaskNew},
		nil,
	)
	seedTask(t, store,
		Record{ID: "t-dirty", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-dirty", Worktree: dir},
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644))

	results, err := engine.Refresh("")
	require.NoError(t, err)
	byID := map[string]RefreshResult{}
	for _, res := range results {
		byID[res.TaskID] = res
	}
	assert.Equal(t, "missing", byID["t-miss"].WorktreeStatus)
	assert.Equal(t, "clean", byID["t-clean"].WorktreeStatus)
	assert.Equal(t, "none", byID["t-none"].WorktreeStatus)
	assert.Equal(t, "none", byID["t-none"].Session)
	assert.NotEqual(t, "clean", byID["t-dirty"].WorktreeStatus)
	assert.NotEqual(t, "missing", byID["t-dirty"].WorktreeStatus)
}

func TestGCDryRunRules(t *testing.T) {
	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t,
		map[string]bool{"sess-done": true, "sess-plan": true, "sess-code": true, "sess-att": true},
		map[string]bool{"sess-att": true},
		[]string{"sess-done", "sess-plan", "sess-code", "sess-att", "sess-orphan"},
	)
	var killed []string
	stubTmuxKill(t, &killed)

	seedTask(t, store,
		Record{ID: "t-done", State: "done", CurrentNode: "done"},
		&AttemptRecord{
			ID: "attempt-1", TaskID: "t-done", Worktree: filepath.Join(t.TempDir(), "gone"),
			TmuxSessions: []TmuxSessionRecord{{Node: "code", Session: "sess-done", Socket: gmcTmuxSocket}},
		},
		agentRun("r-1", "t-done", "code", "sess-done", RunStatusRunning),
	)
	seedTask(t, store,
		Record{ID: "t-live", State: "code", CurrentNode: "code"},
		&AttemptRecord{
			ID: "attempt-1", TaskID: "t-live", Worktree: dir,
			TmuxSessions: []TmuxSessionRecord{
				{Node: "plan", Session: "sess-plan", Socket: gmcTmuxSocket},
				{Node: "code", Session: "sess-code", Socket: gmcTmuxSocket},
			},
		},
		agentRun("r-2", "t-live", "plan", "sess-plan", RunStatusRunning),
		agentRun("r-3", "t-live", "code", "sess-code", RunStatusRunning),
	)
	seedTask(t, store,
		Record{ID: "t-att", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-att", Worktree: dir},
		agentRun("r-4", "t-att", "code", "sess-att", RunStatusRunning),
	)
	cleanDir := t.TempDir()
	initGitRepo(t, cleanDir)
	seedTask(t, store,
		Record{ID: "t-suggest", State: "done", CurrentNode: "done"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-suggest", Worktree: cleanDir},
	)
	seedTask(t, store,
		Record{ID: "t-clean-live", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-clean-live", Worktree: cleanDir},
	)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644))
	// t-live and t-att share dir: both see dirty worktree.

	items, err := engine.GC(GCOptions{})
	require.NoError(t, err)
	assert.Empty(t, killed)

	byTarget := map[string]GCItem{}
	for _, item := range items {
		byTarget[item.Kind+":"+item.Target] = item
	}

	killDone := byTarget["tmux-session:sess-done"]
	assert.Equal(t, "kill", killDone.Action)
	assert.Equal(t, "task is done", killDone.Reason)

	killOld := byTarget["tmux-session:sess-plan"]
	assert.Equal(t, "kill", killOld.Action)
	assert.Equal(t, "node plan finished; task is at code", killOld.Reason)

	keepLive := byTarget["tmux-session:sess-code"]
	assert.Equal(t, "keep", keepLive.Action)
	assert.Equal(t, "task in progress at code", keepLive.Reason)

	keepAtt := byTarget["tmux-session:sess-att"]
	assert.Equal(t, "keep", keepAtt.Action)
	assert.Equal(t, "attached", keepAtt.Reason)

	orphan := byTarget["tmux-session:sess-orphan"]
	assert.Equal(t, "keep", orphan.Action)
	assert.Equal(t, "not referenced by any task", orphan.Reason)
	assert.Empty(t, orphan.TaskID)

	var missingItem, dirtyItem, suggestItem *GCItem
	for i := range items {
		if items[i].Kind != "worktree" {
			continue
		}
		switch items[i].TaskID {
		case "t-done":
			missingItem = &items[i]
		case "t-live":
			dirtyItem = &items[i]
		case "t-suggest":
			suggestItem = &items[i]
		}
	}
	require.NotNil(t, missingItem)
	assert.Equal(t, "keep", missingItem.Action)
	assert.Contains(t, missingItem.Reason, "gmc task rm t-done")
	require.NotNil(t, dirtyItem)
	assert.Equal(t, "keep", dirtyItem.Action)
	assert.Equal(t, "uncommitted changes", dirtyItem.Reason)
	require.NotNil(t, suggestItem)
	assert.Equal(t, "suggest", suggestItem.Action)
	assert.Contains(t, suggestItem.Reason, "gmc task rm t-suggest")

	for _, item := range items {
		assert.Empty(t, item.Result)
	}
	for _, item := range items {
		assert.NotEqual(t, "t-clean-live", item.TaskID)
	}
}

func TestGCApplyKillsOnlyKillItems(t *testing.T) {
	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t,
		map[string]bool{"sess-done": true, "sess-live": true},
		nil,
		[]string{"sess-done", "sess-live", "sess-orphan"},
	)
	var killed []string
	stubTmuxKill(t, &killed)

	seedTask(t, store,
		Record{ID: "t-done", State: "done", CurrentNode: "done"},
		&AttemptRecord{
			ID: "attempt-1", TaskID: "t-done", Worktree: dir,
			TmuxSessions: []TmuxSessionRecord{{Node: "code", Session: "sess-done", Socket: gmcTmuxSocket}},
		},
		agentRun("r-1", "t-done", "code", "sess-done", RunStatusRunning),
	)
	seedTask(t, store,
		Record{ID: "t-live", State: "code", CurrentNode: "code"},
		&AttemptRecord{
			ID: "attempt-1", TaskID: "t-live", Worktree: dir,
			TmuxSessions: []TmuxSessionRecord{{Node: "code", Session: "sess-live", Socket: gmcTmuxSocket}},
		},
		agentRun("r-2", "t-live", "code", "sess-live", RunStatusRunning),
	)

	items, err := engine.GC(GCOptions{Apply: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"sess-done"}, killed)

	for _, item := range items {
		switch item.Target {
		case "sess-done":
			assert.Equal(t, "killed", item.Result)
		default:
			assert.Empty(t, item.Result)
		}
	}

	events, err := store.LoadEvents("t-done")
	require.NoError(t, err)
	last := events[len(events)-1]
	assert.Equal(t, EventGCKilled, last.Type)
	assert.Equal(t, "sess-done", last.Message)
	assert.Equal(t, "t-done", last.TaskID)
}

func TestEngineRunWarningsWhenEventsUnwritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	engine, store, _ := newRunTestEngine(t)
	eventsPath, err := store.EventsPath("t-run")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(eventsPath, 0o755))

	res, err := engine.Run(RunOptions{
		TaskID:  "t-run",
		Command: []string{"sh", "-c", "exit 3"},
	})
	require.NoError(t, err)
	assert.Equal(t, RunStatusFailed, res.Run.Status)
	require.NotNil(t, res.Run.ExitCode)
	assert.Equal(t, 3, *res.Run.ExitCode)
	assert.Len(t, res.Warnings, 2)

	stored, err := store.LoadRuns("t-run")
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, RunStatusFailed, stored[0].Status)
}

func TestEngineCreateTaskEventWarning(t *testing.T) {
	engine, store := newTestEngineWithGit(t)
	old := storeAppendEvent
	t.Cleanup(func() { storeAppendEvent = old })
	storeAppendEvent = func(*Store, EventRecord) error {
		return errors.New("events.jsonl is a directory")
	}

	rec, warnings, err := engine.CreateTask("demo task")
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "events.jsonl")

	_, err = store.LoadTask(rec.ID)
	require.NoError(t, err)
}

func TestRefreshSkipsTmuxRunsWhenServerUnreachable(t *testing.T) {
	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t, map[string]bool{"sess-1": true}, nil, nil)
	tmuxReachable = func() bool { return false }
	seedTask(t, store,
		Record{ID: "t-a", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-a", Worktree: dir},
		agentRun("r-1", "t-a", "code", "sess-1", RunStatusRunning),
	)

	results, err := engine.Refresh("t-a")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "unknown", results[0].Session)
	assert.Empty(t, results[0].Updated)

	runs, err := store.LoadRuns("t-a")
	require.NoError(t, err)
	assert.Equal(t, RunStatusRunning, runs[0].Status)
}

func TestGCUsesAttemptLatestSession(t *testing.T) {
	engine, store, dir := newLedgerTestEnv(t)
	stubTmuxRuntime(t,
		map[string]bool{"sess-code": true, "sess-review": true},
		nil,
		[]string{"sess-code", "sess-review"},
	)
	seedTask(t, store,
		Record{ID: "t-r", State: "review", CurrentNode: "review"},
		&AttemptRecord{
			ID: "attempt-1", TaskID: "t-r", Worktree: dir,
			TmuxSessions: []TmuxSessionRecord{
				{Node: "code", Session: "sess-code", Socket: gmcTmuxSocket},
				{Node: "review", Session: "sess-review", Socket: gmcTmuxSocket},
			},
		},
		agentRun("r-1", "t-r", "code", "sess-code", RunStatusRunning),
	)

	items, err := engine.GC(GCOptions{})
	require.NoError(t, err)
	byTarget := map[string]GCItem{}
	for _, item := range items {
		byTarget[item.Target] = item
	}
	assert.Equal(t, "keep", byTarget["sess-review"].Action)
	assert.Equal(t, "task in progress at review", byTarget["sess-review"].Reason)
	assert.Equal(t, "kill", byTarget["sess-code"].Action)
}

func TestGCUnknownWorktreeStatus(t *testing.T) {
	engine, store, _ := newLedgerTestEnv(t)
	stubTmuxRuntime(t, nil, nil, nil)
	nonGit := t.TempDir()
	seedTask(t, store,
		Record{ID: "t-u", State: "code", CurrentNode: "code"},
		&AttemptRecord{ID: "attempt-1", TaskID: "t-u", Worktree: nonGit},
	)

	items, err := engine.GC(GCOptions{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "keep", items[0].Action)
	assert.Equal(t, "could not determine worktree status", items[0].Reason)
}

func TestTmuxSessionNamesUseExactMatch(t *testing.T) {
	assert.Equal(t, "=sess-2", tmuxTarget("sess-2"))
	if !tmuxAvailable() {
		t.Skip("tmux not installed")
	}
	socket := "gmc-test-" + strconv.Itoa(os.Getpid())
	profile := func(session string) TmuxProfile {
		return TmuxProfile{Session: session, Socket: socket}
	}
	tmux := func(args ...string) {
		full := append([]string{"-L", socket}, args...)
		require.NoError(t, exec.Command("tmux", full...).Run())
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
		tmpdir := os.Getenv("TMUX_TMPDIR")
		if tmpdir == "" {
			tmpdir = filepath.Join(os.TempDir(), "tmux-"+strconv.Itoa(os.Getuid()))
		}
		_ = os.Remove(filepath.Join(tmpdir, socket))
	})
	tmux("new-session", "-d", "-s", "sess-20", "sleep 60")

	assert.False(t, tmuxHasSession(profile("sess-2")), "prefix must not match sess-20")
	assert.True(t, tmuxHasSession(profile("sess-20")))

	require.NoError(t, KillTmuxSession(profile("sess-2")))
	assert.True(t, tmuxHasSession(profile("sess-20")), "killing sess-2 must not touch sess-20")
}

func TestTmuxSessionAttachedDetection(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not installed")
	}
	socket := "gmc-test-att-" + strconv.Itoa(os.Getpid())
	profile := TmuxProfile{Session: "sess-att", Socket: socket}
	require.NoError(t, exec.Command("tmux", "-L", socket,
		"new-session", "-d", "-s", "sess-att", "sleep 60").Run())
	require.NoError(t, exec.Command("tmux", "-L", socket,
		"new-session", "-d", "-s", "sess-att-x", "sleep 60").Run())
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
		tmpdir := os.Getenv("TMUX_TMPDIR")
		if tmpdir == "" {
			tmpdir = filepath.Join(os.TempDir(), "tmux-"+strconv.Itoa(os.Getuid()))
		}
		_ = os.Remove(filepath.Join(tmpdir, socket))
	})

	assert.False(t, tmuxSessionAttached(profile))
	assert.False(t, tmuxSessionAttached(TmuxProfile{Session: "sess-att-x", Socket: socket}))

	ctx, cancel := context.WithCancel(context.Background())
	ctl := exec.CommandContext(ctx, "tmux", "-L", socket, "-C", "attach-session", "-t", "=sess-att")
	stdin, err := ctl.StdinPipe()
	require.NoError(t, err)
	defer stdin.Close()
	require.NoError(t, ctl.Start())
	defer func() { _ = ctl.Wait() }()
	defer cancel()

	clientAttached := func() bool {
		out, err := exec.Command("tmux", "-L", socket, "list-clients", "-t", "=sess-att").Output()
		return err == nil && strings.TrimSpace(string(out)) != ""
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !clientAttached() {
		time.Sleep(50 * time.Millisecond)
	}
	if !clientAttached() {
		t.Skip("control client did not attach")
	}
	assert.True(t, tmuxSessionAttached(profile), "attached client must be detected")
	assert.False(t, tmuxSessionAttached(TmuxProfile{Session: "sess-att-x", Socket: socket}),
		"detached session sharing the name as a prefix must stay detached")
}
