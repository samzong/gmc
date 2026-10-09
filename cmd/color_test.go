package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sgrPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestColorAllowedRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	assert.False(t, colorAllowed(true))
	t.Setenv("NO_COLOR", "")
	assert.True(t, colorAllowed(true))
	require.NoError(t, os.Unsetenv("NO_COLOR"))
	assert.True(t, colorAllowed(true))
	assert.False(t, colorAllowed(false))
	assert.False(t, colorEnabled(new(bytes.Buffer)))
}

func TestColorStatus(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"clean", "\x1b[32mclean\x1b[0m"},
		{"agent", "\x1b[36magent\x1b[0m"},
		{"bare", "\x1b[2mbare\x1b[0m"},
		{"clean, locked, prunable", "\x1b[32mclean\x1b[0m, \x1b[35mlocked\x1b[0m, \x1b[31mprunable\x1b[0m"},
		{"1 file changed, 3 files (+4 -2)", "1 file changed, 3 files (\x1b[32m+4\x1b[0m \x1b[31m-2\x1b[0m)"},
		{"2 untracked", "2 untracked"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, colorStatus(tt.status, true), tt.status)
		assert.Equal(t, tt.status, colorStatus(tt.status, false), tt.status)
	}
}

func TestWorktreeTableColor(t *testing.T) {
	repoDir, client, out := newWorktreeOutputTest(t)
	worktrees := []worktree.Info{
		{Path: repoDir, Branch: "feature/open", Commit: strings.Repeat("a", 40)},
		{Path: filepath.Join(repoDir, "gone"), Branch: "feature/merged", Commit: strings.Repeat("b", 40)},
		{Path: filepath.Join(repoDir, "closed"), Branch: "feature/closed", Commit: strings.Repeat("c", 40)},
		{Path: filepath.Join(repoDir, "none"), Branch: "feature/none", Commit: strings.Repeat("d", 40)},
	}
	reviews := map[string]worktree.ReviewInfo{
		"feature/open":   {Number: 1, State: "OPEN"},
		"feature/merged": {Number: 22, State: "MERGED"},
		"feature/closed": {Number: 333, State: "CLOSED"},
	}
	sizes := worktreeSizes{repoDir: {Path: repoDir, Bytes: 2048}}

	printWorktreeTable(client, worktrees, reviews, nil, sizes, false)
	plain := out.String()
	assert.NotContains(t, plain, "\x1b[")
	out.Reset()
	printWorktreeTable(client, worktrees, reviews, nil, sizes, true)
	colored := out.String()

	assert.Equal(t, plain, sgrPattern.ReplaceAllString(colored, ""))
	assert.True(t, strings.HasPrefix(colored, "\x1b[2mNAME"), colored)
	for _, fragment := range []string{
		"\x1b[1m" + filepath.Base(repoDir) + "\x1b[0m",
		"\x1b[32m#1 OPEN\x1b[0m",
		"\x1b[35m#22 MERGED\x1b[0m",
		"\x1b[31m#333 CLOSED\x1b[0m",
		"\x1b[2m-\x1b[0m",
	} {
		assert.Contains(t, colored, fragment)
	}
	assert.Equal(t, 1, strings.Count(colored, "\x1b[1m"))
}

func TestWorktreeListAllColor(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(dir)
	worktrees := []worktree.Info{
		{Repository: "/repo", Path: dir, Branch: "main", Commit: strings.Repeat("a", 40), Status: "clean"},
		{Repository: "/repo", Path: "/repo/功能", Branch: "功能", Commit: strings.Repeat("b", 40),
			Status: "1 untracked", IsLocked: true},
	}
	sizes := worktreeSizes{dir: {Path: dir, Bytes: 1 << 20}}

	var plain, colored bytes.Buffer
	printWorktreeListAll(&plain, worktrees, sizes, false)
	printWorktreeListAll(&colored, worktrees, sizes, true)

	assert.NotContains(t, plain.String(), "\x1b[")
	assert.Equal(t, plain.String(), sgrPattern.ReplaceAllString(colored.String(), ""))
	for _, fragment := range []string{
		"\x1b[2mPATH",
		"\x1b[1m" + abbrevPath(dir) + "\x1b[0m",
		"\x1b[32mclean\x1b[0m",
		"1 untracked, \x1b[35mlocked\x1b[0m",
		"\x1b[2m-\x1b[0m",
	} {
		assert.Contains(t, colored.String(), fragment)
	}
}

func TestColorReviewWrapsTerminalLink(t *testing.T) {
	reviews := map[string]worktree.ReviewInfo{
		"feature/open": {Number: 1, State: "OPEN", URL: "https://example.com/pull/1"},
	}
	display := formatWorktreeReviewDisplay(reviews, "feature/open", true)
	assert.Equal(t,
		"\x1b[32m\x1b]8;;https://example.com/pull/1\x1b\\#1\x1b]8;;\x1b\\ OPEN\x1b[0m",
		colorReview(reviews, "feature/open", display, true))
	assert.Equal(t, "\x1b[2m-\x1b[0m", colorReview(reviews, "feature/none", "-", true))
	assert.Equal(t, "-", colorReview(reviews, "feature/none", "-", false))
}
