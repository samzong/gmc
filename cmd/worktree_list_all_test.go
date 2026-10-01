package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeListAllRejectsCurrentRepositoryFlags(t *testing.T) {
	tests := []struct {
		args     []string
		conflict string
	}{
		{[]string{"-A", "--pr"}, "pr"},
		{[]string{"--all", "--diff-base", "main"}, "diff-base"},
		{[]string{"-A"}, ""},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Cleanup(func() {
				for _, name := range []string{"all", "pr", "diff-base"} {
					flag := wtListCmd.Flags().Lookup(name)
					require.NoError(t, flag.Value.Set(flag.DefValue))
					flag.Changed = false
				}
			})
			require.NoError(t, wtListCmd.ParseFlags(tt.args))
			err := wtListCmd.ValidateFlagGroups()
			if tt.conflict == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "all")
			assert.Contains(t, err.Error(), tt.conflict)
		})
	}
}

func TestWorktreeListAllOutsideRepository(t *testing.T) {
	repo := initCmdTestRepo(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	linked := filepath.Join(home, ".codex", "worktrees", "feature")
	runGitCmd(t, repo, "worktree", "add", "-b", "feature", linked)
	require.NoError(t, os.WriteFile(filepath.Join(linked, "untracked.txt"), []byte("new"), 0o600))
	standalone := filepath.Join(home, "standalone")
	runGitCmd(t, home, "clone", repo, standalone)
	require.NoError(t, os.WriteFile(filepath.Join(standalone, "README.md"), []byte("changed"), 0o600))
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
			command.SetArgs([]string{"-A"})
			require.NoError(t, command.Execute())
			assert.Contains(t, progress.String(), "Scanning")
			assert.NotContains(t, out.String(), "Scanning")
			assert.NotContains(t, out.String(), "standalone")
			if format == "text" {
				assert.Contains(t, out.String(), repo)
				assert.Contains(t, out.String(), "~/.codex/worktrees/feature")
				assert.Contains(t, out.String(), "1 untracked")
				return
			}
			var items []WorktreeJSON
			require.NoError(t, json.Unmarshal(out.Bytes(), &items))
			require.Len(t, items, 2)
			assert.Equal(t, repo, items[0].Repository)
			assert.Equal(t, linked, items[1].Path)
			assert.Equal(t, "1 untracked", items[1].Status)
		})
	}
}
