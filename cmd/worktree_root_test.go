package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cmdRootClient(t *testing.T, available bool) (string, string, *worktree.Client) {
	t.Helper()
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	if !available {
		root = filepath.Join(root, "unmounted")
	}
	cfg := filepath.Join(repo, "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte(fmt.Sprintf("worktree:\n  root: %q\n", root)), 0600))
	return repo, root, worktree.NewClient(worktree.Options{Dir: repo, GlobalConfigPath: cfg})
}

func TestGlobalRootCLIOutput(t *testing.T) {
	for name, dup := range map[string]bool{"missing-root": false, "dup-and-list": true} {
		t.Run(name, func(t *testing.T) {
			repo, root, client := cmdRootClient(t, dup)
			setTestValue(t, &outputFlag.value, "json")
			setTestValue(t, &wtBaseBranch, "main")
			setTestValue(t, &wtDupBase, "main")
			var out, errOut bytes.Buffer
			withWriters(t, &out, &errOut)
			if !dup {
				require.NoError(t, runWorktreeAdd(client, []string{"feat/x"}))
				var items []WorktreeCreateJSON
				require.NoError(t, json.Unmarshal(out.Bytes(), &items))
				require.Len(t, items, 1)
				assert.True(t, items[0].Created)
				assert.Equal(t, repo+"--feat--x", items[0].Path)
				require.Len(t, items[0].Warnings, 1)
				assert.Contains(t, errOut.String(), items[0].Warnings[0])
				assert.Contains(t, items[0].Warnings[0], "Warning: worktree root "+root+" is unavailable")
				_, err := os.Stat(root)
				require.ErrorIs(t, err, os.ErrNotExist)
				return
			}
			setTestValue(t, &outputFlag.value, "text")
			require.NoError(t, runWorktreeDup(client, []string{"1"}))
			assert.Contains(t, out.String(), "  "+root+string(filepath.Separator))
			assert.NotContains(t, out.String(), "../")
			assert.NotContains(t, out.String(), " (")
			cfg := filepath.Join(repo, "config.yaml")
			require.NoError(t, os.WriteFile(cfg, []byte("{}\n"), 0600))
			client = worktree.NewClient(worktree.Options{Dir: repo, GlobalConfigPath: cfg})
			worktrees, err := client.List()
			require.NoError(t, err)
			require.Len(t, worktrees, 2)
			global := worktrees[1]
			require.NoError(t, os.WriteFile(filepath.Join(global.Path, "changed"), []byte("preserve"), 0600))
			assert.Equal(t, "1 untracked", resolveWorktreeStatus(client, getDisplayRoot(client), global))
			names, err := resolveAllRemovableWorktrees(client)
			require.NoError(t, err)
			assert.Contains(t, names, displayWorktreeName(getDisplayRoot(client), global.Path))
			require.NoError(t, os.Rename(global.Path, global.Path+"-unmounted"))
			assert.Equal(t, "missing", resolveWorktreeStatus(client, getDisplayRoot(client), global))
		})
	}
}
