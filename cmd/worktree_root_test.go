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
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
	for name, dup := range map[string]bool{"missing-root": false, "dup-and-list": true} {
		t.Run(name, func(t *testing.T) {
			repo, root, client := cmdRootClient(t, dup)
			if dup {
				root = filepath.Join(home, "worktrees")
				require.NoError(t, os.Mkdir(root, 0o755))
				cfg := filepath.Join(repo, "config.yaml")
				require.NoError(t, os.WriteFile(cfg, []byte(fmt.Sprintf("worktree:\n  root: %q\n", root)), 0600))
				client = worktree.NewClient(worktree.Options{Dir: repo, GlobalConfigPath: cfg})
			}
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
			require.NoError(t, runWorktreeDup(client, []string{"2"}))
			assert.Contains(t, out.String(), "  "+root+string(filepath.Separator))
			assert.NotContains(t, out.String(), "../")
			assert.NotContains(t, out.String(), " (")
			cfg := filepath.Join(repo, "config.yaml")
			require.NoError(t, os.WriteFile(cfg, []byte("{}\n"), 0600))
			client = worktree.NewClient(worktree.Options{Dir: repo, GlobalConfigPath: cfg})
			worktrees, err := client.List()
			require.NoError(t, err)
			require.Len(t, worktrees, 3)
			global := worktrees[1]
			require.NoError(t, os.WriteFile(filepath.Join(global.Path, "changed"), []byte("preserve"), 0600))
			assert.Equal(t, "1 untracked", resolveWorktreeStatus(client, getDisplayRoot(client), global))
			names, err := resolveAllRemovableWorktrees(client)
			require.NoError(t, err)
			assert.Contains(t, names, filepath.Base(global.Path))
			require.Len(t, names, 2)
			result := client.RemoveBatch(names, worktree.RemoveOptions{DryRun: true})
			require.Empty(t, result.Failed, "%v", result.Failed)
			assert.ElementsMatch(t, names, result.Succeeded)
			for _, wt := range worktrees {
				if client.IsGlobalWorktree(wt) {
					assert.Contains(t, result.Report.Events, worktree.Event{
						Level: worktree.EventWarn, Message: "Would remove worktree: " + wt.Path,
					})
					t.Logf("resolved global worktree: %s", wt.Path)
				}
				assert.DirExists(t, wt.Path)
			}
			completions, _ := completeWorktreeNames(nil, nil, "")
			assert.Contains(t, completions, filepath.Base(global.Path))
			require.NoError(t, os.Rename(global.Path, global.Path+"-unmounted"))
			assert.Equal(t, "missing", resolveWorktreeStatus(client, getDisplayRoot(client), global))
		})
	}
}

func TestWorktreeSwitchNamesMatchList(t *testing.T) {
	_, _, client := cmdRootClient(t, true)
	created, err := client.Add("feature", worktree.AddOptions{BaseBranch: "main"})
	require.NoError(t, err)
	worktrees, err := client.List()
	require.NoError(t, err)
	worktrees = filterBareWorktrees(worktrees)
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprintf("ambiguous=%t", ambiguous), func(t *testing.T) {
			items := append([]worktree.Info(nil), worktrees...)
			expected := filepath.Base(created.Path)
			if ambiguous {
				for _, wt := range worktrees {
					if wt.Path == created.Path {
						wt.Path = filepath.Join(t.TempDir(), filepath.Base(wt.Path))
						items = append(items, wt)
						break
					}
				}
				expected = created.Path
			}
			listed := buildWorktreeJSON(client, items, nil, nil, nil)
			options := buildWorktreeSwitchOptions(client, items)
			require.Len(t, options, len(listed))
			for i, item := range listed {
				assert.Equal(t, item.Name+" ("+item.Branch+")", options[i].Key)
				assert.Equal(t, item.Path, options[i].Value)
				if item.Path == created.Path {
					assert.Equal(t, expected, item.Name)
				}
			}
		})
	}
}
