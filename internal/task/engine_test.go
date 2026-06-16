package task

import (
	"os"
	"os/exec"
	"strings"
	"testing"

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
	rec, err := engine.CreateTask("override test")
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

func newTestEngineWithGit(t *testing.T) (*Engine, *Store) {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))
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
