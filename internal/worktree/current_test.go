package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentWorktree(t *testing.T) {
	root := physicalTempDir(t)
	repo := filepath.Join(root, "repo")
	nested := filepath.Join(repo, "team", "feat")
	claude := filepath.Join(repo, ".claude", "worktrees", "agent")
	codex := filepath.Join(repo, ".codex", "worktrees", "agent")
	sibling := filepath.Join(root, "repo--a")
	global := filepath.Join(root, "global", "repo--global")
	bare := filepath.Join(root, ".bare")
	flaggedBare := filepath.Join(root, "bare-repository")
	outside := filepath.Join(root, "outside")
	worktrees := []Info{
		{Path: repo}, {Path: nested}, {Path: claude}, {Path: codex},
		{Path: sibling}, {Path: global}, {Path: bare}, {Path: flaggedBare, IsBare: true},
	}
	for _, path := range []string{repo, nested, claude, codex, sibling, global, bare, flaggedBare, outside} {
		require.NoError(t, os.MkdirAll(filepath.Join(path, "sub", "dir"), 0o755))
	}
	alias := filepath.Join(root, "longer-symlink-to-repo")
	require.NoError(t, os.Symlink(repo, alias))
	client := NewClient(Options{})
	for _, tc := range []struct{ name, cwd, want string }{
		{"root", repo, repo},
		{"nested", nested, nested},
		{"nested subdirectory", filepath.Join(nested, "sub", "dir"), nested},
		{"claude", claude, claude},
		{"codex", codex, codex},
		{"global", global, global},
		{"sibling prefix", sibling, sibling},
		{"sibling subdirectory", filepath.Join(sibling, "sub", "dir"), sibling},
		{"symlink cwd", filepath.Join(alias, "team", "feat", "sub"), nested},
		{"bare directory", bare, ""},
		{"bare flag", flaggedBare, ""},
		{"outside", outside, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(tc.cwd)
			for _, reverse := range []bool{false, true} {
				list := append([]Info(nil), worktrees...)
				if reverse {
					for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
						list[i], list[j] = list[j], list[i]
					}
				}
				got, ok := client.CurrentWorktree(list)
				require.Equal(t, tc.want != "", ok)
				require.Equal(t, tc.want, got.Path)
			}
		})
	}
	t.Run("symlink ancestor", func(t *testing.T) {
		t.Chdir(filepath.Join(nested, "sub"))
		got, ok := client.CurrentWorktree([]Info{{Path: alias}, {Path: nested}})
		require.True(t, ok)
		require.Equal(t, nested, got.Path)
	})
	t.Run("symlink worktree", func(t *testing.T) {
		t.Chdir(filepath.Join(repo, "sub"))
		got, ok := client.CurrentWorktree([]Info{{Path: alias}})
		require.True(t, ok)
		require.Equal(t, alias, got.Path)
	})
}
