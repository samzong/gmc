package worktree

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalPreparationPreservesConfigAndRepositoryOverrides(t *testing.T) {
	repoDir := initTestRepo(t)
	t.Chdir(repoDir)
	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	globalYAML := []byte("model: example-model\napi_key: fixture-only\nother:\n  enabled: true\n" +
		"worktree: &preparation\n  shared: []\n  hooks: []\npreparation_view: *preparation\n")
	configTarget := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configTarget, globalYAML, 0o600))
	require.NoError(t, os.Symlink(configTarget, globalPath))
	client := NewClient(Options{GlobalConfigPath: globalPath})
	_, err := client.AddGlobalSharedResource("**/node_modules", StrategySymlink)
	require.NoError(t, err)
	_, err = client.AddGlobalSharedResource(".local", StrategySymlink)
	require.NoError(t, err)
	_, err = client.AddGlobalHook(Hook{ID: "prepare", Cmd: "test -L .local && printf global > prepared"})
	require.NoError(t, err)
	_, err = client.AddGlobalHook(Hook{ID: "disabled", Cmd: "touch must-not-exist"})
	require.NoError(t, err)
	_, err = client.AddHook(Hook{ID: "prepare", Cmd: "test -L .local && printf local > prepared"})
	require.NoError(t, err)
	_, err = client.RemoveHookByID("disabled", false)
	require.NoError(t, err)
	_, err = client.RemoveSharedResource("web/node_modules")
	require.NoError(t, err)
	for _, dir := range []string{".local", "web/node_modules", "other/node_modules"} {
		require.NoError(t, os.MkdirAll(filepath.Join(repoDir, dir), 0o755))
	}
	target := t.TempDir()
	_, err = client.syncSharedResourcesToPath(target, true)
	require.NoError(t, err)
	prepared, err := os.ReadFile(filepath.Join(target, "prepared"))
	require.NoError(t, err)
	assert.Equal(t, "local", string(prepared))
	_, err = os.Readlink(filepath.Join(target, "other/node_modules"))
	require.NoError(t, err)
	_, err = os.Lstat(filepath.Join(target, "web/node_modules"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(target, "must-not-exist"))
	assert.True(t, os.IsNotExist(err))
	globalData, err := os.ReadFile(globalPath)
	require.NoError(t, err)
	assert.Contains(t, string(globalData), "api_key: fixture-only")
	assert.Contains(t, string(globalData), "example-model")
	assert.Contains(t, string(globalData), "enabled: true")
	assert.NotContains(t, string(globalData), "printf local")
	resolvedConfig, err := os.Readlink(globalPath)
	require.NoError(t, err)
	assert.Equal(t, configTarget, resolvedConfig)
	info, err := os.Stat(globalPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	local, _, err := client.LoadSharedConfig()
	require.NoError(t, err)
	require.Len(t, local.Resources, 1)
	assert.True(t, local.Resources[0].Disabled)
	assert.Equal(t, "web/node_modules", local.Resources[0].Path)
	_, err = client.RemoveSharedResource("**/node_modules")
	require.NoError(t, err)
	exemptTarget := t.TempDir()
	_, err = client.syncSharedResourcesToPath(exemptTarget, false)
	require.NoError(t, err)
	linked, err := os.Lstat(filepath.Join(exemptTarget, ".local"))
	require.NoError(t, err)
	assert.NotZero(t, linked.Mode()&os.ModeSymlink)
	_, err = client.RemoveGlobalSharedResource(".local")
	require.NoError(t, err)
	_, err = client.RemoveGlobalHook(1)
	require.NoError(t, err)
	global, _, err := client.LoadGlobalSharedConfig()
	require.NoError(t, err)
	require.Len(t, global.Resources, 1)
	require.Len(t, global.Hooks, 1)
}

func TestPreparationExclusionsCoverNestedPatterns(t *testing.T) {
	repoDir := initTestRepo(t)
	t.Chdir(repoDir)
	client := NewClient(Options{})
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "apps/web/node_modules"), 0o755))
	resources, err := client.expandSharedResources([]SharedResource{
		{Path: "**/node_modules", Strategy: StrategySymlink, Origin: "global"},
		{Path: "apps", Disabled: true, Origin: "repository"},
	})
	require.NoError(t, err)
	assert.Empty(t, resources)
	_, err = client.expandSharedResources([]SharedResource{
		{Path: "apps", Strategy: StrategySymlink, Origin: "global"},
		{Path: "**/node_modules", Disabled: true, Origin: "repository"},
	})
	require.Error(t, err)
	_, configPath, err := client.LoadSharedConfig()
	require.NoError(t, err)
	require.NoError(t, client.SaveSharedConfig(&SharedConfig{Resources: []SharedResource{
		{Path: "apps", Strategy: StrategySymlink},
		{Path: "**/node_modules", Disabled: true},
	}}, configPath))
	_, err = client.SyncAllSharedResources()
	require.Error(t, err)
}

func TestPreparationKeepsIndependentAndBrokenResources(t *testing.T) {
	repoDir := initTestRepo(t)
	t.Chdir(repoDir)
	client := NewClient(Options{})
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "node_modules"), 0o755))
	_, err := client.AddSharedResource("node_modules", StrategySymlink)
	require.NoError(t, err)
	target := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(target, "node_modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "node_modules/owned"), []byte("preserve"), 0o644))
	report, err := client.syncSharedResourcesToPath(target, false)
	require.NoError(t, err)
	require.NotEmpty(t, report.Events)
	assert.Equal(t, EventWarn, report.Events[0].Level)
	data, err := os.ReadFile(filepath.Join(target, "node_modules/owned"))
	require.NoError(t, err)
	assert.Equal(t, "preserve", string(data))
	broken := t.TempDir()
	require.NoError(t, os.Symlink("missing-source", filepath.Join(broken, "node_modules")))
	report, err = client.syncSharedResourcesToPath(broken, false)
	require.NoError(t, err)
	require.NotEmpty(t, report.Events)
	assert.Equal(t, EventWarn, report.Events[0].Level)
	link, err := os.Readlink(filepath.Join(broken, "node_modules"))
	require.NoError(t, err)
	assert.Equal(t, "missing-source", link)
}

func TestPreparationRejectsTrackedContentAndSymlinkParents(t *testing.T) {
	repoDir := initTestRepo(t)
	t.Chdir(repoDir)
	client := NewClient(Options{})
	target := t.TempDir()
	_, err := client.syncOneResource(repoDir, target, SharedResource{Path: "README.md", Strategy: StrategySymlink})
	require.Error(t, err)
	_, err = os.Lstat(filepath.Join(target, "README.md"))
	assert.True(t, os.IsNotExist(err))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "app/node_modules"), 0o755))
	external := t.TempDir()
	require.NoError(t, os.Symlink(external, filepath.Join(target, "app")))
	_, err = client.syncOneResource(repoDir, target, SharedResource{Path: "app/node_modules", Strategy: StrategySymlink})
	require.Error(t, err)
	_, err = os.Lstat(filepath.Join(external, "node_modules"))
	assert.True(t, os.IsNotExist(err))
}

func TestGlobalConfigInvalidEditDoesNotChangeFile(t *testing.T) {
	globalPath := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("model: preserve\nworktree:\n  hooks:\n    - id: setup\n      cmd: echo ok\n")
	require.NoError(t, os.WriteFile(globalPath, original, 0o600))
	client := NewClient(Options{GlobalConfigPath: globalPath})
	_, err := client.AddGlobalSharedResource(".git/config", StrategySymlink)
	require.Error(t, err)
	_, err = client.AddGlobalHook(Hook{ID: "2", Cmd: "echo no"})
	require.Error(t, err)
	after, err := os.ReadFile(globalPath)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(original, after))
	anchored := []byte("worktree:\n  hooks:\n    - &saved\n      id: saved\n      cmd: true\nother: *saved\n")
	require.NoError(t, os.WriteFile(globalPath, anchored, 0o600))
	_, err = client.AddGlobalSharedResource(".local", StrategySymlink)
	require.Error(t, err)
	unchanged, err := os.ReadFile(globalPath)
	require.NoError(t, err)
	assert.Equal(t, anchored, unchanged)
	multiple := []byte("worktree: {}\n---\nother: preserve\n")
	require.NoError(t, os.WriteFile(globalPath, multiple, 0o600))
	_, err = client.AddGlobalSharedResource(".local", StrategySymlink)
	require.Error(t, err)
	unchanged, err = os.ReadFile(globalPath)
	require.NoError(t, err)
	assert.Equal(t, multiple, unchanged)
}

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
	config := []byte("shared:\n  - path: .env\n    strategy: copy\n")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "gmc-share.yml"), config, 0o644))

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
	config := []byte("shared:\n  - path: .env\n    strategy: copy\nhooks:\n  - cmd: printf 'hook-ran' > hook.txt\n")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "gmc-share.yml"), config, 0o644))

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
	config := []byte("shared:\n  - path: .env\n    strategy: copy\n")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, legacySharedConfigYML), config, 0o644))

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

	config := []byte("shared:\n  - path: config/.env\n    strategy: copy\n")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "gmc-share.yml"), config, 0o644))

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

func TestPreparationPreservesNestedPathsMatchingWorktreeNames(t *testing.T) {
	repo := initTestRepo(t)
	t.Chdir(repo)
	client := NewClient(Options{})
	_, err := client.Add("web", AddOptions{})
	require.NoError(t, err)
	worktrees, err := client.ListCached()
	require.NoError(t, err)
	var target string
	for _, wt := range worktrees {
		if wt.Branch == "web" {
			target = wt.Path
		}
	}
	require.NotEmpty(t, target)
	relative := filepath.Join(filepath.Base(target), "node_modules")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, relative), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, relative, "package"), []byte("nested"), 0o644))
	_, err = client.AddSharedResource("**/node_modules", StrategySymlink)
	require.NoError(t, err)
	_, err = client.SyncAllSharedResources()
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(target, relative, "package"))
	require.NoError(t, err)
	assert.Equal(t, "nested", string(data))
	_, err = os.Lstat(filepath.Join(target, "node_modules"))
	assert.True(t, os.IsNotExist(err))
	_, err = client.RemoveSharedResource("**/node_modules")
	require.NoError(t, err)
	_, err = client.AddSharedResource(relative, StrategySymlink)
	require.NoError(t, err)
	_, err = client.SyncAllSharedResources()
	require.NoError(t, err)
	_, err = os.Lstat(filepath.Join(target, "node_modules"))
	assert.True(t, os.IsNotExist(err))
}

func TestDiscoverReportsBrokenSourceLink(t *testing.T) {
	repo := initTestRepo(t)
	t.Chdir(repo)
	client := NewClient(Options{})
	_, err := client.AddSharedResource(".local", StrategySymlink)
	require.NoError(t, err)
	require.NoError(t, os.Symlink("missing", filepath.Join(repo, ".local")))
	results, err := client.Discover(DiscoverOptions{IncludeConfigured: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "broken-link", results[0].TargetState)
	require.NoError(t, os.Mkdir(filepath.Join(repo, "missing"), 0o755))
	results, err = client.Discover(DiscoverOptions{IncludeConfigured: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "source", results[0].TargetState)
}

func TestAutomaticSharingDoesNotBorrowTransientSources(t *testing.T) {
	repo := initTestRepo(t)
	t.Chdir(repo)
	client := NewClient(Options{GlobalConfigPath: filepath.Join(t.TempDir(), "global.yaml")})
	_, err := client.Add("feature", AddOptions{})
	require.NoError(t, err)
	worktrees, err := client.ListCached()
	require.NoError(t, err)
	var feature string
	for _, wt := range worktrees {
		if wt.Branch == "feature" {
			feature = wt.Path
		}
	}
	require.NotEmpty(t, feature)
	for _, relative := range []string{"pkg/node_modules", ".local"} {
		require.NoError(t, os.MkdirAll(filepath.Join(feature, relative), 0o755))
	}
	_, err = client.AddGlobalSharedResource("**/node_modules", StrategySymlink)
	require.NoError(t, err)
	_, err = client.AddGlobalSharedResource(".local", StrategySymlink)
	require.NoError(t, err)
	_, err = client.SyncAllSharedResources()
	require.NoError(t, err)
	for _, relative := range []string{"pkg", ".local"} {
		_, err = os.Lstat(filepath.Join(repo, relative))
		assert.True(t, os.IsNotExist(err))
	}
	results, err := client.Discover(DiscoverOptions{IncludeConfigured: true})
	require.NoError(t, err)
	var inspected bool
	for _, result := range results {
		if result.Path == "pkg/node_modules" {
			inspected = true
			assert.Equal(t, filepath.Join(feature, "pkg/node_modules"), result.Source)
			assert.Equal(t, "missing", result.TargetState)
		}
	}
	assert.True(t, inspected)
	_, err = client.Add("consumer", AddOptions{})
	require.NoError(t, err)
	for _, relative := range []string{"pkg/node_modules", ".local"} {
		require.NoError(t, os.MkdirAll(filepath.Join(repo, relative), 0o755))
	}
	_, err = client.SyncAllSharedResources()
	require.NoError(t, err)
	worktrees, err = client.ListCached()
	require.NoError(t, err)
	for _, wt := range worktrees {
		if wt.Branch == "consumer" {
			for _, relative := range []string{"pkg/node_modules", ".local"} {
				resolved, err := filepath.EvalSymlinks(filepath.Join(wt.Path, relative))
				require.NoError(t, err)
				expected, err := filepath.EvalSymlinks(filepath.Join(repo, relative))
				require.NoError(t, err)
				assert.Equal(t, expected, resolved)
			}
		}
	}
}
