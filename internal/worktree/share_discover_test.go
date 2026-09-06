package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDiscover_FindsCandidates(t *testing.T) {
	mainWT := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(mainWT, ".env"), []byte("X=1"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(mainWT, "node_modules"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(mainWT, ".claude"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(mainWT, ".claude", "CLAUDE.md"), []byte("# hi"), 0644))

	repoDir := t.TempDir()
	bareDir := filepath.Join(repoDir, ".bare")
	require.NoError(t, os.Mkdir(bareDir, 0755))

	oldCwd, _ := os.Getwd()
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldCwd) }()

	client := NewClient(Options{})
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: mainWT})
	require.NoError(t, err)

	assert.Len(t, results, 2)

	paths := make(map[string]ResourceStrategy)
	for _, r := range results {
		paths[r.Path] = r.Strategy
	}
	assert.Equal(t, StrategyCopy, paths[".env"])
	assert.Equal(t, StrategyCopy, paths[".claude/CLAUDE.md"])
	assert.NotContains(t, paths, "node_modules")
}

func TestDiscover_SkipsExisting(t *testing.T) {
	mainWT := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mainWT, ".env"), []byte("X=1"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(mainWT, "node_modules"), 0755))

	repoDir := t.TempDir()
	bareDir := filepath.Join(repoDir, ".bare")
	require.NoError(t, os.Mkdir(bareDir, 0755))

	cfg := SharedConfig{
		Resources: []SharedResource{
			{Path: ".env", Strategy: StrategyCopy},
		},
	}
	data, _ := yaml.Marshal(&cfg)
	require.NoError(t, os.WriteFile(filepath.Join(bareDir, "gmc-share.yml"), data, 0644))

	oldCwd, _ := os.Getwd()
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldCwd) }()

	client := NewClient(Options{})
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: mainWT})
	require.NoError(t, err)

	assert.Empty(t, results)
}

func TestDiscover_EmptyWorktree(t *testing.T) {
	mainWT := t.TempDir()

	repoDir := t.TempDir()
	bareDir := filepath.Join(repoDir, ".bare")
	require.NoError(t, os.Mkdir(bareDir, 0755))

	oldCwd, _ := os.Getwd()
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldCwd) }()

	client := NewClient(Options{})
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: mainWT})
	require.NoError(t, err)

	assert.Empty(t, results)
}

func TestDiscover_AllCopyCandidates(t *testing.T) {
	mainWT := t.TempDir()

	for _, c := range copyCandidates {
		full := filepath.Join(mainWT, c)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte("data"), 0644))
	}

	repoDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repoDir, ".bare"), 0755))

	oldCwd, _ := os.Getwd()
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldCwd) }()

	client := NewClient(Options{})
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: mainWT})
	require.NoError(t, err)

	var copyCount int
	for _, r := range results {
		if r.Strategy == StrategyCopy {
			copyCount++
		}
	}
	assert.Equal(t, len(copyCandidates), copyCount)
}

func TestAddDiscoveredResources_Batch(t *testing.T) {
	repoDir := t.TempDir()
	bareDir := filepath.Join(repoDir, ".bare")
	require.NoError(t, os.Mkdir(bareDir, 0755))

	oldCwd, _ := os.Getwd()
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldCwd) }()

	client := NewClient(Options{})

	results := []DiscoverResult{
		{Path: ".env", Strategy: StrategyCopy, Reason: "test"},
		{Path: "node_modules", Strategy: StrategySymlink, Reason: "test"},
	}

	report, err := client.AddDiscoveredResources(results)
	require.NoError(t, err)
	assert.Len(t, report.Events, 2)

	cfg, _, err := client.LoadSharedConfig()
	require.NoError(t, err)
	assert.Len(t, cfg.Resources, 2)
	assert.Equal(t, ".env", cfg.Resources[0].Path)
	assert.Equal(t, StrategyCopy, cfg.Resources[0].Strategy)
	assert.Equal(t, "node_modules", cfg.Resources[1].Path)
	assert.Equal(t, StrategySymlink, cfg.Resources[1].Strategy)
}

func TestDiscover_MonorepoProjectsAndTraversalBoundaries(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".bare"), 0755))
	t.Chdir(repo)
	root := t.TempDir()
	files := map[string]string{
		"apps/web/package.json":                               "{}",
		"apps/web/pnpm-lock.yaml":                             "lockfileVersion: '9.0'",
		"apps/web/node_modules/pkg/index.js":                  "module.exports = 1",
		"services/api/pyproject.toml":                         "[project]",
		"services/api/uv.lock":                                "version = 1",
		"services/api/.venv/lib/package.py":                   "x = 1",
		"services/gateway/go.mod":                             "module example.org/gateway",
		"crates/worker/Cargo.toml":                            "[package]",
		"crates/worker/target/fake/package.json":              "{}",
		"crates/worker/target/fake/node_modules/pkg/index.js": "x",
		".local/scratch/node_modules/pkg/index.js":            "x",
		"apps/web/node_modules/pkg/.env":                      "X=1",
		"nested/.git/HEAD":                                    "ref: refs/heads/main",
		"nested/node_modules/pkg/index.js":                    "x",
	}
	for path, content := range files {
		full := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0644))
	}
	external := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(external, "node_modules"), 0755))
	require.NoError(t, os.Symlink(external, filepath.Join(root, "external")))
	client := NewClient(Options{})
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: root, IncludeConfigured: true})
	require.NoError(t, err)
	var candidates []DiscoverResult
	byPath := make(map[string]DiscoverResult)
	var buildArtifact DiscoverResult
	projects := make(map[string]DiscoverResult)
	for _, result := range results {
		byPath[result.Path] = result
		if result.Path == "crates/worker/target" {
			buildArtifact = result
		}
		if result.Status == "candidate" {
			candidates = append(candidates, result)
		}
		if result.Status == "managed" && result.Project != "" {
			projects[result.Project] = result
		}
	}
	assert.Equal(t, "managed", buildArtifact.Status)
	assert.Greater(t, buildArtifact.SizeBytes, int64(0))
	assert.Empty(t, buildArtifact.Strategy)
	assert.Empty(t, candidates)
	node := byPath["apps/web/node_modules"]
	assert.Equal(t, "managed", node.Status)
	assert.Empty(t, node.Strategy)
	assert.Equal(t, "pnpm", node.PackageManager)
	packageBytes := len(files["apps/web/node_modules/pkg/index.js"]) + len(files["apps/web/node_modules/pkg/.env"])
	assert.Equal(t, int64(packageBytes), node.SizeBytes)
	python := byPath["services/api/.venv"]
	assert.Equal(t, "managed", python.Status)
	assert.Empty(t, python.Strategy)
	assert.Equal(t, "uv", python.PackageManager)
	require.Len(t, projects, 4)
	assert.Equal(t, "Go", projects["services/gateway"].Ecosystem)
	assert.Equal(t, "Rust", projects["crates/worker"].Ecosystem)
	_, err = client.AddDiscoveredResources(results)
	require.NoError(t, err)
	cfg, _, err := client.LoadSharedConfig()
	require.NoError(t, err)
	assert.Empty(t, cfg.Resources)
}

func TestDiscover_ParentCoverageAndExclusions(t *testing.T) {
	repo := t.TempDir()
	bare := filepath.Join(repo, ".bare")
	require.NoError(t, os.Mkdir(bare, 0755))
	t.Chdir(repo)
	root := t.TempDir()
	for _, path := range []string{".serena/project.yml", "apps/web/node_modules/pkg/index.js", ".local/result.txt"} {
		full := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte("data"), 0644))
	}
	cfg := SharedConfig{Resources: []SharedResource{
		{Path: ".serena", Strategy: StrategySymlink},
		{Path: "**/node_modules", Strategy: StrategySymlink, Disabled: true},
		{Path: ".local", Strategy: StrategySymlink},
	}}
	client := NewClient(Options{})
	require.NoError(t, client.SaveSharedConfig(&cfg, filepath.Join(bare, "gmc-share.yml")))
	candidates, err := client.Discover(DiscoverOptions{MainWorktreePath: root})
	require.NoError(t, err)
	assert.Empty(t, candidates)
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: root, IncludeConfigured: true})
	require.NoError(t, err)
	byPath := make(map[string]DiscoverResult)
	for _, result := range results {
		byPath[result.Path] = result
	}
	assert.Equal(t, "configured", byPath[".serena/project.yml"].Status)
	assert.Equal(t, ".serena", byPath[".serena/project.yml"].ConfiguredBy)
	assert.Equal(t, "excluded", byPath["apps/web/node_modules"].Status)
	assert.Equal(t, "configured", byPath[".local"].Status)
	report, err := client.AddDiscoveredResources(results)
	require.NoError(t, err)
	assert.Empty(t, report.Events)
}

func TestDiscover_TrackedContentCannotBecomeShared(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.org")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "vendor", "pkg"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "vendor", "pkg", "source.go"), []byte("package pkg"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("X=1"), 0644))
	runGit(t, root, "add", "vendor")
	t.Chdir(root)
	client := NewClient(Options{})
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: root, IncludeConfigured: true})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "candidate", results[0].Status)
	assert.Equal(t, "tracked", results[1].Status)
	_, err = client.AddDiscoveredResources(results)
	require.NoError(t, err)
	cfg, _, err := client.LoadSharedConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Resources, 1)
	assert.Equal(t, ".env", cfg.Resources[0].Path)
}

func TestDiscover_ExplicitResourceInsidePrunedDirectoryAndInheritedManager(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".bare"), 0755))
	t.Chdir(repo)
	root := t.TempDir()
	for path, content := range map[string]string{
		"pnpm-lock.yaml":                         "lockfileVersion: '9.0'",
		"apps/web/package.json":                  "{}",
		"apps/web/node_modules/package/index.js": "module.exports = 1",
		".local/reports/latest.json":             "{}",
	} {
		full := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0644))
	}
	client := NewClient(Options{})
	_, err := client.AddSharedResource(".local/reports/latest.json", StrategyCopy)
	require.NoError(t, err)
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: root, IncludeConfigured: true})
	require.NoError(t, err)
	byPath := make(map[string]DiscoverResult)
	for _, result := range results {
		byPath[result.Path] = result
	}
	assert.Equal(t, "configured", byPath[".local/reports/latest.json"].Status)
	assert.Equal(t, filepath.Join(root, ".local/reports/latest.json"), byPath[".local/reports/latest.json"].Source)
	assert.Equal(t, "pnpm", byPath["apps/web/node_modules"].PackageManager)
	assert.Equal(t, "pnpm", byPath["apps/web/package.json"].PackageManager)
	_, err = client.AddSharedResource("**/node_modules", StrategySymlink)
	require.NoError(t, err)
	results, err = client.Discover(DiscoverOptions{MainWorktreePath: root, IncludeConfigured: true})
	require.NoError(t, err)
	for _, result := range results {
		if result.Path == "apps/web/node_modules" {
			assert.Equal(t, "configured", result.Status)
			assert.Equal(t, StrategySymlink, result.Strategy)
			assert.Equal(t, "**/node_modules", result.ConfiguredBy)
		}
	}
}
