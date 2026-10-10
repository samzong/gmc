package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/exitcode"
	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeAddJSONReportsEachName(t *testing.T) {
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "gmc-share.yml"),
		[]byte("hooks:\n  - cmd: echo hook-ran\n"), 0o644))
	blocked := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"--b")
	require.NoError(t, os.Mkdir(blocked, 0o755))
	setTestValue(t, &outputFlag.value, "json")
	setTestValue(t, &wtBaseBranch, "main")

	var out, errOut bytes.Buffer
	withWriters(t, &out, &errOut)
	err := runWorktreeAdd(newWorktreeClient(), []string{"a", "b"})
	require.Error(t, err)

	var items []WorktreeCreateJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &items), out.String())
	require.Len(t, items, 2)
	assert.Equal(t, WorktreeCreateJSON{
		Name: "a", Path: filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"--a"),
		Branch: "a", Base: "main", Created: true,
	}, items[0])
	assert.Equal(t, "b", items[1].Name)
	assert.Equal(t, blocked, items[1].Path)
	assert.False(t, items[1].Created)
	assert.Contains(t, items[1].Error, "directory already exists")
	assert.Contains(t, errOut.String(), "hook-ran")
	assert.Contains(t, errOut.String(), "Created worktree 'a'")
	assert.Contains(t, errOut.String(), "Error adding 'b'")
}

func TestWorktreeDupJSON(t *testing.T) {
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	setTestValue(t, &outputFlag.value, "json")
	setTestValue(t, &wtDupBase, "main")

	var out, errOut bytes.Buffer
	withWriters(t, &out, &errOut)
	require.NoError(t, runWorktreeDup(newWorktreeClient(), []string{"2"}))

	var items []WorktreeCreateJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &items), out.String())
	require.Len(t, items, 2)
	for i, item := range items {
		assert.Equal(t, ".dup-"+string(rune('1'+i)), item.Name)
		assert.True(t, item.Created)
		assert.Equal(t, "main", item.Base)
		assert.True(t, strings.HasPrefix(item.Branch, "_dup/main/"), item.Branch)
		assert.DirExists(t, item.Path)
	}

	out.Reset()
	require.Error(t, runWorktreeDup(newWorktreeClient(), []string{"1"}))
	var failed []WorktreeCreateJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &failed), out.String())
	require.Len(t, failed, 1)
	assert.False(t, failed[0].Created)
	assert.Contains(t, failed[0].Error, "directory already exists")
}

func TestWorktreeListJSONIssues(t *testing.T) {
	repoDir, client, _ := newWorktreeOutputTest(t)
	locked := filepath.Join(repoDir, "locked")
	require.NoError(t, os.Mkdir(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	missing := filepath.Join(t.TempDir(), "gone")
	worktrees := []worktree.Info{
		{Path: repoDir, Branch: "main", Commit: strings.Repeat("a", 40)},
		{Path: missing, Branch: "gone", Commit: strings.Repeat("b", 40)},
	}
	var warnings bytes.Buffer
	items := buildWorktreeJSON(client, worktrees, nil, nil, loadWorktreeSizes(&warnings, worktrees))

	require.Len(t, items[0].Issues, 1)
	assert.Equal(t, "size", items[0].Issues[0].Source)
	assert.Equal(t, worktree.MeasurePermissionDenied, items[0].Issues[0].Code)
	assert.Contains(t, items[0].Issues[0].Message, locked)
	assert.Nil(t, items[0].AllocatedBytes)
	require.Len(t, items[1].Issues, 1)
	assert.Equal(t, worktree.MeasureNotFound, items[1].Issues[0].Code)
	assert.Equal(t, 2, strings.Count(warnings.String(), "Warning: cannot measure size of "))

	clean := buildWorktreeJSON(client, worktrees[:1], nil, nil, worktreeSizes{repoDir: {Path: repoDir, Bytes: 1}})
	data, err := json.Marshal(clean[0])
	require.NoError(t, err)
	assert.NotContains(t, string(data), "issues")
}

func TestWorktreeAddPRJSON(t *testing.T) {
	remote := initCmdTestRepo(t)
	runGitCmd(t, remote, "update-ref", "refs/pull/42/head", "HEAD")
	repo := initCmdTestRepo(t)
	runGitCmd(t, repo, "remote", "add", "origin", remote)
	t.Chdir(repo)
	setTestValue(t, &outputFlag.value, "json")

	var out, errOut bytes.Buffer
	withWriters(t, &out, &errOut)
	require.NoError(t, runWorktreeAddPR(newWorktreeClient(), 42))
	var items []WorktreeCreateJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &items), out.String())
	assert.Equal(t, []WorktreeCreateJSON{{
		Name: "pr/42", Path: repo + "--pr--42", Branch: "pr/42", Base: "origin/pull/42/head", Created: true,
	}}, items)
	assert.Contains(t, errOut.String(), "Created PR worktree 'pr/42'")

	out.Reset()
	err := runWorktreeAddPR(newWorktreeClient(), 7)
	require.Error(t, err)
	var failed []WorktreeCreateJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &failed), out.String())
	assert.Equal(t, []WorktreeCreateJSON{{
		Name: "pr/7", Error: "PR #7 not found on remote 'origin'",
	}}, failed)
}

func TestWorktreeAddJSONSyncFailureListsEveryName(t *testing.T) {
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	setTestValue(t, &outputFlag.value, "json")
	setTestValue(t, &wtAddSync, true)
	setTestValue(t, &wtBaseBranch, "main")

	var out, errOut bytes.Buffer
	withWriters(t, &out, &errOut)
	err := runWorktreeAdd(newWorktreeClient(), []string{"a", "b"})
	require.Error(t, err)
	var exitErr *exitcode.Error
	assert.False(t, errors.As(err, &exitErr), "sync failure must map to exit code %d", exitcode.General)

	var items []WorktreeCreateJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &items), out.String())
	assert.Equal(t, []WorktreeCreateJSON{
		{Name: "a", Error: err.Error()},
		{Name: "b", Error: err.Error()},
	}, items)
	assert.NoDirExists(t, filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"--a"))
}
