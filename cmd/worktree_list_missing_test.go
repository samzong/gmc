package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeListMissing(t *testing.T) {
	for _, name := range []string{"prunable", "locked"} {
		t.Run(name, func(t *testing.T) {
			repo := initCmdTestRepo(t)
			missing := repo + "--gone"
			runGitCmd(t, repo, "worktree", "add", "-b", "gone", missing)
			if name == "locked" {
				runGitCmd(t, repo, "worktree", "lock", missing)
			}
			require.NoError(t, os.RemoveAll(missing))
			t.Chdir(repo)
			client := worktree.NewClient(worktree.Options{})
			for _, format := range []string{"text", "json"} {
				t.Run(format, func(t *testing.T) {
					setTestValue(t, &outputFlag.value, format)
					var out, warnings bytes.Buffer
					withWriters(t, &out, &warnings)
					require.NoError(t, runWorktreeList(client, false))
					assert.Empty(t, warnings.String())
					if format == "text" {
						assert.Regexp(t, `(?m)gone +[0-9a-f]{7} +- +missing$`, out.String())
						return
					}
					var items []WorktreeJSON
					require.NoError(t, json.Unmarshal(out.Bytes(), &items))
					require.Len(t, items, 2)
					assert.Equal(t, missing, items[1].Path)
					assert.Equal(t, "missing", items[1].Status)
					assert.Equal(t, name == "prunable", items[1].Prunable)
					assert.Nil(t, items[1].AllocatedBytes)
					require.Len(t, items[1].Issues, 1)
					assert.Equal(t, worktree.MeasureNotFound, items[1].Issues[0].Code)
				})
			}
		})
	}
}

func TestWorktreeListAllMissing(t *testing.T) {
	repo := initCmdTestRepo(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	linked := filepath.Join(home, "linked")
	missing := filepath.Join(home, "gone")
	locked := filepath.Join(home, "lockgone")
	runGitCmd(t, repo, "worktree", "add", "-b", "linked", linked)
	runGitCmd(t, repo, "worktree", "add", "-b", "gone", missing)
	runGitCmd(t, repo, "worktree", "add", "-b", "lockgone", locked)
	runGitCmd(t, repo, "worktree", "lock", locked)
	require.NoError(t, os.RemoveAll(missing))
	require.NoError(t, os.RemoveAll(locked))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			setTestValue(t, &outputFlag.value, format)
			var out, progress bytes.Buffer
			command := &cobra.Command{Use: "list", Args: wtListCmd.Args, RunE: wtListCmd.RunE}
			command.Flags().BoolP("all", "A", false, "")
			command.SetOut(&out)
			command.SetErr(&progress)
			command.SetArgs([]string{"--all"})
			require.NoError(t, command.Execute())
			assert.NotContains(t, progress.String(), "Warning")
			if format == "text" {
				assert.Regexp(t, `(?m)^~/gone +gone +[0-9a-f]{7} +- +missing, prunable$`, out.String())
				assert.Regexp(t, `(?m)^~/lockgone +lockgone +[0-9a-f]{7} +- +missing, locked$`, out.String())
				return
			}
			var items []WorktreeJSON
			require.NoError(t, json.Unmarshal(out.Bytes(), &items))
			require.Len(t, items, 4)
			found := 0
			for _, item := range items {
				if item.Path != missing && item.Path != locked {
					continue
				}
				found++
				assert.Equal(t, "missing", item.Status)
				assert.Equal(t, item.Path == missing, item.Prunable)
				assert.Equal(t, item.Path == locked, item.Locked)
				assert.Nil(t, item.AllocatedBytes)
				require.Len(t, item.Issues, 1)
				assert.Equal(t, worktree.MeasureNotFound, item.Issues[0].Code)
			}
			assert.Equal(t, 2, found)
		})
	}
}

func TestWorktreeListSizePermissionFailure(t *testing.T) {
	path := t.TempDir()
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { _ = os.Chmod(path, 0700) })
	if _, err := os.ReadDir(path); err == nil {
		t.Skip("the current user can read directories without permission")
	}
	var warnings bytes.Buffer
	sizes := loadWorktreeSizes(&warnings, []worktree.Info{{Path: path}})
	assert.Contains(t, warnings.String(), "cannot measure size of ")
	assert.Nil(t, sizes.bytes(path))
	require.Len(t, sizes.issues(path), 1)
	assert.Equal(t, worktree.MeasurePermissionDenied, sizes.issues(path)[0].Code)
}
