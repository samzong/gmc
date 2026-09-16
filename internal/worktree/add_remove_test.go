package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveProtectedWorktree(t *testing.T) {
	repoDir := initTestRepo(t)

	featureDir := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+"--feature")
	runGit(t, repoDir, "worktree", "add", "-b", "feature", featureDir, "main")
	defer os.RemoveAll(featureDir)

	t.Chdir(featureDir)

	repoName := filepath.Base(repoDir)
	client := NewClient(Options{})
	_, err := client.Remove(repoName, RemoveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot remove protected worktree")
}

func TestAddWorktreeConfig(t *testing.T) {
	for _, scope := range []string{"repository", "global", "none"} {
		t.Run(scope, func(t *testing.T) {
			repoDir := initBareLayoutRepo(t)
			if scope == "repository" {
				runGit(t, filepath.Join(repoDir, ".bare"), "config", "extensions.worktreeConfig", "true")
				runGit(t, filepath.Join(repoDir, "main"), "config", "--worktree", "core.bare", "false")
			}
			if scope == "global" {
				globalConfig := filepath.Join(t.TempDir(), "global.gitconfig")
				writeFile(t, globalConfig, "[extensions]\n\tworktreeConfig = true\n")
				t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
			}
			t.Chdir(filepath.Join(repoDir, "main"))
			_, err := NewClient(Options{}).Add("feature", AddOptions{BaseBranch: "main"})
			require.NoError(t, err)
			featureDir := filepath.Join(repoDir, "feature")
			require.Contains(t, runGit(t, featureDir, "status", "--short", "--branch"), "## feature")
			if scope == "repository" {
				require.Equal(t, "false", strings.TrimSpace(runGit(t, featureDir, "config", "--worktree", "--bool", "core.bare")))
			}
		})
	}
}

func TestRemoveBatch(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	t.Chdir(mainDir)

	client := NewClient(Options{})
	bareDir := filepath.Join(repoDir, ".bare")

	names := []string{"wt-a", "wt-b", "wt-c"}
	for _, name := range names {
		dir := filepath.Join(repoDir, name)
		runGit(t, bareDir, "worktree", "add", "-b", name, dir, "main")
	}
	client.InvalidateList()

	result := client.RemoveBatch(names, RemoveOptions{Force: true, DeleteBranch: true})

	require.Empty(t, result.Failed)
	assert.Len(t, result.Succeeded, 3)

	for _, name := range names {
		dir := filepath.Join(repoDir, name)
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("worktree %s still exists", name)
		}
	}

	for _, name := range names {
		if _, err := client.runner.Run("-C", bareDir, "rev-parse", "--verify", "refs/heads/"+name); err == nil {
			t.Errorf("branch %s still exists", name)
		}
	}
}

func TestRemoveBatchPartialFailure(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	t.Chdir(mainDir)

	client := NewClient(Options{})
	bareDir := filepath.Join(repoDir, ".bare")

	dir := filepath.Join(repoDir, "wt-good")
	runGit(t, bareDir, "worktree", "add", "-b", "wt-good", dir, "main")
	client.InvalidateList()

	result := client.RemoveBatch([]string{"wt-good", "wt-nonexistent"}, RemoveOptions{Force: true})

	require.NotEmpty(t, result.Failed)
	assert.Contains(t, result.Failed, "wt-nonexistent")

	assert.Len(t, result.Succeeded, 0)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("wt-good should NOT be removed when batch validation fails (fail-fast)")
	}
}

func TestRemoveBatchDryRun(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	t.Chdir(mainDir)

	client := NewClient(Options{})
	bareDir := filepath.Join(repoDir, ".bare")

	dir := filepath.Join(repoDir, "wt-dry")
	runGit(t, bareDir, "worktree", "add", "-b", "wt-dry", dir, "main")
	client.InvalidateList()

	result := client.RemoveBatch([]string{"wt-dry"}, RemoveOptions{DryRun: true, DeleteBranch: true})

	require.Empty(t, result.Failed)
	assert.Len(t, result.Succeeded, 1)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("DryRun should not actually remove the worktree")
	}

	assert.NotEmpty(t, result.Report.Events)
}

func BenchmarkRemoveBatch(b *testing.B) {
	for range b.N {
		b.StopTimer()
		repo := initBareLayoutRepo(b)
		b.Chdir(filepath.Join(repo, "main"))
		names := make([]string, 5)
		for j := range names {
			names[j] = fmt.Sprintf("bench-%d", j)
			runGit(b, filepath.Join(repo, ".bare"), "worktree", "add", "-b", names[j], filepath.Join(repo, names[j]), "main")
		}
		client := NewClient(Options{})
		b.StartTimer()
		result := client.RemoveBatch(names, RemoveOptions{Force: true, DeleteBranch: true})
		b.StopTimer()
		require.Empty(b, result.Failed)
	}
}

func TestRemovalModes(t *testing.T) {
	for _, mode := range []string{"single", "batch"} {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dry=%t", mode, dryRun), func(t *testing.T) {
				repo := initBareLayoutRepo(t)
				t.Chdir(filepath.Join(repo, "main"))
				client := NewClient(Options{})
				_, err := client.Add("feature", AddOptions{})
				require.NoError(t, err)
				before, err := client.ListCached()
				require.NoError(t, err)
				opts := RemoveOptions{DeleteBranch: true, DryRun: dryRun}
				var report Report
				if mode == "single" {
					report, err = client.Remove("feature", opts)
					require.NoError(t, err)
				} else {
					result := client.RemoveBatch([]string{"feature"}, opts)
					require.Empty(t, result.Failed)
					report = result.Report
				}
				after, err := client.ListCached()
				require.NoError(t, err)
				if dryRun {
					require.Equal(t, before, after)
					require.Contains(t, report.Events, Event{EventWarn, "Would delete branch: feature"})
				} else {
					require.Len(t, after, len(before)-1)
					require.Equal(t, []Event{
						{EventWarn, "Removed worktree 'feature'"},
						{EventWarn, "Deleted branch 'feature'"},
					}, report.Events)
				}
				require.Equal(t, dryRun, client.gitRefExists(client.repoDir, "refs/heads/feature"))
			})
		}
	}
}
