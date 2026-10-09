package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func headerColumns(t *testing.T, output, first string) []string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, first) {
			return strings.Fields(line)
		}
	}
	t.Fatalf("no header starting with %q in:\n%s", first, output)
	return nil
}

func TestWorktreeListSizeColumn(t *testing.T) {
	repo := initCmdTestRepo(t)
	linked := filepath.Join(t.TempDir(), "feature-wt")
	runGitCmd(t, repo, "worktree", "add", "-b", "feature/size", linked, "main")
	require.NoError(t, os.WriteFile(filepath.Join(linked, "data.bin"), make([]byte, 64*1024), 0o600))
	t.Chdir(repo)

	run := func(t *testing.T, size bool, format string) (string, string) {
		t.Helper()
		setTestValue(t, &wtShowSize, size)
		setTestValue(t, &outputFlag.value, format)
		var out, errOut bytes.Buffer
		withWriters(t, &out, &errOut)
		require.NoError(t, runWorktreeList(worktree.NewClient(worktree.Options{}), false))
		return out.String(), errOut.String()
	}

	plain, _ := run(t, false, "text")
	assert.NotContains(t, plain, "SIZE")
	assert.Regexp(t, regexp.MustCompile(`(?m)^NAME +BRANCH +COMMIT +STATUS$`), plain)

	sized, errOut := run(t, true, "text")
	assert.Empty(t, errOut)
	assert.Equal(t, []string{"NAME", "BRANCH", "COMMIT", "SIZE", "STATUS"}, headerColumns(t, sized, "NAME"))
	assert.Regexp(t, regexp.MustCompile(`(?m)feature-wt +feature/size +[0-9a-f]{7} +[0-9.]+[KM] +1 untracked$`), sized)

	var items []WorktreeJSON
	rawPlain, _ := run(t, false, "json")
	assert.NotContains(t, rawPlain, "allocated_bytes")
	rawSized, _ := run(t, true, "json")
	require.NoError(t, json.Unmarshal([]byte(rawSized), &items))
	require.Len(t, items, 2)
	for _, item := range items {
		require.NotNil(t, item.AllocatedBytes, item.Path)
	}
	assert.GreaterOrEqual(t, *items[1].AllocatedBytes, uint64(64*1024))
}

func TestWorktreeTableSizeAfterPRAndFailureRow(t *testing.T) {
	repoDir, client, out := newWorktreeOutputTest(t)
	missing := filepath.Join(repoDir, "gone")
	var warnings bytes.Buffer
	setTestValue(t, &wtShowSize, true)
	worktrees := []worktree.Info{
		{Path: repoDir, Branch: "feature/ok", Commit: strings.Repeat("a", 40)},
		{Path: missing, Branch: "feature/gone", Commit: strings.Repeat("b", 40)},
	}
	sizes := loadWorktreeSizes(&warnings, worktrees)

	printWorktreeTable(client, worktrees, map[string]worktree.ReviewInfo{
		"feature/ok": {Number: 7, State: "OPEN"},
	}, nil, sizes)

	assert.Equal(t, []string{"NAME", "BRANCH", "COMMIT", "PR", "SIZE", "STATUS"}, headerColumns(t, out.String(), "NAME"))
	assert.Regexp(t, regexp.MustCompile(`(?m)feature/gone +bbbbbbb +- +- +`), out.String())
	assert.Equal(t, 1, strings.Count(warnings.String(), "Warning: "))
	assert.Contains(t, warnings.String(), "cannot measure size of "+abbrevPath(missing))

	items := buildWorktreeJSON(client, worktrees, nil, nil, sizes)
	require.NotNil(t, items[0].AllocatedBytes)
	assert.Nil(t, items[1].AllocatedBytes)
	data, err := json.Marshal(items[1])
	require.NoError(t, err)
	assert.NotContains(t, string(data), "allocated_bytes")
}

func TestWorktreeListAllSize(t *testing.T) {
	repo := initCmdTestRepo(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	linked := filepath.Join(home, ".codex", "worktrees", "feature")
	runGitCmd(t, repo, "worktree", "add", "-b", "feature", linked)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	setTestValue(t, &wtShowSize, true)

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
			assert.NotContains(t, progress.String(), "Warning")
			if format == "text" {
				assert.Equal(t, []string{"PATH", "BRANCH", "COMMIT", "SIZE", "STATUS"}, headerColumns(t, out.String(), "PATH"))
				row := regexp.MustCompile(`(?m)^~/.codex/worktrees/feature +feature +[0-9a-f]{7} +[0-9.]+[BKM] +clean$`)
				assert.Regexp(t, row, out.String())
				return
			}
			var items []WorktreeJSON
			require.NoError(t, json.Unmarshal(out.Bytes(), &items))
			require.Len(t, items, 2)
			for _, item := range items {
				require.NotNil(t, item.AllocatedBytes, item.Path)
			}
		})
	}
}
