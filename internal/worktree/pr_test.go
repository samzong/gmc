package worktree

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddPRUsesAddWorktreeNaming(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(fmt.Sprintf("bare=%t", bare), func(t *testing.T) {
			var repo, current, target string
			if bare {
				root := initBareLayoutRepo(t)
				repo, current = filepath.Join(root, ".bare"), filepath.Join(root, "main")
				target = filepath.Join(root, "pr--42")
			} else {
				repo = initTestRepo(t)
				current, target = repo, repo+"--pr--42"
			}
			runGit(t, repo, "remote", "add", "origin", initPRRemote(t, 42))
			t.Chdir(current)
			_, err := NewClient(Options{}).AddPR(42, "")
			require.NoError(t, err)
			require.Contains(t, runGit(t, target, "status", "--short", "--branch"), "## pr/42")
		})
	}
}

func initPRRemote(t *testing.T, prNumber int) string {
	t.Helper()
	remoteDir := initTestRepo(t)
	runGit(t, remoteDir, "checkout", "-b", "feature/review")
	writeFile(t, filepath.Join(remoteDir, "review.txt"), "review")
	commitFiles(t, remoteDir, "review", ".")
	runGit(t, remoteDir, "update-ref", fmt.Sprintf("refs/pull/%d/head", prNumber), "HEAD")
	return remoteDir
}
