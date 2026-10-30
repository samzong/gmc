package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListDetailsPreservesOrderAndDiffs(t *testing.T) {
	repo := initTestRepo(t)
	client := NewClient(Options{Dir: repo})
	paths := []Info{{Path: repo}, {Path: filepath.Join(t.TempDir(), "missing")}}
	for i, branch := range []string{"one", "two", "three", "four", "five"} {
		path := filepath.Join(t.TempDir(), branch)
		runGit(t, repo, "worktree", "add", "-b", branch, path, "main")
		require.NoError(t, os.WriteFile(filepath.Join(path, "change.txt"), []byte(branch+"\n"), 0o600))
		runGit(t, path, "add", "change.txt")
		paths = append(paths, Info{Path: path})
		if i == 0 {
			runGit(t, path, "commit", "-m", "Add change")
		}
	}
	details, err := client.ListDetails(paths, "")
	require.NoError(t, err)
	require.Len(t, details, len(paths))
	for i, wt := range paths {
		assert.Equal(t, client.GetWorktreeStatus(wt.Path), details[i].Status)
		base, baseErr := client.ResolveDiffBaseForWorktree(wt.Path, "")
		if baseErr != nil {
			assert.Nil(t, details[i].DiffStat)
			continue
		}
		stat, statErr := client.WorktreeDiffStat(wt.Path, base)
		require.NoError(t, statErr)
		require.NotNil(t, details[i].DiffStat)
		assert.Equal(t, stat, *details[i].DiffStat)
	}
	assert.Equal(t, "clean", details[2].Status)
	assert.Equal(t, 1, details[2].DiffStat.Files)
	assert.Equal(t, "1 file changed", details[3].Status)
}

func TestListDetailsExplicitBaseErrorOrder(t *testing.T) {
	client := NewClient(Options{Dir: t.TempDir()})
	paths := []Info{{Path: filepath.Join(t.TempDir(), "first")}, {Path: filepath.Join(t.TempDir(), "second")}}
	_, want := client.WorktreeDiffStat(paths[0].Path, "unknown-base")
	require.Error(t, want)
	_, err := client.ListDetails(paths, "unknown-base")
	require.EqualError(t, err, want.Error())

	details, err := client.ListDetails(nil, "")
	require.NoError(t, err)
	assert.Empty(t, details)
}
