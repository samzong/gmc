package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorktreeHomeNameResolves(t *testing.T) {
	repo := initTestRepo(t)
	home := physicalTempDir(t)
	t.Setenv("HOME", home)
	target := filepath.Join(home, "external")
	runGit(t, repo, "worktree", "add", "-b", "external", target, "main")
	client := NewClient(Options{Dir: repo})
	worktrees, err := client.ListCached()
	require.NoError(t, err)
	resolved, err := client.ResolveWorktree("~/external", worktrees, DisplayName)
	require.NoError(t, err)
	require.Equal(t, target, resolved.Path)
	require.DirExists(t, target)
}

func TestWorktreeNamesRoundTrip(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(fmt.Sprintf("bare=%t", bare), func(t *testing.T) {
			home := physicalTempDir(t)
			t.Setenv("HOME", home)
			repo := initTestRepo(t)
			if !bare {
				nestedRepo := filepath.Join(physicalTempDir(t), "repo")
				require.NoError(t, os.Rename(repo, nestedRepo))
				repo = nestedRepo
			}
			if bare {
				repo = filepath.Join(initBareLayoutRepo(t), "main")
			}
			client := rootClient(t, repo, "")
			require.NoError(t, client.ensureInit())
			cases := []struct{ branch, path, name string }{
				{"ordinary", filepath.Join(client.searchRoot, "ordinary"), "ordinary"},
				{"nested", filepath.Join(client.searchRoot, "deep", "nested"), filepath.Join("deep", "nested")},
				{"external-home", filepath.Join(home, "external"), "~/external"},
				{"external-away", filepath.Join(physicalTempDir(t), "external"), ""},
				{"claude", filepath.Join(home, ".claude", "worktrees", "agent"), "~/.claude/worktrees/agent"},
				{"codex", filepath.Join(home, ".codex", "worktrees", "agent"), "~/.codex/worktrees/agent"},
			}
			expected := map[string]string{}
			var collision Info
			for _, tc := range cases {
				runGit(t, repo, "worktree", "add", "-b", tc.branch, tc.path, "main")
				if tc.name == "" {
					tc.name = tc.path
				}
				expected[tc.path] = tc.name
			}
			for i, root := range []string{filepath.Join(home, "global"), physicalTempDir(t)} {
				require.NoError(t, os.MkdirAll(root, 0o755))
				globalClient := rootClient(t, repo, root)
				added, err := globalClient.Add(fmt.Sprintf("global-%d", i), AddOptions{BaseBranch: "main"})
				require.NoError(t, err)
				expected[added.Path] = filepath.Base(added.Path)
				if i == 1 {
					collision = Info{Path: added.Path, IsLocked: true, LockReason: globalRootLockReason}
				}
			}
			worktrees, err := client.List()
			require.NoError(t, err)
			duplicate := collision
			duplicate.Path = filepath.Join(physicalTempDir(t), filepath.Base(duplicate.Path))
			worktrees = append(worktrees, duplicate)
			expected[duplicate.Path] = duplicate.Path
			expected[collision.Path] = collision.Path
			for _, wt := range worktrees {
				t.Run(wt.Branch, func(t *testing.T) {
					name := client.WorktreeName(wt, worktrees)
					if want, ok := expected[wt.Path]; ok {
						require.Equal(t, want, name)
					}
					for _, use := range []NameUse{DisplayName, RemoveName, PromoteName} {
						resolved, err := client.ResolveWorktree(name, worktrees, use)
						require.NoError(t, err, "name=%s path=%s use=%d", name, wt.Path, use)
						require.Equal(t, wt.Path, resolved.Path)
					}
				})
			}
		})
	}
}

func TestWorktreeResolutionSymlinksAndMissingPaths(t *testing.T) {
	repo := initTestRepo(t)
	client := rootClient(t, repo, "")
	target := filepath.Join(physicalTempDir(t), "target")
	runGit(t, repo, "worktree", "add", "-b", "target", target, "main")
	alias := filepath.Join(physicalTempDir(t), "alias")
	require.NoError(t, os.Symlink(target, alias))
	worktrees, err := client.List()
	require.NoError(t, err)
	resolved, err := client.ResolveWorktree(alias, worktrees, DisplayName)
	require.NoError(t, err)
	require.Equal(t, target, resolved.Path)
	worktrees = append(worktrees, Info{Path: alias})
	_, err = client.ResolveWorktree(target, worktrees, DisplayName)
	require.ErrorContains(t, err, "ambiguous worktree")
	client.listCache, client.listValid = worktrees, true
	_, err = client.Remove(target, RemoveOptions{Force: true})
	require.ErrorContains(t, err, "ambiguous worktree")
	require.DirExists(t, target)
	require.NoError(t, os.RemoveAll(target))
	resolved, err = client.ResolveWorktree(target, worktrees[:len(worktrees)-1], DisplayName)
	require.NoError(t, err)
	require.Equal(t, target, resolved.Path)
	_, err = client.ResolveWorktree(alias, worktrees[:len(worktrees)-1], DisplayName)
	require.ErrorIs(t, err, errWorktreeNotFound)
}

func TestWorktreeResolutionPreservesCallerPriorities(t *testing.T) {
	repo := initTestRepo(t)
	t.Chdir(repo)
	client := rootClient(t, repo, "")
	require.NoError(t, client.ensureInit())
	sibling := filepath.Join(client.searchRoot, "x")
	child := filepath.Join(repo, "x")
	for i, path := range []string{sibling, child} {
		runGit(t, repo, "worktree", "add", "-b", fmt.Sprintf("x-%d", i), path, "main")
	}
	worktrees, err := client.List()
	require.NoError(t, err)
	for _, tc := range []struct {
		use  NameUse
		path string
	}{
		{RemoveName, sibling}, {PromoteName, child},
	} {
		resolved, err := client.ResolveWorktree("x", worktrees, tc.use)
		require.NoError(t, err)
		require.Equal(t, tc.path, resolved.Path)
	}
	_, err = client.ResolveWorktree("x", worktrees, SharedPrefix)
	require.ErrorContains(t, err, "ambiguous")
	require.Equal(t, sibling, client.WorktreeName(Info{Path: sibling}, worktrees))
	report, err := client.Remove("x", RemoveOptions{DryRun: true})
	require.NoError(t, err)
	require.Contains(t, report.Warnings(), "Would remove worktree: "+sibling)
	globalRoot := physicalTempDir(t)
	global := rootClient(t, repo, globalRoot)
	added, err := global.Add("agent", AddOptions{BaseBranch: "main"})
	require.NoError(t, err)
	agent := filepath.Join(repo, ".claude", "worktrees", "agent")
	runGit(t, repo, "worktree", "add", "-b", "agent-candidate", agent, "main")
	worktrees, err = global.List()
	require.NoError(t, err)
	for _, tc := range []struct {
		use  NameUse
		path string
	}{
		{RemoveName, added.Path}, {PromoteName, agent}, {SharedPrefix, agent},
	} {
		resolved, err := global.ResolveWorktree("agent", worktrees, tc.use)
		require.NoError(t, err)
		require.Equal(t, tc.path, resolved.Path)
	}
}

func TestWorktreeAmbiguityRecovery(t *testing.T) {
	client := rootClient(t, initTestRepo(t), "")
	worktrees := []Info{
		{Path: filepath.Join(physicalTempDir(t), "dup"), IsLocked: true, LockReason: globalRootLockReason},
		{Path: filepath.Join(physicalTempDir(t), "dup"), IsLocked: true, LockReason: globalRootLockReason},
	}
	for _, use := range []NameUse{DisplayName, RemoveName, PromoteName, SharedPrefix} {
		_, err := client.ResolveWorktree("dup", worktrees, use)
		require.ErrorContains(t, err, "ambiguous worktree")
		for _, wt := range worktrees {
			require.ErrorContains(t, err, wt.Path)
		}
		if use == SharedPrefix {
			require.NotContains(t, err.Error(), "absolute path")
			continue
		}
		require.ErrorContains(t, err, "use an absolute path")
		for _, wt := range worktrees {
			resolved, err := client.ResolveWorktree(wt.Path, worktrees, use)
			require.NoError(t, err)
			require.Equal(t, wt.Path, resolved.Path)
		}
	}
}
