package cmd

import (
	"bytes"
	"fmt"
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

	t.Chdir(linkedWt)

	var out bytes.Buffer
	withWriters(t, &out, &out)

	client := worktree.NewClient(worktree.Options{})

	err := runWorktreeList(client, true)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "Current Worktrees:")
	assert.Contains(t, output, "feature/demo")
	assert.NotContains(t, output, "not using the bare worktree pattern")
}

func TestWtHookRemoveCmd_UnknownIDPreservesHooks(t *testing.T) {
	repoDir := initCmdTestRepo(t)
	cfgPath := filepath.Join(repoDir, ".git", "gmc-share.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("hooks:\n  - cmd: echo ok\n"), 0o644))

	t.Chdir(repoDir)

	var out bytes.Buffer
	withWriters(t, &out, &out)

	err := wtHookRemoveCmd.RunE(wtHookRemoveCmd, []string{"1abc"})
	require.Error(t, err)

	client := worktree.NewClient(worktree.Options{})
	cfg, _, err := client.LoadSharedConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Hooks, 1)
}

func initCmdTestRepo(t *testing.T) string {
	t.Helper()
	repoDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
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
	require.NoError(t, err, "git %v: %s", args, output)
	return string(output)
}

var execCommand = exec.Command

func TestRemoveAll_SkipsProtected(t *testing.T) {
	repoDir := initCmdTestRepo(t)

	feat1 := filepath.Join(repoDir, "feat-1")
	feat2 := filepath.Join(repoDir, "feat-2")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feat-1", feat1, "main")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feat-2", feat2, "main")

	t.Chdir(repoDir)

	var out bytes.Buffer
	withWriters(t, &out, &out)

	setTestValue(t, &wtAll, true)
	setTestValue(t, &wtForce, false)
	setTestValue(t, &wtDeleteBranch, true)
	setTestValue(t, &wtDryRun, false)

	client := worktree.NewClient(worktree.Options{})
	err := runWorktreeRemove(client, nil)
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
	setTestValue(t, &wtAll, true)

	err := wtRemoveCmd.Args(wtRemoveCmd, []string{"some-worktree"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestRemoveRequiresArgsOrAll(t *testing.T) {
	setTestValue(t, &wtAll, false)

	err := wtRemoveCmd.Args(wtRemoveCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires at least 1 arg")
}

func TestWtAddPRArgs(t *testing.T) {
	tests := []struct {
		name    string
		pr      string
		args    []string
		base    string
		sync    bool
		wantErr string
	}{
		{name: "accepts pr", pr: "42"},
		{name: "rejects zero", pr: "0", wantErr: "greater than 0"},
		{name: "rejects names", pr: "42", args: []string{"feature"}, wantErr: "mutually exclusive with worktree names"},
		{name: "rejects base", pr: "42", base: "main", wantErr: "mutually exclusive with -b/--base"},
		{name: "rejects sync", pr: "42", sync: true, wantErr: "mutually exclusive with --sync"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetWtAddState(t)
			cmd := &cobra.Command{Use: "add"}
			cmd.Flags().IntVar(&wtAddPR, "pr", 0, "")
			require.NoError(t, cmd.Flags().Set("pr", tt.pr))
			wtBaseBranch = tt.base
			wtAddSync = tt.sync

			err := wtAddCmd.Args(cmd, tt.args)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPrReviewHasNoRemoteFlag(t *testing.T) {
	assert.Nil(t, wtPrReviewCmd.Flags().Lookup("remote"))
}

func TestRunWorktreeAddPRCreatesPRWorktree(t *testing.T) {
	resetWtAddState(t)
	repoDir := initCmdTestRepo(t)
	runGitCmd(t, repoDir, "remote", "add", "origin", initCmdPRRemote(t, 42))

	t.Chdir(repoDir)

	wtAddPR = 42
	client := worktree.NewClient(worktree.Options{})
	require.NoError(t, runWorktreeAdd(client, nil))

	prDir := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+"--pr--42")
	status := runGitCmd(t, prDir, "status", "--short", "--branch")
	assert.Contains(t, status, "## pr/42")
}

func resetWtAddState(t *testing.T) {
	t.Helper()
	setTestValue(t, &wtBaseBranch, "")
	setTestValue(t, &wtAddSync, false)
	setTestValue(t, &wtAddPR, 0)
}

func initCmdPRRemote(t *testing.T, prNumber int) string {
	t.Helper()
	remoteDir := initCmdTestRepo(t)
	runGitCmd(t, remoteDir, "checkout", "-b", "feature/review")
	require.NoError(t, os.WriteFile(filepath.Join(remoteDir, "review.txt"), []byte("review"), 0o644))
	runGitCmd(t, remoteDir, "add", ".")
	runGitCmd(t, remoteDir, "commit", "-m", "review")
	runGitCmd(t, remoteDir, "update-ref", fmt.Sprintf("refs/pull/%d/head", prNumber), "HEAD")
	return remoteDir
}
