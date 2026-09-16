package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t testing.TB) string {
	return initTestRepoWithBranch(t, "main")
}

func initTestRepoWithBranch(t testing.TB, branch string) string {
	t.Helper()
	repoDir := physicalTempDir(t)

	runGit(t, repoDir, "init", "-b", branch)
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	writeFile(t, filepath.Join(repoDir, "README.md"), "init")
	commitFiles(t, repoDir, "init", ".")

	return repoDir
}

func initBareRepo(t testing.TB) string {
	t.Helper()
	repoDir := physicalTempDir(t)
	runGit(t, repoDir, "init", "--bare")
	return repoDir
}

func runGit(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v\n%s", args, output)
	return string(output)
}

func writeFile(t testing.TB, path string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func readFile(t testing.TB, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func assertFileContent(t testing.TB, path, want string) {
	t.Helper()
	require.Equal(t, want, readFile(t, path))
}

func createPromoteCandidate(t *testing.T) (string, string, string) {
	t.Helper()
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	t.Chdir(mainDir)

	client := NewClient(Options{})
	_, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1})
	require.NoError(t, err)
	return repoDir, mainDir, filepath.Join(repoDir, ".dup-1")
}

func initBareLayoutRepo(t testing.TB) string {
	t.Helper()
	tmpDir := physicalTempDir(t)

	bareDir := filepath.Join(tmpDir, ".bare")
	runGit(t, tmpDir, "init", "--bare", bareDir)
	runGit(t, bareDir, "config", "user.name", "Test User")
	runGit(t, bareDir, "config", "user.email", "test@example.com")

	mainDir := filepath.Join(tmpDir, "main")
	runGit(t, bareDir, "worktree", "add", mainDir, "-b", "main")
	writeFile(t, filepath.Join(mainDir, "README.md"), "init")
	commitFiles(t, mainDir, "init", ".")

	return tmpDir
}

func initBareLayoutRepoWithWorktreeConfig(t testing.TB) string {
	t.Helper()
	repo := initBareLayoutRepo(t)
	runGit(t, filepath.Join(repo, ".bare"), "config", "extensions.worktreeConfig", "true")
	runGit(t, filepath.Join(repo, "main"), "config", "--worktree", "core.bare", "false")
	return repo
}

func physicalTempDir(t testing.TB) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return path
}

func commitFiles(t testing.TB, repo, message string, paths ...string) {
	t.Helper()
	runGit(t, repo, append([]string{"add"}, paths...)...)
	runGit(t, repo, "commit", "-m", message)
}

func assertMissing(t testing.TB, path string) {
	t.Helper()
	_, err := os.Lstat(path)
	assert.ErrorIs(t, err, os.ErrNotExist, path)
}
