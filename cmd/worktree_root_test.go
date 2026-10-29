package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeGlobalRootOutput(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(strconv.FormatBool(available), func(t *testing.T) {
			repo := initCmdTestRepo(t)
			t.Chdir(repo)
			root := t.TempDir()
			if !available {
				root = filepath.Join(root, "unmounted")
			}
			cfg := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(cfg, []byte(fmt.Sprintf("worktree:\n  root: %q\n", root)), 0600))
			setTestValue(t, &outputFlag.value, "json")
			setTestValue(t, &wtBaseBranch, "main")
			var out, errOut bytes.Buffer
			withWriters(t, &out, &errOut)
			client := worktree.NewClient(worktree.Options{Dir: repo, GlobalConfigPath: cfg})
			require.NoError(t, runWorktreeAdd(client, []string{"feat/x"}))
			var items []WorktreeCreateJSON
			require.NoError(t, json.Unmarshal(out.Bytes(), &items))
			require.Len(t, items, 1)
			assert.True(t, items[0].Created)
			if available {
				assert.Contains(t, items[0].Path, root+string(filepath.Separator))
				assert.NotContains(t, errOut.String(), "unavailable")
			} else {
				assert.Equal(t, repo+"--feat--x", items[0].Path)
				assert.Contains(t, errOut.String(), "Warning: worktree root "+root+" is unavailable")
				assert.Len(t, items[0].Warnings, 1)
				_, err := os.Stat(root)
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestGlobalRootListAfterConfigurationRemoval(t *testing.T) {
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte(fmt.Sprintf("worktree:\n  root: %q\n", root)), 0600))
	client := worktree.NewClient(worktree.Options{Dir: repo, GlobalConfigPath: cfg})
	created, err := client.Add("feat/list", worktree.AddOptions{})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg, []byte("{}\n"), 0600))
	worktrees, err := client.List()
	require.NoError(t, err)
	var global worktree.Info
	for _, wt := range worktrees {
		if wt.Path == created.Path {
			global = wt
		}
	}
	require.NotEmpty(t, global.Path)
	require.NoError(t, os.WriteFile(filepath.Join(created.Path, "changed"), []byte("preserve"), 0600))
	assert.Equal(t, "1 untracked", resolveWorktreeStatus(client, getDisplayRoot(client), global))
	names, err := resolveAllRemovableWorktrees(client)
	require.NoError(t, err)
	assert.Contains(t, names, displayWorktreeName(getDisplayRoot(client), created.Path))
	moved := created.Path + "-unmounted"
	require.NoError(t, os.Rename(created.Path, moved))
	assert.Equal(t, "missing", resolveWorktreeStatus(client, getDisplayRoot(client), global))
	require.NoError(t, os.Rename(moved, created.Path))
}

func TestGlobalRootDupHumanPath(t *testing.T) {
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte(fmt.Sprintf("worktree:\n  root: %q\n", root)), 0600))
	setTestValue(t, &outputFlag.value, "text")
	setTestValue(t, &wtDupBase, "main")
	var out, errOut bytes.Buffer
	withWriters(t, &out, &errOut)
	client := worktree.NewClient(worktree.Options{Dir: repo, GlobalConfigPath: cfg})
	require.NoError(t, runWorktreeDup(client, []string{"1"}))
	assert.Contains(t, out.String(), "  "+root+string(filepath.Separator))
	assert.NotContains(t, out.String(), "../")
	assert.NotContains(t, out.String(), " (")
}
