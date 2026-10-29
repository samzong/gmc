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

func TestGlobalRootLifecycle(t *testing.T) {
	for _, bare := range []bool{false, true} {
		for _, removed := range []bool{false, true} {
			t.Run(strconv.FormatBool(bare)+"/removed="+strconv.FormatBool(removed), func(t *testing.T) {
				repo, root, c := rootFixture(t, bare)
				runGit(t, repo, "remote", "add", "origin", "git@github.com:team/remote-name.git")
				paths := map[string]string{}
				for _, name := range []string{"offline", "remove", "prune"} {
					result, err := c.Add(name, AddOptions{})
					require.NoError(t, err)
					paths[name] = result.Path
					base := filepath.Base(repo)
					if bare {
						base = filepath.Base(filepath.Dir(repo))
					}
					assert.Equal(t, filepath.Join(root, "team", base, base+"--"+name), result.Path)
				}
				external := rootClient(t, paths["offline"], root)
				require.NoError(t, external.ensureInit())
				assert.Equal(t, c.worktreeRoot, external.worktreeRoot)
				next := rootClient(t, repo, physicalTempDir(t))
				if removed {
					writeFile(t, next.globalConfigPath, "{}\n")
				}
				c = next
				moved := paths["offline"] + "-unmounted"
				require.NoError(t, os.Rename(paths["offline"], moved))
				runGit(t, repo, "worktree", "prune", "--expire", "now")
				for _, force := range []bool{false, true} {
					_, err := c.Remove(paths["offline"], RemoveOptions{Force: force, DeleteBranch: true})
					require.ErrorContains(t, err, "reconnect "+root+" and retry")
					result, err := c.Prune(PruneOptions{Force: force, Branches: true, DryRun: true})
					require.NoError(t, err)
					assert.Contains(t, strings.Join(result.Warnings(), "\n"), "reconnect "+root+" and retry")
				}
				assert.Contains(t, runGit(t, repo, "branch", "--list", "offline"), "offline")
				assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), paths["offline"])
				writeFile(t, filepath.Join(paths["remove"], "dirty"), "preserve")
				_, err := c.Remove(paths["remove"], RemoveOptions{})
				require.Error(t, err)
				gitDir := strings.TrimSpace(strings.TrimPrefix(readFile(t, filepath.Join(paths["remove"], ".git")), "gitdir: "))
				assertFileContent(t, filepath.Join(gitDir, "locked"), globalRootLockReason+"\n")
				_, err = c.Remove(paths["remove"], RemoveOptions{Force: true})
				require.NoError(t, err)
				writeFile(t, filepath.Join(paths["prune"], "README.md"), "changed")
				_, err = c.Prune(PruneOptions{})
				require.NoError(t, err)
				assert.DirExists(t, paths["prune"])
				_, err = c.Prune(PruneOptions{Force: true})
				require.NoError(t, err)
				assertMissing(t, paths["prune"])
				require.NoError(t, os.Rename(moved, paths["offline"]))
				_, err = c.Remove("offline", RemoveOptions{DeleteBranch: true})
				require.NoError(t, err)
			})
		}
	}
}

func TestWorktreeRemoveNames(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(strconv.FormatBool(bare), func(t *testing.T) {
			repo, _, first := rootFixture(t, bare)
			c := NewClient(Options{Dir: repo})
			for _, action := range []string{"remove", "force-remove", "prune"} {
				created, err := c.Add(action, AddOptions{})
				require.NoError(t, err)
				require.NoError(t, os.RemoveAll(created.Path))
				if action == "prune" {
					_, err = c.Prune(PruneOptions{Force: true})
				} else {
					_, err = c.Remove(created.Path, RemoveOptions{Force: action == "force-remove", DeleteBranch: true})
				}
				require.NoError(t, err)
				assert.NotContains(t, runGit(t, repo, "worktree", "list", "--porcelain"), created.Path)
				assert.Empty(t, runGit(t, repo, "branch", "--list", action))
			}
			for _, relative := range []string{".claude/worktrees/agent-x", "sub/nested"} {
				path := filepath.Join(repo, relative)
				runGit(t, repo, "worktree", "add", "-b", filepath.Base(path), path)
				writeFile(t, filepath.Join(path, "dirty"), "preserve")
				_, err := c.Remove(filepath.Base(path), RemoveOptions{Force: true})
				require.ErrorContains(t, err, "worktree not found")
				assert.FileExists(t, filepath.Join(path, "dirty"))
			}
			exact, err := c.Add("feat/x", AddOptions{})
			require.NoError(t, err)
			nested := filepath.Join(c.searchRoot, "deep", filepath.Base(exact.Path))
			runGit(t, repo, "worktree", "add", "-b", "deepx", nested)
			_, err = c.Remove(filepath.Base(exact.Path), RemoveOptions{})
			require.NoError(t, err)
			assertMissing(t, exact.Path)
			assert.DirExists(t, nested)
			a, err := first.Add("feat/x", AddOptions{Branch: "first"})
			require.NoError(t, err)
			second := rootClient(t, repo, physicalTempDir(t))
			b, err := second.Add("feat/x", AddOptions{Branch: "second"})
			require.NoError(t, err)
			sibling := filepath.Join(second.searchRoot, filepath.Base(a.Path))
			runGit(t, repo, "worktree", "add", "-b", "sibling", sibling)
			_, err = second.Remove(filepath.Base(sibling), RemoveOptions{})
			require.NoError(t, err)
			assertMissing(t, sibling)
			_, err = second.Remove("feat--x", RemoveOptions{})
			require.ErrorContains(t, err, "ambiguous")
			assert.DirExists(t, a.Path)
			assert.DirExists(t, b.Path)
			_, err = second.Remove(a.Path, RemoveOptions{})
			require.NoError(t, err)
			_, err = second.Remove("feat--x", RemoveOptions{})
			require.NoError(t, err)
			assertMissing(t, b.Path)
		})
	}
}

func TestManualLockRemovalPreservesGitDiagnostics(t *testing.T) {
	for _, reason := range []string{"mine", ""} {
		t.Run(reason, func(t *testing.T) {
			repo := initTestRepo(t)
			c := NewClient(Options{Dir: repo})
			created, err := c.Add("manual", AddOptions{})
			require.NoError(t, err)
			args := []string{"worktree", "lock"}
			if reason != "" {
				args = append(args, "--reason", reason)
			}
			runGit(t, repo, append(args, created.Path)...)
			for _, force := range []bool{false, true} {
				_, err = c.Remove(created.Path, RemoveOptions{Force: force})
				require.ErrorContains(t, err, "fatal: cannot remove a locked working tree")
				assert.Contains(t, err.Error(), "use 'remove -f -f' to override or unlock first")
				if reason != "" {
					assert.Contains(t, err.Error(), "lock reason: "+reason)
				}
			}
			_, err = c.Prune(PruneOptions{Force: true})
			require.NoError(t, err)
			assert.DirExists(t, created.Path)
			assert.Contains(t, runGit(t, repo, "worktree", "list", "--porcelain"), "\nlocked")
		})
	}
}
