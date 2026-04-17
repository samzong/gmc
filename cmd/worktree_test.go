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
	cfgPath := filepath.Join(repoDir, ".git", "gmc-share.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("hooks:\n  - cmd: echo ok\n"), 0o644))

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

var execCommand = exec.Command

func TestRemoveAll_SkipsProtected(t *testing.T) {
	repoDir := initCmdTestRepo(t)

	feat1 := filepath.Join(repoDir, "feat-1")
	feat2 := filepath.Join(repoDir, "feat-2")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feat-1", feat1, "main")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feat-2", feat2, "main")

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

	oldAll := wtAll
	oldForce := wtForce
	oldDelete := wtDeleteBranch
	oldDry := wtDryRun
	defer func() {
		wtAll = oldAll
		wtForce = oldForce
		wtDeleteBranch = oldDelete
		wtDryRun = oldDry
	}()

	wtAll = true
	wtForce = false
	wtDeleteBranch = true
	wtDryRun = false

	client := worktree.NewClient(worktree.Options{})
	err = runWorktreeRemove(client, nil)
	require.NoError(t, err)

	_, err = os.Stat(feat1)
	assert.True(t, os.IsNotExist(err), "feat-1 should be removed")
	_, err = os.Stat(feat2)
	assert.True(t, os.IsNotExist(err), "feat-2 should be removed")

	_, err = os.Stat(repoDir)
	assert.NoError(t, err, "main worktree (repoDir) must survive --all")

	remaining, err := client.List()
	require.NoError(t, err)
	var mainFound bool
	for _, wt := range remaining {
		if wt.Branch == "feat-1" || wt.Branch == "feat-2" {
			t.Errorf("branch %s should have been deleted", wt.Branch)
		}
		if wt.Branch == "main" {
			mainFound = true
		}
	}
	assert.True(t, mainFound, "main branch worktree must still exist")
}

func TestRemoveAllMutuallyExclusiveWithArgs(t *testing.T) {
	oldAll := wtAll
	defer func() { wtAll = oldAll }()
	wtAll = true

	err := wtRemoveCmd.Args(wtRemoveCmd, []string{"some-worktree"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestRemoveRequiresArgsOrAll(t *testing.T) {
	oldAll := wtAll
	defer func() { wtAll = oldAll }()
	wtAll = false

	err := wtRemoveCmd.Args(wtRemoveCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires at least 1 arg")
}
