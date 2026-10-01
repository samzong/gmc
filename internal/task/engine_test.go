package task

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordTmuxSessionStoresCommand(t *testing.T) {
	attempt := AttemptRecord{Agent: "codex"}
	profile := TmuxProfile{Session: "sess-1", Socket: gmcTmuxSocket}
	command := []string{"custom-cmd", "--flag", "prompt text"}

	attempt = recordTmuxSession(attempt, "plan", profile, command)

	require.Len(t, attempt.TmuxSessions, 1)
	assert.Equal(t, "plan", attempt.TmuxSessions[0].Node)
	assert.Equal(t, shellJoin(command), attempt.TmuxSessions[0].Command)
	assert.Equal(t, "sess-1", attempt.TmuxSessions[0].Session)
}

func TestEngineStartCommandOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	oldStarter := tmuxSessionStarter
	t.Cleanup(func() { tmuxSessionStarter = oldStarter })

	var gotSession, gotWorkdir string
	var gotCommand []string
	tmuxSessionStarter = func(session, workdir string, command []string) (TmuxProfile, error) {
		gotSession = session
		gotWorkdir = workdir
		gotCommand = append([]string(nil), command...)
		return TmuxProfile{Session: session, Socket: gmcTmuxSocket}, nil
	}

	engine, store := newTestEngineWithGit(t)
	rec, _, err := engine.CreateTask("override test")
	require.NoError(t, err)

	override := "custom-agent --yolo"
	sum, err := engine.Start(StartOptions{
		TaskID:  rec.ID,
		Agent:   "codex",
		Command: override,
	})
	require.NoError(t, err)
	require.NotNil(t, sum.Attempt)

	assert.True(t, strings.HasPrefix(gotCommand[0], "custom-agent"))
	assert.NotEmpty(t, gotSession)
	assert.NotEmpty(t, gotWorkdir)

	stored, err := store.LoadTask(rec.ID)
	require.NoError(t, err)
	assert.Equal(t, DefaultWorkflowConfig().Workflows[DefaultWorkflowName].Nodes["plan"].Command,
		stored.WorkflowSnapshot.Nodes["plan"].Command)

	attempt, err := store.LoadAttempt(rec.ID)
	require.NoError(t, err)
	require.Len(t, attempt.TmuxSessions, 1)
	assert.Contains(t, attempt.TmuxSessions[0].Command, "custom-agent")
}

func TestEngineAdvanceModelCarry(t *testing.T) {
	tests := []struct {
		name      string
		workflow  string
		wantAgent string
		wantModel bool
	}{
		{name: "different agent drops model", wantAgent: "grok", wantModel: false},
		{
			name: "same agent keeps model",
			workflow: `version: 1
start: plan
nodes:
  plan:
    agent: codex
    prompt: Plan it.
    next: code
  code:
    agent: codex
    prompt: Code it.
    next: done
`,
			wantAgent: "codex",
			wantModel: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xdg := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", xdg)
			t.Setenv("HOME", t.TempDir())
			if tt.workflow != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(xdg, "gmc"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(xdg, "gmc", "workflow.yaml"), []byte(tt.workflow), 0o644))
			}

			oldStarter := tmuxSessionStarter
			t.Cleanup(func() { tmuxSessionStarter = oldStarter })
			var launched [][]string
			tmuxSessionStarter = func(session, _ string, command []string) (TmuxProfile, error) {
				launched = append(launched, append([]string(nil), command...))
				return TmuxProfile{Session: session, Socket: gmcTmuxSocket}, nil
			}

			engine, _ := newTestEngineWithGit(t)
			rec, _, err := engine.CreateTask("model carry test")
			require.NoError(t, err)
			_, err = engine.Start(StartOptions{TaskID: rec.ID, Model: "gpt-5"})
			require.NoError(t, err)
			sum, err := engine.Advance(AdvanceOptions{TaskID: rec.ID})
			require.NoError(t, err)

			require.Len(t, launched, 2)
			assert.Equal(t, []string{"codex", "-m", "gpt-5"}, launched[0][:3])
			code := launched[1]
			assert.Equal(t, tt.wantAgent, code[0])
			assert.Equal(t, tt.wantAgent, sum.Attempt.Agent)
			if tt.wantModel {
				assert.Equal(t, []string{"-m", "gpt-5"}, code[1:3])
				assert.Equal(t, "gpt-5", sum.Attempt.Model)
			} else {
				assert.NotContains(t, code, "gpt-5")
				assert.Empty(t, sum.Attempt.Model)
			}
		})
	}
}

func stubTmuxStarter(t *testing.T) {
	t.Helper()
	oldStarter := tmuxSessionStarter
	t.Cleanup(func() { tmuxSessionStarter = oldStarter })
	tmuxSessionStarter = func(session, workdir string, command []string) (TmuxProfile, error) {
		return TmuxProfile{Session: session, Socket: gmcTmuxSocket}, nil
	}
}

func eventTypes(events []EventRecord) []string {
	types := make([]string, 0, len(events))
	for _, ev := range events {
		types = append(types, ev.Type)
	}
	return types
}

func TestEngineStartRecordsRunAndEvents(t *testing.T) {
	stubTmuxStarter(t)
	engine, store := newTestEngineWithGit(t)
	rec, _, err := engine.CreateTask("demo task")
	require.NoError(t, err)

	sum, err := engine.Start(StartOptions{TaskID: rec.ID, Agent: "codex"})
	require.NoError(t, err)
	require.NotNil(t, sum.Attempt)
	node := sum.Task.CurrentNode

	runs, err := store.LoadRuns(rec.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	run := runs[0]
	assert.Equal(t, RunKindAgent, run.Kind)
	assert.Equal(t, RunRuntimeTmux, run.Runtime)
	assert.Equal(t, RunStatusRunning, run.Status)
	assert.Equal(t, node, run.Node)
	assert.Equal(t, sum.Attempt.ID, run.AttemptID)
	assert.Equal(t, sum.Attempt.Worktree, run.Cwd)
	assert.NotEmpty(t, run.Session)
	assert.NotEmpty(t, run.Socket)
	assert.NotEmpty(t, run.Command)
	assert.True(t, run.EndedAt == nil)

	events, err := store.LoadEvents(rec.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{EventTaskCreated, EventTaskStarted, EventRunStarted}, eventTypes(events))
	assert.Equal(t, node, events[1].Node)
	assert.Equal(t, run.ID, events[2].RunID)
}

func TestEngineAdvanceRecordsRunAndEvents(t *testing.T) {
	stubTmuxStarter(t)
	engine, store := newTestEngineWithGit(t)
	rec, _, err := engine.CreateTask("demo task")
	require.NoError(t, err)
	started, err := engine.Start(StartOptions{TaskID: rec.ID, Agent: "codex"})
	require.NoError(t, err)
	prev := started.Task.CurrentNode

	advanced, err := engine.Advance(AdvanceOptions{TaskID: rec.ID})
	require.NoError(t, err)
	next := advanced.Task.CurrentNode
	require.NotEqual(t, "done", next)

	runs, err := store.LoadRuns(rec.ID)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	assert.Equal(t, next, runs[1].Node)
	assert.Equal(t, RunKindAgent, runs[1].Kind)

	events, err := store.LoadEvents(rec.ID)
	require.NoError(t, err)
	require.Len(t, events, 5)
	assert.Equal(t, EventTaskAdvanced, events[3].Type)
	assert.Equal(t, next, events[3].Node)
	assert.Equal(t, "from "+prev+" to "+next, events[3].Message)
	assert.Equal(t, EventRunStarted, events[4].Type)
	assert.Equal(t, runs[1].ID, events[4].RunID)
}

func TestEngineAdvanceToDoneOnlyEmitsAdvanced(t *testing.T) {
	stubTmuxStarter(t)
	engine, store := newTestEngineWithGit(t)
	rec, _, err := engine.CreateTask("demo task")
	require.NoError(t, err)
	_, err = engine.Start(StartOptions{TaskID: rec.ID, Agent: "codex"})
	require.NoError(t, err)

	sum, err := engine.Advance(AdvanceOptions{TaskID: rec.ID, ToNode: "done"})
	require.NoError(t, err)
	assert.Equal(t, "done", sum.Task.State)

	runs, err := store.LoadRuns(rec.ID)
	require.NoError(t, err)
	assert.Len(t, runs, 1)

	events, err := store.LoadEvents(rec.ID)
	require.NoError(t, err)
	last := events[len(events)-1]
	assert.Equal(t, EventTaskAdvanced, last.Type)
	assert.Equal(t, "done", last.Node)
}

func newRunTestEngine(t *testing.T) (*Engine, *Store, string) {
	t.Helper()
	store := NewStore(t.TempDir())
	engine := NewEngine(store, nil)
	workDir := t.TempDir()
	require.NoError(t, store.CreateTask(Record{
		ID:          "t-run",
		State:       "code",
		Source:      "x",
		CurrentNode: "code",
		CreatedAt:   time.Now().UTC(),
	}))
	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID:       "attempt-1",
		TaskID:   "t-run",
		Worktree: workDir,
	}))
	return engine, store, workDir
}

func TestEngineRunHeadlessFailedExit(t *testing.T) {
	engine, store, workDir := newRunTestEngine(t)

	var stdout, stderr bytes.Buffer
	res, err := engine.Run(RunOptions{
		TaskID:  "t-run",
		Command: []string{"sh", "-c", "echo out; echo err 1>&2; exit 3"},
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	require.NoError(t, err)
	run := res.Run
	assert.Equal(t, RunStatusFailed, run.Status)
	require.NotNil(t, run.ExitCode)
	assert.Equal(t, 3, *run.ExitCode)
	assert.Equal(t, RunKindCommand, run.Kind)
	assert.Equal(t, RunRuntimeHeadless, run.Runtime)
	assert.Equal(t, "code", run.Node)
	assert.Equal(t, "attempt-1", run.AttemptID)
	assert.Equal(t, workDir, run.Cwd)
	require.NotNil(t, run.EndedAt)
	assert.Contains(t, stdout.String(), "out")
	assert.Contains(t, stderr.String(), "err")

	stdoutPath, err := store.RunLogPath("t-run", run.Stdout)
	require.NoError(t, err)
	data, err := os.ReadFile(stdoutPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "out")
	assert.Equal(t, filepath.Join("logs", run.ID+".stdout.log"), filepath.FromSlash(run.Stdout))

	stderrPath, err := store.RunLogPath("t-run", run.Stderr)
	require.NoError(t, err)
	data, err = os.ReadFile(stderrPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "err")

	stored, err := store.LoadRuns("t-run")
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.NotNil(t, stored[0].ExitCode)
	assert.Equal(t, 3, *stored[0].ExitCode)

	events, err := store.LoadEvents("t-run")
	require.NoError(t, err)
	require.Equal(t, []string{EventRunStarted, EventRunFinished}, eventTypes(events))
	assert.Equal(t, run.ID, events[0].RunID)
	assert.Equal(t, RunStatusFailed, events[1].Status)
	require.NotNil(t, events[1].ExitCode)
	assert.Equal(t, 3, *events[1].ExitCode)
}

func TestEngineRunHeadlessPassed(t *testing.T) {
	engine, _, _ := newRunTestEngine(t)

	res, err := engine.Run(RunOptions{TaskID: "t-run", Command: []string{"true"}})
	require.NoError(t, err)
	run := res.Run
	assert.Equal(t, RunStatusPassed, run.Status)
	assert.Nil(t, run.ExitCode)
}

func TestEngineRunHeadlessStartFailure(t *testing.T) {
	engine, _, _ := newRunTestEngine(t)

	res, err := engine.Run(RunOptions{TaskID: "t-run", Command: []string{"gmc-nonexistent-binary-xyz"}})
	require.NoError(t, err)
	run := res.Run
	assert.Equal(t, RunStatusFailed, run.Status)
	assert.NotEmpty(t, run.Error)
	assert.Nil(t, run.ExitCode)
}

func TestEngineRunRequiresAttemptAndWorktree(t *testing.T) {
	store := NewStore(t.TempDir())
	engine := NewEngine(store, nil)
	require.NoError(t, store.CreateTask(Record{ID: "t-noatt", State: TaskNew, Source: "x", CreatedAt: time.Now().UTC()}))

	_, err := engine.Run(RunOptions{TaskID: "t-noatt", Command: []string{"true"}})
	require.ErrorIs(t, err, ErrNoAttempt)

	require.NoError(t, store.SaveAttempt(AttemptRecord{
		ID:       "attempt-1",
		TaskID:   "t-noatt",
		Worktree: filepath.Join(t.TempDir(), "gone"),
	}))
	_, err = engine.Run(RunOptions{TaskID: "t-noatt", Command: []string{"true"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree")

	_, err = engine.Run(RunOptions{TaskID: "t-noatt"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command")
}

type failAfterWriter struct {
	remaining int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, errors.New("broken pipe")
	}
	w.remaining -= len(p)
	return len(p), nil
}

func TestEngineRunSwallowsConsoleWriteErrors(t *testing.T) {
	engine, store, _ := newRunTestEngine(t)

	var stderr bytes.Buffer
	res, err := engine.Run(RunOptions{
		TaskID:  "t-run",
		Command: []string{"sh", "-c", "yes abcdefghij | head -c 100000"},
		Stdout:  &failAfterWriter{remaining: 10},
		Stderr:  &stderr,
	})
	require.NoError(t, err)
	run := res.Run
	assert.Equal(t, RunStatusPassed, run.Status)

	stdoutPath, err := store.RunLogPath("t-run", run.Stdout)
	require.NoError(t, err)
	data, err := os.ReadFile(stdoutPath)
	require.NoError(t, err)
	assert.Len(t, data, 100000)
}

func TestEngineRunSignalTerminatedChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix signal semantics")
	}
	engine, _, _ := newRunTestEngine(t)

	res, err := engine.Run(RunOptions{TaskID: "t-run", Command: []string{"sh", "-c", "kill -INT $$"}})
	require.NoError(t, err)
	run := res.Run
	assert.Equal(t, RunStatusFailed, run.Status)
	require.NotNil(t, run.ExitCode)
	assert.Equal(t, 130, *run.ExitCode)
	assert.Contains(t, run.Error, "interrupt")
}

func TestEngineRunBackgroundChildDoesNotHang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires sh")
	}
	engine, store, _ := newRunTestEngine(t)

	start := time.Now()
	res, err := engine.Run(RunOptions{
		TaskID:  "t-run",
		Command: []string{"sh", "-c", "sleep 3 & echo started"},
	})
	require.NoError(t, err)
	run := res.Run
	assert.Less(t, time.Since(start), 3*time.Second)
	assert.Equal(t, RunStatusPassed, run.Status)

	stdoutPath, err := store.RunLogPath("t-run", run.Stdout)
	require.NoError(t, err)
	data, err := os.ReadFile(stdoutPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "started")
}

func newTestEngineWithGit(t *testing.T) (*Engine, *Store) {
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
	return NewEngine(store, wt), store
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}
