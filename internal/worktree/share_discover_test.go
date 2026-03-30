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

	assert.Len(t, results, 3)

	paths := make(map[string]ResourceStrategy)
	for _, r := range results {
		paths[r.Path] = r.Strategy
	}
	assert.Equal(t, StrategyCopy, paths[".env"])
	assert.Equal(t, StrategyCopy, paths[".claude/CLAUDE.md"])
	assert.Equal(t, StrategySymlink, paths["node_modules"])
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

	assert.Len(t, results, 1)
	assert.Equal(t, "node_modules", results[0].Path)
	assert.Equal(t, StrategySymlink, results[0].Strategy)
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

func TestDiscover_AllLinkCandidates(t *testing.T) {
	mainWT := t.TempDir()

	for _, c := range linkCandidates {
		require.NoError(t, os.Mkdir(filepath.Join(mainWT, c), 0755))
	}

	repoDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repoDir, ".bare"), 0755))

	oldCwd, _ := os.Getwd()
	require.NoError(t, os.Chdir(repoDir))
	defer func() { _ = os.Chdir(oldCwd) }()

	client := NewClient(Options{})
	results, err := client.Discover(DiscoverOptions{MainWorktreePath: mainWT})
	require.NoError(t, err)

	var linkCount int
	for _, r := range results {
		if r.Strategy == StrategySymlink {
			linkCount++
		}
	}
	assert.Equal(t, len(linkCandidates), linkCount)
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
