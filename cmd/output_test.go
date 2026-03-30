package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputFormatFlag_RejectsInvalid(t *testing.T) {
	f := &outputFormatFlag{value: "text"}
	err := f.Set("xml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be text or json")
	assert.Equal(t, "text", f.String())
}

func TestOutputFormatFlag_AcceptsValid(t *testing.T) {
	f := &outputFormatFlag{value: "text"}
	require.NoError(t, f.Set("json"))
	assert.Equal(t, "json", f.String())
	require.NoError(t, f.Set("text"))
	assert.Equal(t, "text", f.String())
}

func TestOutputFormatFlag_Type(t *testing.T) {
	f := &outputFormatFlag{value: "text"}
	assert.Equal(t, "string", f.Type())
}

func TestPrintJSON_RoundTrip(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
		OK   bool   `json:"ok"`
	}

	var buf bytes.Buffer
	err := printJSON(&buf, sample{Name: "test", OK: true})
	require.NoError(t, err)

	var got sample
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, "test", got.Name)
	assert.True(t, got.OK)
}

func TestPrintJSON_EmptySlice(t *testing.T) {
	var buf bytes.Buffer
	err := printJSON(&buf, []string{})
	require.NoError(t, err)
	assert.Equal(t, "[]\n", buf.String())
}

func TestPrintJSON_NilSlice(t *testing.T) {
	var buf bytes.Buffer
	err := printJSON(&buf, []string(nil))
	require.NoError(t, err)
	assert.Equal(t, "null\n", buf.String())
}

func withOutputFormat(t *testing.T, format string) {
	t.Helper()
	old := outputFlag.value
	outputFlag.value = format
	t.Cleanup(func() { outputFlag.value = old })
}

func withWriters(t *testing.T, out, errw io.Writer) {
	t.Helper()
	oldOut := outWriterFunc
	oldErr := errWriterFunc
	outWriterFunc = func() io.Writer { return out }
	errWriterFunc = func() io.Writer { return errw }
	t.Cleanup(func() {
		outWriterFunc = oldOut
		errWriterFunc = oldErr
	})
}

func TestRunWorktreeList_JSON(t *testing.T) {
	repoDir := initCmdTestRepo(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feature/json-test", linkedWt, "main")

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	var out bytes.Buffer
	withWriters(t, &out, io.Discard)
	withOutputFormat(t, "json")

	client := worktree.NewClient(worktree.Options{})
	err = runWorktreeList(client)
	require.NoError(t, err)

	var items []WorktreeJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &items))
	assert.GreaterOrEqual(t, len(items), 1)

	var found bool
	for _, item := range items {
		if item.Branch == "feature/json-test" {
			found = true
			assert.NotEmpty(t, item.Commit, "JSON commit should be full hash")
			assert.True(t, len(item.Commit) >= 40, "commit should be full hash, got %q", item.Commit)
			assert.NotEmpty(t, item.Path)
			assert.NotEmpty(t, item.Name)
			break
		}
	}
	assert.True(t, found, "expected feature/json-test in JSON output")
}

func TestRunWorktreeDefault_JSON(t *testing.T) {
	repoDir := initCmdTestRepo(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feature/default-json", linkedWt, "main")

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	var out bytes.Buffer
	withWriters(t, &out, io.Discard)
	withOutputFormat(t, "json")

	client := worktree.NewClient(worktree.Options{})
	cmd := &cobra.Command{Use: "wt"}
	err = runWorktreeDefault(client, cmd)
	require.NoError(t, err)

	var items []WorktreeJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &items))
	assert.GreaterOrEqual(t, len(items), 1)
}

func TestRunWorktreeList_TextUnchanged(t *testing.T) {
	repoDir := initCmdTestRepo(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feature/text-check", linkedWt, "main")

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	var out bytes.Buffer
	withWriters(t, &out, io.Discard)
	withOutputFormat(t, "text")

	client := worktree.NewClient(worktree.Options{})
	err = runWorktreeList(client)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "feature/text-check")
	assert.NotContains(t, output, `"name"`)
}

func TestResolveWorktreeStatus(t *testing.T) {
	repoDir := initCmdTestRepo(t)

	oldCwd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldCwd) }()
	require.NoError(t, os.Chdir(repoDir))

	client := worktree.NewClient(worktree.Options{})
	root := getDisplayRoot(client)

	info := worktree.Info{Path: repoDir, Branch: "main", IsBare: true}
	assert.Equal(t, "bare", resolveWorktreeStatus(client, root, info))

	info = worktree.Info{Path: "/tmp/.claude/worktrees/foo", Branch: "feat"}
	assert.Equal(t, "agent", resolveWorktreeStatus(client, root, info))
}
