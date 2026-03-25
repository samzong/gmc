package cmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWorktreeDefault_ShowsWorktreesInNonBareRepo(t *testing.T) {
	repoDir := initCmdTestRepo(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feature/demo", linkedWt, "main")

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(linkedWt))

	var out bytes.Buffer
	oldOut := outWriterFunc
	oldErr := errWriterFunc
	outWriterFunc = func() io.Writer { return &out }
	errWriterFunc = func() io.Writer { return &out }
	defer func() {
		outWriterFunc = oldOut
		errWriterFunc = oldErr
	}()

	client := worktree.NewClient(worktree.Options{})
	cmd := &cobra.Command{Use: "wt"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List all worktrees"})

	err = runWorktreeDefault(client, cmd)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "Current Worktrees:")
	assert.Contains(t, output, "feature/demo")
	assert.NotContains(t, output, "not using the bare worktree pattern")
}

func TestWtHookRemoveCmd_RejectsTrailingCharactersInIndex(t *testing.T) {
	repoDir := initCmdTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "gmc-share.yml"), []byte("hooks:\n  - cmd: echo ok\n"), 0o644))

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	var out bytes.Buffer
	oldOut := outWriterFunc
	oldErr := errWriterFunc
	outWriterFunc = func() io.Writer { return &out }
	errWriterFunc = func() io.Writer { return &out }
	defer func() {
		outWriterFunc = oldOut
		errWriterFunc = oldErr
	}()

	err = wtHookRemoveCmd.RunE(wtHookRemoveCmd, []string{"1abc"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hook index")

	client := worktree.NewClient(worktree.Options{})
	cfg, _, err := client.LoadSharedConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Hooks, 1)
}

func initCmdTestRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	runGitCmd(t, repoDir, "init", "-b", "main")
	runGitCmd(t, repoDir, "config", "user.name", "Test User")
	runGitCmd(t, repoDir, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("init"), 0o644))
	runGitCmd(t, repoDir, "add", ".")
	runGitCmd(t, repoDir, "commit", "-m", "init")
	return repoDir
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := execCommand("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}

var execCommand = func(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
