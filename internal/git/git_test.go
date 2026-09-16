package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRepo(t *testing.T) *Client {
	t.Helper()
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
		{"config", "commit.gpgsign", "false"},
		{"config", "core.hooksPath", t.TempDir()},
	} {
		runGitCommand(t, args...)
	}
	return NewClient(Options{})
}

func runGitCommand(t *testing.T, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), output)
	return strings.TrimSpace(string(output))
}

func TestCommitAndTag(t *testing.T) {
	client := setupRepo(t)
	assert.True(t, client.IsGitRepository())
	require.NoError(t, client.CheckGitRepository())
	require.NoError(t, os.WriteFile("test.txt", []byte("Hello World"), 0o644))
	require.NoError(t, client.AddAll())
	files, err := client.ParseStagedFiles()
	require.NoError(t, err)
	assert.Equal(t, []string{"test.txt"}, files)
	diff, err := client.GetStagedDiff()
	require.NoError(t, err)
	assert.Contains(t, diff, "test.txt")
	assert.Contains(t, diff, "Hello World")
	stats, err := client.GetStagedDiffStats()
	require.NoError(t, err)
	assert.Contains(t, stats, "test.txt")
	require.NoError(t, client.Commit("test: safe commit in temp repo"))
	commits, err := client.GetCommitsSinceTag("")
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, "test: safe commit in temp repo", commits[0].Message)
	assert.Equal(t, "Test User", commits[0].Author)
	assert.NotEmpty(t, commits[0].Hash)
	assert.NotEmpty(t, commits[0].Date)
	tag, err := client.GetLatestTag()
	require.NoError(t, err)
	assert.Empty(t, tag)
	require.NoError(t, client.CreateAndSwitchBranch("feature/test-branch"))
	assert.Equal(t, "feature/test-branch", runGitCommand(t, "branch", "--show-current"))
	require.Error(t, client.CreateAndSwitchBranch("feature/test-branch"))
	require.Error(t, client.CreateAndSwitchBranch("invalid..branch"))
	require.NoError(t, client.CreateAnnotatedTag("v0.1.0", "Release v0.1.0"))
	tag, err = client.GetLatestTag()
	require.NoError(t, err)
	assert.Equal(t, "v0.1.0", tag)
	commits, err = client.GetCommitsSinceTag(tag)
	require.NoError(t, err)
	assert.Empty(t, commits)
	require.NoError(t, os.WriteFile("feature.txt", []byte("new feature"), 0o644))
	require.NoError(t, client.AddAll())
	require.NoError(t, client.Commit("feat: add new capability", "-m", "Additional context for feature"))
	commits, err = client.GetCommitsSinceTag(tag)
	require.NoError(t, err)
	require.Len(t, commits, 1)
	assert.Equal(t, "feat: add new capability", commits[0].Message)
	assert.Equal(t, "Additional context for feature", commits[0].Body)
	commits, err = client.GetCommitsSinceTag("v9.9.9")
	require.NoError(t, err)
	assert.Len(t, commits, 2)
}

func TestSelectiveFiles(t *testing.T) {
	client := setupRepo(t)
	require.NoError(t, os.Mkdir("pkg", 0o755))
	require.NoError(t, os.WriteFile("pkg/tracked.txt", []byte("tracked"), 0o644))
	require.NoError(t, client.AddAll())
	require.NoError(t, client.Commit("initial commit"))
	require.NoError(t, os.WriteFile("pkg/draft.txt", []byte("draft"), 0o644))
	files, err := client.ResolveFiles([]string{"pkg", "pkg/tracked.txt"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"pkg/tracked.txt", "pkg/draft.txt"}, files)
	require.NoError(t, os.WriteFile("pkg/tracked.txt", []byte("modified"), 0o644))
	staged, modified, untracked, err := client.CheckFileStatus(files)
	require.NoError(t, err)
	assert.Empty(t, staged)
	assert.Equal(t, []string{"pkg/tracked.txt"}, modified)
	assert.Equal(t, []string{"pkg/draft.txt"}, untracked)
	require.NoError(t, client.StageFiles(files))
	staged, modified, untracked, err = client.CheckFileStatus(files)
	require.NoError(t, err)
	assert.ElementsMatch(t, files, staged)
	assert.Empty(t, modified)
	assert.Empty(t, untracked)
	diff, err := client.GetFilesDiff([]string{"pkg/draft.txt"})
	require.NoError(t, err)
	assert.Contains(t, diff, "+draft")
	assert.NotContains(t, diff, "tracked.txt")
	require.NoError(t, client.CommitFiles("feat: draft", []string{"pkg/draft.txt"}, "-s"))
	remaining, err := client.ParseStagedFiles()
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg/tracked.txt"}, remaining)
	assert.Contains(t, runGitCommand(t, "log", "-1", "--format=%B"), "Signed-off-by: Test User <test@example.com>")
	runGitCommand(t, "rm", "pkg/draft.txt")
	files, err = client.ResolveFiles([]string{"pkg/draft.txt"})
	require.NoError(t, err)
	assert.Equal(t, []string{"pkg/draft.txt"}, files)
	_, err = client.ResolveFiles([]string{"missing.txt"})
	require.Error(t, err)
}

func TestOutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	client := NewClient(Options{})
	assert.False(t, client.IsGitRepository())
	for name, call := range map[string]func() error{
		"check":  client.CheckGitRepository,
		"add":    client.AddAll,
		"diff":   func() error { _, err := client.GetStagedDiff(); return err },
		"files":  func() error { _, err := client.ParseStagedFiles(); return err },
		"commit": func() error { return client.Commit("test message") },
		"branch": func() error { return client.CreateAndSwitchBranch("test-branch") },
	} {
		t.Run(name, func(t *testing.T) { assert.ErrorIs(t, call(), ErrNotGitRepo) })
	}
}

func TestCommitAllowsNonTempDirInTestEnv(t *testing.T) {
	cacheDir, err := os.UserCacheDir()
	require.NoError(t, err)
	repoDir, err := os.MkdirTemp(cacheDir, "gmc_safe_repo_")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(repoDir)) })
	if strings.Contains(filepath.Clean(repoDir), "/tmp/") {
		t.Skip("cache directory is a temporary directory")
	}
	t.Chdir(repoDir)
	t.Setenv("GO_TEST_ENV", "1")
	runGitCommand(t, "init")
	runGitCommand(t, "config", "user.name", "Test")
	runGitCommand(t, "config", "user.email", "test@test.com")
	runGitCommand(t, "config", "commit.gpgsign", "false")
	runGitCommand(t, "config", "core.hooksPath", t.TempDir())
	require.NoError(t, os.WriteFile("test.txt", []byte("test"), 0o644))
	client := NewClient(Options{Verbose: true})
	require.NoError(t, client.AddAll())
	require.NoError(t, client.Commit("test: safe commit outside temp patterns"))
}
