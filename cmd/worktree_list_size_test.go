package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
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

	run := func(t *testing.T, format string) (string, string) {
		t.Helper()
		setTestValue(t, &outputFlag.value, format)
		var out, errOut bytes.Buffer
		withWriters(t, &out, &errOut)
		require.NoError(t, runWorktreeList(worktree.NewClient(worktree.Options{}), false))
		return out.String(), errOut.String()
	}

	sized, errOut := run(t, "text")
	assert.Empty(t, errOut)
	assert.Equal(t, []string{"NAME", "BRANCH", "COMMIT", "SIZE", "STATUS"}, headerColumns(t, sized, "NAME"))
	assert.Regexp(t, regexp.MustCompile(`(?m)feature-wt +feature/size +[0-9a-f]{7} +[0-9.]+[KM] +1 untracked$`), sized)

	var items []WorktreeJSON
	rawSized, _ := run(t, "json")
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

func assertColumnsAligned(t *testing.T, lines []string) {
	t.Helper()
	sizeEnds := map[int]bool{}
	statusStarts := map[int]bool{}
	for _, line := range lines {
		fields := strings.Fields(line)
		status := strings.LastIndex(line, fields[len(fields)-1])
		size := strings.LastIndex(line[:status], fields[len(fields)-2]) + len(fields[len(fields)-2])
		sizeEnds[runewidth.StringWidth(line[:size])] = true
		statusStarts[runewidth.StringWidth(line[:status])] = true
	}
	assert.Len(t, sizeEnds, 1, strings.Join(lines, "\n"))
	assert.Len(t, statusStarts, 1, strings.Join(lines, "\n"))
}

func TestWorktreeListSizeAlignsWideNames(t *testing.T) {
	repo := initCmdTestRepo(t)
	parent := filepath.Dir(repo)
	runGitCmd(t, repo, "worktree", "add", "-b", "功能/测试", filepath.Join(parent, "功能-测试"), "main")
	runGitCmd(t, repo, "worktree", "add", "-b", "feature/ascii", filepath.Join(parent, "feature-ascii"), "main")
	t.Chdir(repo)
	setTestValue(t, &outputFlag.value, "text")
	var out bytes.Buffer
	withWriters(t, &out, &out)
	require.NoError(t, runWorktreeList(worktree.NewClient(worktree.Options{}), false))

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 4)
	assert.Contains(t, out.String(), "功能-测试")
	assertColumnsAligned(t, lines)
}

func TestWorktreeListAllAlignsWideNames(t *testing.T) {
	repo := initCmdTestRepo(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	agents := filepath.Join(home, ".codex", "worktrees")
	runGitCmd(t, repo, "worktree", "add", "-b", "功能/测试", filepath.Join(agents, "功能-测试"))
	runGitCmd(t, repo, "worktree", "add", "-b", "feature/ascii", filepath.Join(agents, "feature-ascii"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	setTestValue(t, &outputFlag.value, "text")

	var out, progress bytes.Buffer
	command := &cobra.Command{Use: "list", Args: wtListCmd.Args, RunE: wtListCmd.RunE}
	command.Flags().BoolP("all", "A", false, "")
	command.SetOut(&out)
	command.SetErr(&progress)
	command.SetArgs([]string{"-A"})
	require.NoError(t, command.Execute())

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 5)
	assert.Contains(t, out.String(), "~/.codex/worktrees/功能-测试")
	assertColumnsAligned(t, lines[1:])
}
