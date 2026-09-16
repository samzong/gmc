package worktree

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeDiffStat(t *testing.T) {
	repoDir := initTestRepo(t)
	runGit(t, repoDir, "checkout", "-b", "feature/diff-stat")
	writeFile(t, filepath.Join(repoDir, "feature.txt"), "one\ntwo\n")
	commitFiles(t, repoDir, "add feature", "feature.txt")

	writeFile(t, filepath.Join(repoDir, "README.md"), "initial\nupdated\n")

	client := NewClient(Options{})
	stat, err := client.WorktreeDiffStat(repoDir, "main")
	require.NoError(t, err)

	assert.Equal(t, 2, stat.Files)
	assert.Equal(t, 4, stat.Insertions)
	assert.Equal(t, 1, stat.Deletions)
}

func TestParseDiffNumstatHandlesRename(t *testing.T) {
	output := []byte("1\t0\t\x00old.txt\x00new.txt\x00")
	stat := parseDiffNumstat(output)

	assert.Equal(t, 1, stat.Files)
	assert.Equal(t, 1, stat.Insertions)
	assert.Equal(t, 0, stat.Deletions)
}

func TestResolveDiffBaseForWorktree_UsesBranchUpstream(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "checkout", "-b", "feature/upstream-tracked")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	runGit(t, repoDir, "update-ref", "refs/heads/tracked-base", "HEAD")
	runGit(t, repoDir, "branch", "--set-upstream-to=tracked-base")

	client := NewClient(Options{})
	base, err := client.ResolveDiffBaseForWorktree(repoDir, "")
	require.NoError(t, err)
	assert.Equal(t, "tracked-base", base)
}

func TestResolveDiffBaseForWorktree_RemoteFallback(t *testing.T) {
	for _, detachedClient := range []bool{false, true} {
		t.Run(fmt.Sprintf("detached client=%t", detachedClient), func(t *testing.T) {
			repo := initTestRepo(t)
			runGit(t, repo, "checkout", "-b", "feature/no-upstream")
			runGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
			client := NewClient(Options{})
			if detachedClient {
				client.repoDir = t.TempDir()
			}
			base, err := client.ResolveDiffBaseForWorktree(repo, "")
			require.NoError(t, err)
			assert.Equal(t, "origin/main", base)
		})
	}
}
