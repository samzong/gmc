package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rootClient(t *testing.T, repo, root string) *Client {
	t.Helper()
	config := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, config, fmt.Sprintf("worktree:\n  root: %q\n", root))
	return NewClient(Options{Dir: repo, GlobalConfigPath: config})
}

func rootFixture(t *testing.T, bare bool) (string, string, *Client) {
	t.Helper()
	var repo string
	if bare {
		repo = filepath.Join(initBareLayoutRepo(t), "main")
	} else {
		repo = initTestRepo(t)
	}
	root := physicalTempDir(t)
	return repo, root, rootClient(t, repo, root)
}

func TestGlobalRootUntrustedConfig(t *testing.T) {
	repo := initTestRepo(t)
	writeFile(t, filepath.Join(repo, ".gmc.yaml"), "worktree:\n  root: /untrusted\n")
	writeFile(t, filepath.Join(repo, ".git", sharedConfigName), "root: /untrusted\nshared: []\n")
	result, err := NewClient(Options{Dir: repo}).Add("unchanged", AddOptions{})
	require.NoError(t, err)
	assert.Equal(t, repo+"--unchanged", result.Path)
	assert.Empty(t, result.Warnings)
}

func TestGlobalRootCreation(t *testing.T) {
	repo, root, c := rootFixture(t, false)
	require.NoError(t, os.Mkdir(filepath.Join(repo, "node_modules"), 0755))
	_, err := c.AddSharedResource("node_modules", StrategySymlink, true)
	require.NoError(t, err)
	cfg, _, err := c.LoadGlobalSharedConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Resources, 1)
	assert.Equal(t, root, cfg.Root)
	dup, err := c.Dup(DupOptions{Count: 1})
	require.NoError(t, err)
	require.Len(t, dup.Items, 1)
	path := dup.Items[0].Path
	want := filepath.Join(root, filepath.Base(filepath.Dir(repo)), filepath.Base(repo), filepath.Base(repo)+"--.dup-1")
	assert.Equal(t, want, path)
	linked, err := filepath.EvalSymlinks(filepath.Join(path, "node_modules"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(repo, "node_modules"), linked)
	_, err = c.RemoveSharedResource("node_modules", true)
	require.NoError(t, err)
	cfg, _, err = c.LoadGlobalSharedConfig()
	require.NoError(t, err)
	assert.Equal(t, root, cfg.Root)
	runGit(t, repo, "remote", "add", "origin", initPRRemote(t, 42))
	result, err := c.AddPR(42, "")
	require.NoError(t, err)
	assert.Contains(t, result.Path, filepath.Base(repo)+"--pr--42")
	assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), "locked "+globalRootLockReason)
}

func TestRemoteOwner(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/team/repo.git":        "team",
		"ssh://git@github.com:2222/team/repo.git": "team",
		"git@github.com:team/repo.git":            "team",
		"github.com:team/repo":                    "team",
		"/tmp/team/repo":                          "",
		"file:///tmp/team/repo":                   "",
		"https://github.com/../repo":              "",
		"https://github.com/team":                 "",
		"https://github.com/team/nested/repo":     "",
	} {
		assert.Equal(t, want, remoteOwner(remote), remote)
	}
}

func TestGlobalRootPathValidation(t *testing.T) {
	repo := initTestRepo(t)
	home := physicalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for value, want := range map[string]string{"~": home, "~/worktrees": filepath.Join(home, "worktrees")} {
		c := rootClient(t, repo, value)
		root, _, err := c.globalWorktreeDirectory()
		require.NoError(t, err)
		assert.Equal(t, want, root)
	}
	_, err := rootClient(t, repo, "relative/root").Add("invalid", AddOptions{})
	require.ErrorContains(t, err, "absolute path")
	root := physicalTempDir(t)
	file := filepath.Join(root, "file")
	writeFile(t, file, "not a directory")
	_, err = rootClient(t, repo, file).Add("file", AddOptions{})
	require.Error(t, err)
	runGit(t, repo, "remote", "add", "origin", "https://github.com/team/repo.git")
	require.NoError(t, os.Symlink(physicalTempDir(t), filepath.Join(root, "team")))
	_, err = rootClient(t, repo, root).Add("escape", AddOptions{})
	require.Error(t, err)
}

func TestGlobalRootConfigurationResolvedOncePerClient(t *testing.T) {
	repo := initTestRepo(t)
	runGit(t, repo, "remote", "add", "origin", "git@github.com:first/repo.git")
	c := rootClient(t, repo, physicalTempDir(t))
	root, relative, err := c.globalWorktreeDirectory()
	require.NoError(t, err)
	nextRoot := physicalTempDir(t)
	writeFile(t, c.globalConfigPath, fmt.Sprintf("worktree:\n  root: %q\n", nextRoot))
	runGit(t, repo, "remote", "set-url", "origin", "git@github.com:second/repo.git")
	cachedRoot, cachedRelative, err := c.globalWorktreeDirectory()
	require.NoError(t, err)
	assert.Equal(t, [2]string{root, relative}, [2]string{cachedRoot, cachedRelative})
	next := NewClient(Options{Dir: repo, GlobalConfigPath: c.globalConfigPath})
	newRoot, newRelative, err := next.globalWorktreeDirectory()
	require.NoError(t, err)
	assert.Equal(t, nextRoot, newRoot)
	assert.Equal(t, filepath.Join("second", filepath.Base(repo)), newRelative)
}
