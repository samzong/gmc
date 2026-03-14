package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSharedConfig_UsesGitCommonDirInNonBareWorktree(t *testing.T) {
	repoDir := initTestRepo(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/test-share-config", linkedWt, "main")

	client := NewClient(Options{})
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(linkedWt))

	cfg, configPath, err := client.LoadSharedConfig()
	require.NoError(t, err)
	assert.Empty(t, cfg.Resources)
	expectedCommonDir := strings.TrimSpace(runGit(t, linkedWt, "rev-parse", "--git-common-dir"))
	if !filepath.IsAbs(expectedCommonDir) {
		expectedCommonDir = filepath.Join(linkedWt, expectedCommonDir)
	}
	assert.Equal(t, filepath.Join(expectedCommonDir, "gmc-share.yml"), configPath)
}

func TestSyncAllSharedResources_WorksFromNonBareWorktreeRepo(t *testing.T) {
	repoDir := initTestRepo(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/test-sync-share", linkedWt, "main")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".env"), []byte("SECRET=123"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "gmc-share.yml"), []byte("shared:\n  - path: .env\n    strategy: copy\n"), 0o644))

	client := NewClient(Options{})
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(linkedWt))

	_, err = client.SyncAllSharedResources()
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(linkedWt, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET=123", string(data))
}
