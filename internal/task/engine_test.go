package task

import (
	"os"
	"os/exec"
	"path/filepath"
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
			rec, err := engine.CreateTask("model carry test")
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
