package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputFormatFlag(t *testing.T) {
	f := &outputFormatFlag{value: "text"}
	assert.Equal(t, "string", f.Type())
	require.ErrorContains(t, f.Set("xml"), "must be text or json")
	assert.Equal(t, "text", f.String())
	for _, format := range []string{"json", "text"} {
		require.NoError(t, f.Set(format))
		assert.Equal(t, format, f.String())
	}
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

func setTestValue[T any](t *testing.T, target *T, value T) {
	t.Helper()
	previous := *target
	*target = value
	t.Cleanup(func() { *target = previous })
}

func withWriters(t *testing.T, out, errw io.Writer) {
	t.Helper()
	setTestValue(t, &outWriterFunc, func() io.Writer { return out })
	setTestValue(t, &errWriterFunc, func() io.Writer { return errw })
}

func newWorktreeOutputTest(t *testing.T) (string, *worktree.Client, *bytes.Buffer) {
	t.Helper()
	repo := initCmdTestRepo(t)
	t.Chdir(repo)
	out := new(bytes.Buffer)
	withWriters(t, out, io.Discard)
	return repo, worktree.NewClient(worktree.Options{}), out
}

func TestRunWorktreeJSON(t *testing.T) {
	for _, test := range []struct {
		branch      string
		showCurrent bool
	}{
		{"feature/json-test", false},
		{"feature/default-json", true},
	} {
		t.Run(test.branch, func(t *testing.T) {
			repoDir, client, out := newWorktreeOutputTest(t)
			linkedWt := filepath.Join(t.TempDir(), "feature-wt")
			runGitCmd(t, repoDir, "worktree", "add", "-b", test.branch, linkedWt, "main")
			setTestValue(t, &outputFlag.value, "json")
			require.NoError(t, runWorktreeList(client, test.showCurrent))
			var items []WorktreeJSON
			require.NoError(t, json.Unmarshal(out.Bytes(), &items))
			assert.NotEmpty(t, items)
			index := slices.IndexFunc(items, func(item WorktreeJSON) bool { return item.Branch == test.branch })
			require.NotEqual(t, -1, index, "expected %s in JSON output", test.branch)
			assert.NotEmpty(t, items[index].Commit, "JSON commit should be full hash")
			assert.GreaterOrEqual(t, len(items[index].Commit), 40)
			assert.NotEmpty(t, items[index].Path)
			assert.NotEmpty(t, items[index].Name)
		})
	}
}

func TestRunWorktreeList_TextUnchanged(t *testing.T) {
	repoDir, client, out := newWorktreeOutputTest(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feature/text-check", linkedWt, "main")

	setTestValue(t, &outputFlag.value, "text")

	err := runWorktreeList(client, false)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "feature/text-check")
	assert.NotContains(t, output, `"name"`)
}

func TestRunWorktreeList_AppendsDiffStatToStatus(t *testing.T) {
	repoDir, client, out := newWorktreeOutputTest(t)
	linkedWt := filepath.Join(t.TempDir(), "feature-wt")
	runGitCmd(t, repoDir, "worktree", "add", "-b", "feature/diff-check", linkedWt, "main")
	require.NoError(t, os.WriteFile(filepath.Join(linkedWt, "feature.txt"), []byte("one\ntwo\n"), 0o644))
	runGitCmd(t, linkedWt, "add", "feature.txt")
	runGitCmd(t, linkedWt, "commit", "-m", "add feature")

	setTestValue(t, &outputFlag.value, "text")

	err := runWorktreeList(client, false)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "feature/diff-check")
	assert.Contains(t, output, "1 file (+2 -0)")
	assert.NotContains(t, output, "clean, 1 file (+2 -0)")
}

func TestFormatWorktreeStatus(t *testing.T) {
	stat := worktree.DiffStat{Files: 1, Insertions: 2}

	assert.Equal(t, "clean", formatWorktreeStatus("clean", worktree.DiffStat{}, false))
	assert.Equal(t, "1 file (+2 -0)", formatWorktreeStatus("clean", stat, true))
	assert.Equal(t, "1 file changed, 1 file (+2 -0)", formatWorktreeStatus("1 file changed", stat, true))
	assert.Equal(t, "agent", formatWorktreeStatus("agent", worktree.DiffStat{}, true))
}

func TestBuildWorktreeJSON_WithReviewStates(t *testing.T) {
	repoDir, client, _ := newWorktreeOutputTest(t)

	wt := worktree.Info{
		Path:   repoDir,
		Branch: "feature/pr-json",
		Commit: strings.Repeat("a", 40),
	}
	items := buildWorktreeJSON(client, []worktree.Info{wt}, map[string]worktree.ReviewInfo{
		"feature/pr-json": {
			Provider:   "github",
			Number:     42,
			State:      "OPEN",
			HeadBranch: "feature/pr-json",
			URL:        "https://github.com/example/repo/pull/42",
		},
	}, map[string]worktree.DiffStat{
		repoDir: {
			Base:       "main",
			Files:      2,
			Insertions: 4,
		},
	})

	require.Len(t, items, 1)
	assert.Equal(t, "main", items[0].DiffBase)
	require.NotNil(t, items[0].ChangedFiles)
	require.NotNil(t, items[0].Insertions)
	require.NotNil(t, items[0].Deletions)
	assert.Equal(t, 2, *items[0].ChangedFiles)
	assert.Equal(t, 4, *items[0].Insertions)
	assert.Equal(t, 0, *items[0].Deletions)
	assert.Equal(t, resolveWorktreeStatus(client, getDisplayRoot(client), wt), items[0].Status)
	assert.NotEqual(t, "2 files (+4 -0)", items[0].Status)
	data, err := json.Marshal(items[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), `"deletions":0`)
	assert.Equal(t, "github", items[0].ReviewProvider)
	assert.Equal(t, 42, items[0].ReviewNumber)
	assert.Equal(t, "OPEN", items[0].ReviewState)
	assert.Equal(t, "https://github.com/example/repo/pull/42", items[0].ReviewURL)
}

func TestPrintWorktreeTable_WithReviewStates(t *testing.T) {
	repoDir, client, out := newWorktreeOutputTest(t)

	printWorktreeTable(client, []worktree.Info{{
		Path:   repoDir,
		Branch: "feature/pr-text",
		Commit: strings.Repeat("b", 40),
	}}, map[string]worktree.ReviewInfo{
		"feature/pr-text": {
			Provider:   "github",
			Number:     42,
			State:      "OPEN",
			HeadBranch: "feature/pr-text",
			URL:        "https://github.com/example/repo/pull/42",
		},
	}, nil)

	output := out.String()
	assert.Contains(t, output, "PR")
	assert.Contains(t, output, "#42 OPEN")
	assert.NotContains(t, output, "https://github.com/example/repo/pull/42")
}

func TestFormatWorktreeReviewDisplay_LinksNumber(t *testing.T) {
	text := formatWorktreeReviewDisplay(map[string]worktree.ReviewInfo{
		"feature/pr-link": {
			Provider:   "gitlab",
			Number:     42,
			State:      "OPEN",
			HeadBranch: "feature/pr-link",
			URL:        "https://gitlab.com/example/repo/-/merge_requests/42",
		},
	}, "feature/pr-link", true)

	assert.Contains(t, text, "\x1b]8;;https://gitlab.com/example/repo/-/merge_requests/42\x1b\\#42\x1b]8;;\x1b\\")
	assert.Contains(t, text, " OPEN")
}

func TestResolveWorktreeStatus(t *testing.T) {
	repoDir, client, _ := newWorktreeOutputTest(t)

	root := getDisplayRoot(client)

	info := worktree.Info{Path: repoDir, Branch: "main", IsBare: true}
	assert.Equal(t, "bare", resolveWorktreeStatus(client, root, info))

	info = worktree.Info{Path: "/tmp/.claude/worktrees/foo", Branch: "feat"}
	assert.Equal(t, "agent", resolveWorktreeStatus(client, root, info))
}
