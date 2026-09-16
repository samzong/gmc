package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/internal/gitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"feature-login", false},
		{"fix/bug-123", false},
		{"my_branch", false},
		{"", true},
		{"-invalid", true},
		{"invalid..branch", true},
		{"with space", true},
		{"with~tilde", true},
		{"with^caret", true},
		{"with:colon", true},
		{"with?question", true},
		{"with*star", true},
		{"with[bracket", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := gitutil.ValidateBranchName(tt.name)
			assert.Equal(t, tt.wantErr, (err != nil))
		})
	}
}

func TestParseWorktreeList(t *testing.T) {
	input := `worktree /path/to/project/.bare
bare

worktree /path/to/project/main
HEAD abc123def456
branch refs/heads/main

worktree /path/to/project/feature-x
HEAD 789xyz
branch refs/heads/feature-x
`

	worktrees, err := parseWorktreeList(input)
	require.NoError(t, err)

	assert.Len(t, worktrees, 3)

	assert.True(t, worktrees[0].IsBare)

	assert.Equal(t, "main", worktrees[1].Branch)
	assert.Equal(t, "abc123def456", worktrees[1].Commit)

	assert.Equal(t, "feature-x", worktrees[2].Branch)
}

func TestParseWorktreeListDetached(t *testing.T) {
	input := `worktree /path/to/detached
HEAD abc123
detached
`

	worktrees, err := parseWorktreeList(input)
	require.NoError(t, err)

	require.Len(t, worktrees, 1)

	assert.Equal(t, "(detached)", worktrees[0].Branch)
}

func TestExtractProjectName(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/user/repo.git", "repo"},
		{"https://github.com/user/repo", "repo"},
		{"git@github.com:user/repo.git", "repo"},
		{"git@github.com:user/my-project.git", "my-project"},
		{"https://gitlab.com/org/subgroup/project.git", "project"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got, err := extractProjectName(tt.url)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCleanProjectName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"user/repo.git", "repo"},
		{"user/repo", "repo"},
		{"/org/project.git", "project"},
		{"my-project.git", "my-project"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := cleanProjectName(tt.path)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestFindBareRoot(t *testing.T) {
	for _, start := range []string{"missing", "root", ".bare"} {
		t.Run(start, func(t *testing.T) {
			root := t.TempDir()
			path := root
			if start != "missing" {
				require.NoError(t, os.Mkdir(filepath.Join(root, ".bare"), 0755))
			}
			if start == ".bare" {
				path = filepath.Join(root, ".bare")
			}
			got, err := FindBareRoot(path)
			if start == "missing" {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, root, got)
		})
	}
}

func TestGetWorktreeStatus(t *testing.T) {
	client := NewClient(Options{})

	status := client.GetWorktreeStatus("/nonexistent/path")
	assert.Equal(t, "unknown", status)
}

func TestProtectionPolicy(t *testing.T) {
	repoDir := initTestRepo(t)

	featureDir := filepath.Join(repoDir, "feature-wt")
	runGit(t, repoDir, "worktree", "add", "-b", "feature", featureDir, "main")

	t.Chdir(repoDir)

	client := NewClient(Options{})
	worktrees, err := client.List()
	require.NoError(t, err)

	policy, err := client.NewProtectionPolicy()
	require.NoError(t, err)
	for _, wt := range worktrees {
		protected := policy.IsProtected(wt)
		switch wt.Branch {
		case "main":
			assert.True(t, protected)
		case "feature":
			assert.False(t, protected)
		}
	}
}

func TestClientCacheInitialization(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	repoDir, _ = filepath.EvalSymlinks(repoDir)
	mainDir := filepath.Join(repoDir, "main")

	t.Chdir(mainDir)

	client := NewClient(Options{})

	root, err := client.GetWorktreeRoot()
	require.NoError(t, err)
	assert.Equal(t, repoDir, root)

	assert.Equal(t, repoDir, client.bareRoot)
	assert.Equal(t, repoDir, client.worktreeRoot)
	expectedRepoDir := filepath.Join(repoDir, ".bare")
	assert.Equal(t, expectedRepoDir, client.repoDir)
	assert.Equal(t, repoDir, client.searchRoot)

	root2, _ := client.GetWorktreeRoot()
	assert.Equal(t, root, root2)
}

func TestListCacheInvalidation(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	t.Chdir(mainDir)

	client := NewClient(Options{})

	list1, err := client.ListCached()
	require.NoError(t, err)
	initialCount := len(list1)

	list2, err := client.ListCached()
	require.NoError(t, err)
	assert.Equal(t, initialCount, len(list2))

	bareDir := filepath.Join(repoDir, ".bare")
	featureDir := filepath.Join(repoDir, "feature-test")
	runGit(t, bareDir, "worktree", "add", "-b", "feature-test", featureDir, "main")

	staleList, _ := client.ListCached()
	assert.Equal(t, initialCount, len(staleList))

	client.InvalidateList()

	freshList, err := client.ListCached()
	require.NoError(t, err)
	assert.Equal(t, initialCount+1, len(freshList))
}
