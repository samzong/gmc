package worktree

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingLegacyWorktreeCleanup(t *testing.T) {
	for _, bare := range []bool{false, true} {
		for _, action := range []string{"remove", "force-remove", "prune"} {
			t.Run(strconv.FormatBool(bare)+"/"+action, func(t *testing.T) {
				repo := initTestRepo(t)
				if bare {
					repo = filepath.Join(initBareLayoutRepo(t), "main")
				}
				c := NewClient(Options{Dir: repo})
				result, err := c.Add("feat/legacy", AddOptions{})
				require.NoError(t, err)
				require.NoError(t, os.RemoveAll(result.Path))
				if action == "prune" {
					_, err = c.Prune(PruneOptions{Force: true})
				} else {
					_, err = c.Remove(filepath.Base(result.Path), RemoveOptions{Force: action == "force-remove", DeleteBranch: true})
				}
				require.NoError(t, err)
				assert.NotContains(t, runGit(t, repo, "worktree", "list", "--porcelain"), result.Path)
				assert.Empty(t, runGit(t, repo, "branch", "--list", "feat/legacy"))
			})
		}
	}
}

func TestGlobalRootLifecycleAfterConfigurationChange(t *testing.T) {
	for _, bare := range []bool{false, true} {
		for _, removed := range []bool{false, true} {
			t.Run(strconv.FormatBool(bare)+"/removed="+strconv.FormatBool(removed), func(t *testing.T) {
				repo := initTestRepo(t)
				if !bare {
					nested := filepath.Join(repo, "nested")
					runGit(t, repo, "clone", repo, nested)
					repo = nested
				}
				if bare {
					repo = filepath.Join(initBareLayoutRepo(t), "main")
				}
				t.Chdir(repo)
				c := rootClient(t, repo, physicalTempDir(t))
				paths := make(map[string]string)
				for _, name := range []string{"offline", "remove", "prune"} {
					result, err := c.Add(name, AddOptions{})
					require.NoError(t, err)
					paths[name] = result.Path
				}
				next := rootClient(t, repo, physicalTempDir(t))
				if removed {
					writeFile(t, next.globalConfigPath, "{}\n")
				}
				moved := paths["offline"] + "-unmounted"
				require.NoError(t, os.Rename(paths["offline"], moved))
				_, err := next.Remove(paths["offline"], RemoveOptions{Force: true, DeleteBranch: true})
				require.ErrorContains(t, err, "directory is unavailable")
				assert.Contains(t, err.Error(), "reconnect "+c.globalRoot+" and retry")
				_, err = next.Remove(paths["remove"], RemoveOptions{DeleteBranch: true})
				require.NoError(t, err)
				result, err := next.Prune(PruneOptions{Force: true, Branches: true})
				require.NoError(t, err)
				assertMissing(t, paths["prune"])
				assert.Contains(t, strings.Join(result.Warnings(), "\n"), "reconnect "+c.globalRoot+" and retry")
				assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), paths["offline"])
				assert.Contains(t, runGit(t, repo, "branch", "--list", "offline"), "offline")
				require.NoError(t, os.Rename(moved, paths["offline"]))
				_, err = next.Remove("offline", RemoveOptions{DeleteBranch: true})
				require.NoError(t, err)
			})
		}
	}
}

func TestGlobalRootShortRemoveNames(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(strconv.FormatBool(bare), func(t *testing.T) {
			repo := initTestRepo(t)
			if bare {
				repo = filepath.Join(initBareLayoutRepo(t), "main")
			}
			t.Chdir(repo)
			c := rootClient(t, repo, physicalTempDir(t))
			result, err := c.Add("feat/b", AddOptions{})
			require.NoError(t, err)
			_, err = c.Remove("feat--b", RemoveOptions{DeleteBranch: true})
			require.NoError(t, err)
			assertMissing(t, result.Path)
		})
	}
}

func TestGlobalRootShortRemoveAmbiguity(t *testing.T) {
	repo := initTestRepo(t)
	t.Chdir(repo)
	first := rootClient(t, repo, physicalTempDir(t))
	a, err := first.Add("feat/x", AddOptions{Branch: "first"})
	require.NoError(t, err)
	second := rootClient(t, repo, physicalTempDir(t))
	b, err := second.Add("feat/x", AddOptions{Branch: "second"})
	require.NoError(t, err)
	_, err = second.Remove("feat--x", RemoveOptions{DeleteBranch: true})
	require.ErrorContains(t, err, "ambiguous")
	assert.DirExists(t, a.Path)
	assert.DirExists(t, b.Path)
	_, err = second.Remove(a.Path, RemoveOptions{DeleteBranch: true})
	require.NoError(t, err)
}

func TestLegacyRemoveNameResolution(t *testing.T) {
	repo := initTestRepo(t)
	c := NewClient(Options{Dir: repo})
	for _, relative := range []string{".claude/worktrees/agent-x", "sub/nested"} {
		path := filepath.Join(repo, relative)
		runGit(t, repo, "worktree", "add", "-b", filepath.Base(path), path)
		writeFile(t, filepath.Join(path, "dirty"), "preserve")
		_, err := c.Remove(filepath.Base(path), RemoveOptions{Force: true})
		require.ErrorContains(t, err, "worktree not found")
		assert.FileExists(t, filepath.Join(path, "dirty"))
	}
}

func TestLegacyBareRemoveExactNameWins(t *testing.T) {
	root := initBareLayoutRepo(t)
	repo := filepath.Join(root, "main")
	c := NewClient(Options{Dir: repo})
	exact, err := c.Add("feat/x", AddOptions{})
	require.NoError(t, err)
	nested := filepath.Join(root, "deep", "feat--x")
	runGit(t, repo, "worktree", "add", "-b", "nested", nested)
	_, err = c.Remove("feat--x", RemoveOptions{})
	require.NoError(t, err)
	assertMissing(t, exact.Path)
	assert.DirExists(t, nested)
}

func TestGlobalRootRemoveExactNameWins(t *testing.T) {
	repo := initTestRepo(t)
	c := rootClient(t, repo, physicalTempDir(t))
	global, err := c.Add("c", AddOptions{})
	require.NoError(t, err)
	sibling := repo + "--c"
	runGit(t, repo, "worktree", "add", "-b", "sibling", sibling)
	t.Cleanup(func() { _ = os.RemoveAll(sibling) })
	_, err = c.Remove(filepath.Base(sibling), RemoveOptions{})
	require.NoError(t, err)
	assertMissing(t, sibling)
	assert.DirExists(t, global.Path)
}

func TestManualLockRemovalPreservesGitDiagnostics(t *testing.T) {
	for _, reason := range []string{"mine", ""} {
		for _, force := range []bool{false, true} {
			t.Run(reason+"/force="+strconv.FormatBool(force), func(t *testing.T) {
				repo := initTestRepo(t)
				c := NewClient(Options{Dir: repo})
				created, err := c.Add("locked", AddOptions{})
				require.NoError(t, err)
				t.Cleanup(func() { _ = os.RemoveAll(created.Path) })
				args := []string{"worktree", "lock"}
				if reason != "" {
					args = append(args, "--reason", reason)
				}
				runGit(t, repo, append(args, created.Path)...)
				_, err = c.Remove(filepath.Base(created.Path), RemoveOptions{Force: force})
				require.ErrorContains(t, err, "fatal: cannot remove a locked working tree")
				assert.Contains(t, err.Error(), "use 'remove -f -f' to override or unlock first")
				if reason != "" {
					assert.Contains(t, err.Error(), "lock reason: "+reason)
				}
				assert.DirExists(t, created.Path)
				assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), "\nlocked")
			})
		}
	}
}
