package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func TestGlobalRootAddAndRemove(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(strconv.FormatBool(bare), func(t *testing.T) {
			repo := initTestRepo(t)
			source := repo
			if bare {
				repo = initBareLayoutRepo(t)
				source = filepath.Join(repo, "main")
			}
			t.Chdir(source)
			runGit(t, source, "remote", "add", "origin", "git@github.com:team/remote-name.git")
			root := physicalTempDir(t)
			c := rootClient(t, source, root)
			result, err := c.Add("feat/x", AddOptions{})
			require.NoError(t, err)
			want := filepath.Join(root, "team", filepath.Base(repo), filepath.Base(repo)+"--feat--x")
			require.Equal(t, want, result.Path)
			assert.Contains(t, runGit(t, source, "worktree", "list", "--porcelain"), "locked "+globalRootLockReason)
			external := rootClient(t, want, root)
			require.NoError(t, external.ensureInit())
			assert.Equal(t, repo, external.worktreeRoot)
			writeFile(t, filepath.Join(want, "dirty"), "preserve")
			_, err = c.Remove(filepath.Base(want), RemoveOptions{})
			require.Error(t, err)
			assert.Contains(t, runGit(t, source, "worktree", "list", "--porcelain"), "locked "+globalRootLockReason)
			_, err = c.Remove(filepath.Base(want), RemoveOptions{Force: true})
			require.NoError(t, err)
			assertMissing(t, want)
		})
	}
}

func TestGlobalRootFallbackAndUntrustedConfig(t *testing.T) {
	repo := initTestRepo(t)
	t.Chdir(repo)
	missing := filepath.Join(t.TempDir(), "unmounted", "root")
	c := rootClient(t, repo, missing)
	result, err := c.Add("fallback", AddOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(result.Path) })
	assert.Equal(t, repo+"--fallback", result.Path)
	require.Len(t, result.Warnings, 1)
	assertMissing(t, missing)
	writeFile(t, filepath.Join(repo, ".gmc.yaml"), "worktree:\n  root: /untrusted\n")
	writeFile(t, filepath.Join(repo, ".git", sharedConfigName), "root: /untrusted\nshared: []\n")
	plain := NewClient(Options{Dir: repo})
	result, err = plain.Add("unchanged", AddOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(result.Path) })
	assert.Equal(t, repo+"--unchanged", result.Path)
	assert.Empty(t, result.Warnings)
}

func TestGlobalRootDupAndSharedConfig(t *testing.T) {
	repo := initTestRepo(t)
	t.Chdir(repo)
	root := physicalTempDir(t)
	c := rootClient(t, repo, root)
	require.NoError(t, os.Mkdir(filepath.Join(repo, "node_modules"), 0755))
	_, err := c.AddSharedResource("node_modules", StrategySymlink, true)
	require.NoError(t, err)
	cfg, _, err := c.LoadGlobalSharedConfig()
	require.NoError(t, err)
	assert.Equal(t, root, cfg.Root)
	require.Len(t, cfg.Resources, 1)
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
}

func TestGlobalRootOfflineProtection(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(strconv.FormatBool(bare), func(t *testing.T) {
			repo := initTestRepo(t)
			if bare {
				repo = filepath.Join(initBareLayoutRepo(t), "main")
			}
			t.Chdir(repo)
			c := rootClient(t, repo, physicalTempDir(t))
			result, err := c.Add("offline", AddOptions{})
			require.NoError(t, err)
			moved := result.Path + "-unmounted"
			require.NoError(t, os.Rename(result.Path, moved))
			runGit(t, repo, "worktree", "prune", "--expire", "now")
			assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), result.Path)
			for _, force := range []bool{false, true} {
				result, err := c.Prune(PruneOptions{Force: force, Branches: true})
				require.NoError(t, err)
				assert.Contains(t, fmt.Sprint(result.Warnings()), "directory is unavailable")
			}
			_, err = c.Remove(result.Path, RemoveOptions{Force: true, DeleteBranch: true})
			require.ErrorContains(t, err, "directory is unavailable")
			assert.Contains(t, runGit(t, repo, "branch", "--list", "offline"), "offline")
			assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), result.Path)
			require.NoError(t, os.Rename(moved, result.Path))
			writeFile(t, filepath.Join(result.Path, "README.md"), "changed")
			_, err = c.Prune(PruneOptions{Force: false})
			require.NoError(t, err)
			assert.DirExists(t, result.Path)
			_, err = c.Prune(PruneOptions{Force: true})
			require.NoError(t, err)
			assertMissing(t, result.Path)
		})
	}
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

func TestGlobalRootPR(t *testing.T) {
	repo := initTestRepo(t)
	t.Chdir(repo)
	runGit(t, repo, "remote", "add", "origin", initPRRemote(t, 42))
	c := rootClient(t, repo, physicalTempDir(t))
	result, err := c.AddPR(42, "")
	require.NoError(t, err)
	assert.Contains(t, result.Path, filepath.Base(repo)+"--pr--42")
	assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), "locked "+globalRootLockReason)
}

func TestGlobalRootPathValidation(t *testing.T) {
	repo := initTestRepo(t)
	home := physicalTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, value := range []string{"~", "~/worktrees"} {
		c := rootClient(t, repo, value)
		require.NoError(t, c.ensureInit())
		root, _, err := c.globalWorktreeDirectory()
		require.NoError(t, err)
		want := home
		if value != "~" {
			want = filepath.Join(home, "worktrees")
		}
		assert.Equal(t, want, root)
	}
	c := rootClient(t, repo, "relative/root")
	_, err := c.Add("invalid", AddOptions{})
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

func TestGlobalRootManualLock(t *testing.T) {
	repo := initTestRepo(t)
	c := rootClient(t, repo, physicalTempDir(t))
	result, err := c.Add("manual", AddOptions{})
	require.NoError(t, err)
	runGit(t, repo, "worktree", "unlock", result.Path)
	runGit(t, repo, "worktree", "lock", "--reason", "owner lock", result.Path)
	_, err = c.Remove(result.Path, RemoveOptions{Force: true})
	require.ErrorContains(t, err, "locked")
	_, err = c.Prune(PruneOptions{Force: true})
	require.NoError(t, err)
	assert.DirExists(t, result.Path)
	assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), "locked owner lock")
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
	assert.Equal(t, root, cachedRoot)
	assert.Equal(t, relative, cachedRelative)
	next := NewClient(Options{Dir: repo, GlobalConfigPath: c.globalConfigPath})
	newRoot, newRelative, err := next.globalWorktreeDirectory()
	require.NoError(t, err)
	assert.Equal(t, nextRoot, newRoot)
	assert.Equal(t, filepath.Join("second", filepath.Base(repo)), newRelative)
}
