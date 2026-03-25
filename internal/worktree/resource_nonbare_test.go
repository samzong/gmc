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

func TestSyncAllSharedResources_DoesNotRunHooks(t *testing.T) {
	repoDir := initTestRepo(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/test-sync-hooks", linkedWt, "main")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".env"), []byte("SECRET=123"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "gmc-share.yml"), []byte("shared:\n  - path: .env\n    strategy: copy\nhooks:\n  - cmd: printf 'hook-ran' > hook.txt\n"), 0o644))

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

	_, err = os.Stat(filepath.Join(linkedWt, "hook.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestLoadSharedConfig_FallsBackToLegacyRepoRootConfig(t *testing.T) {
	repoDir := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, legacySharedConfigYML), []byte("shared:\n  - path: .env\n    strategy: copy\n"), 0o644))

	client := NewClient(Options{})
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	cfg, configPath, err := client.LoadSharedConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Resources, 1)
	expectedPath, pathErr := filepath.EvalSymlinks(filepath.Join(repoDir, legacySharedConfigYML))
	if pathErr != nil {
		expectedPath = filepath.Join(repoDir, legacySharedConfigYML)
	}
	assert.Equal(t, expectedPath, configPath)
}

func TestNormalizeSharedResourcePath_RejectsAbsolutePathOutsideWorktree(t *testing.T) {
	repoDir := initTestRepo(t)
	client := NewClient(Options{})
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	_, err = client.NormalizeSharedResourcePath(filepath.Join(t.TempDir(), "outside.env"))
	require.Error(t, err)
}

func TestRemoveSharedResource_NormalizesPath(t *testing.T) {
	repoDir := initTestRepo(t)
	client := NewClient(Options{})
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "gmc-share.yml"), []byte("shared:\n  - path: config/.env\n    strategy: copy\n"), 0o644))

	_, err = client.RemoveSharedResource("config/../config/.env")
	require.NoError(t, err)

	cfg, _, err := client.LoadSharedConfig()
	require.NoError(t, err)
	assert.Empty(t, cfg.Resources)
}

func TestResolveWorktreePath_ErrorsOnAmbiguousBasename(t *testing.T) {
	repoDir := initTestRepo(t)
	wt1 := filepath.Join(t.TempDir(), "dup")
	wt2Parent := filepath.Join(t.TempDir(), "nested")
	require.NoError(t, os.MkdirAll(wt2Parent, 0o755))
	wt2 := filepath.Join(wt2Parent, "dup")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/dup-1", wt1, "main")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/dup-2", wt2, "main")

	client := NewClient(Options{})
	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	_, err = client.resolveWorktreePath("dup")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous worktree")
}
