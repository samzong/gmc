package worktree

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddPRUsesAddWorktreeNaming(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	runGit(t, filepath.Join(repoDir, ".bare"), "remote", "add", "origin", initPRRemote(t, 42))
	chdir(t, filepath.Join(repoDir, "main"))

	client := NewClient(Options{})
	if _, err := client.AddPR(42, ""); err != nil {
		t.Fatalf("AddPR() error = %v", err)
	}

	prDir := filepath.Join(repoDir, "pr--42")
	status := runGit(t, prDir, "status", "--short", "--branch")
	if !strings.Contains(status, "## pr/42") {
		t.Fatalf("new worktree status = %q, want pr/42 branch", status)
	}
}

func TestAddPRUsesAddWorktreeNamingInNormalRepo(t *testing.T) {
	repoDir := initTestRepo(t)
	runGit(t, repoDir, "remote", "add", "origin", initPRRemote(t, 42))
	chdir(t, repoDir)

	client := NewClient(Options{})
	if _, err := client.AddPR(42, ""); err != nil {
		t.Fatalf("AddPR() error = %v", err)
	}

	prDir := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+"--pr--42")
	status := runGit(t, prDir, "status", "--short", "--branch")
	if !strings.Contains(status, "## pr/42") {
		t.Fatalf("new worktree status = %q, want pr/42 branch", status)
	}
}

func initPRRemote(t *testing.T, prNumber int) string {
	t.Helper()
	remoteDir := initTestRepo(t)
	runGit(t, remoteDir, "checkout", "-b", "feature/review")
	writeFile(t, filepath.Join(remoteDir, "review.txt"), "review")
	runGit(t, remoteDir, "add", ".")
	runGit(t, remoteDir, "commit", "-m", "review")
	runGit(t, remoteDir, "update-ref", fmt.Sprintf("refs/pull/%d/head", prNumber), "HEAD")
	return remoteDir
}
