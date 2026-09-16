package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRustCacheCreation(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested=%t", nested), func(t *testing.T) {
			t.Setenv("CARGO_HOME", t.TempDir())
			repo := initTestRepo(t)
			t.Chdir(repo)
			roots := []string{"."}
			if nested {
				roots = []string{"apps/gateway", "tools/worker"}
			}
			for _, root := range roots {
				require.NoError(t, os.MkdirAll(filepath.Join(repo, root, "!child"), 0755))
				writeFile(t, filepath.Join(repo, root, "Cargo.toml"), "[workspace]\nmembers = ['!child']\n")
				writeFile(t, filepath.Join(repo, root, "!child", "Cargo.toml"), "[package]\nname = 'member'\nversion = '0.1.0'\n")
			}
			if nested {
				require.NoError(t, os.MkdirAll(filepath.Join(repo, "custom", ".cargo"), 0755))
				require.NoError(t, os.MkdirAll(filepath.Join(repo, "target", "fixture"), 0755))
				writeFile(t, filepath.Join(repo, "custom", "Cargo.toml"), "[workspace]\n")
				writeFile(t, filepath.Join(repo, "custom", ".cargo", "config.toml"), "[build]\njobs = 2\n")
				writeFile(t, filepath.Join(repo, "target", "fixture", "Cargo.toml"), "[workspace]\n")
			}
			commitFiles(t, repo, "Add Rust package", ".")
			previous := ensureRustCache
			helper := filepath.Join(t.TempDir(), "gmc-rustc")
			ensureRustCache = func() (string, error) {
				return helper, os.WriteFile(helper, []byte("test adapter"), 0755)
			}
			t.Cleanup(func() { ensureRustCache = previous })
			client := NewClient(Options{})
			_, err := client.Add("cache-test", AddOptions{})
			require.NoError(t, err)
			target := repo + "--cache-test"
			t.Cleanup(func() {
				_, err := client.Remove(target, RemoveOptions{Force: true, DeleteBranch: true})
				require.NoError(t, err)
			})
			for _, root := range roots {
				require.FileExists(t, filepath.Join(target, root, ".cargo", "config.toml"))
				require.NoDirExists(t, filepath.Join(target, root, "!child", ".cargo"))
				require.NoDirExists(t, filepath.Join(repo, root, ".cargo"))
			}
			if nested {
				data, err := os.ReadFile(filepath.Join(target, "custom", ".cargo", "config.toml"))
				require.NoError(t, err)
				require.Equal(t, "[build]\njobs = 2\n", string(data))
				require.NoDirExists(t, filepath.Join(target, "target", "fixture", ".cargo"))
				require.NoDirExists(t, filepath.Join(target, ".cargo"))
			}
			require.Empty(t, runGit(t, target, "status", "--porcelain"))
			require.NoDirExists(t, filepath.Join(repo, ".cargo"))
		})
	}
}
