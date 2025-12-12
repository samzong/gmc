package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoTypeString(t *testing.T) {
	tests := []struct {
		repoType RepoType
		expected string
	}{
		{RepoTypeNormal, "normal"},
		{RepoTypeBare, "bare"},
		{RepoTypeWorktree, "worktree"},
		{RepoTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.repoType.String(); got != tt.expected {
				t.Errorf("RepoType.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

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
			err := validateBranchName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBranchName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
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
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 3 {
		t.Errorf("Expected 3 worktrees, got %d", len(worktrees))
	}

	// Check bare worktree
	if !worktrees[0].IsBare {
		t.Error("First worktree should be bare")
	}

	// Check main worktree
	if worktrees[1].Branch != "main" {
		t.Errorf("Second worktree branch = %q, want 'main'", worktrees[1].Branch)
	}
	if worktrees[1].Commit != "abc123def456" {
		t.Errorf("Second worktree commit = %q, want 'abc123def456'", worktrees[1].Commit)
	}

	// Check feature worktree
	if worktrees[2].Branch != "feature-x" {
		t.Errorf("Third worktree branch = %q, want 'feature-x'", worktrees[2].Branch)
	}
}

func TestParseWorktreeListDetached(t *testing.T) {
	input := `worktree /path/to/detached
HEAD abc123
detached
`

	worktrees, err := parseWorktreeList(input)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("Expected 1 worktree, got %d", len(worktrees))
	}

	if worktrees[0].Branch != "(detached)" {
		t.Errorf("Branch = %q, want '(detached)'", worktrees[0].Branch)
	}
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
			if err != nil {
				t.Fatalf("extractProjectName() error = %v", err)
			}
			if got != tt.expected {
				t.Errorf("extractProjectName(%q) = %q, want %q", tt.url, got, tt.expected)
			}
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
			if got != tt.expected {
				t.Errorf("cleanProjectName(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestDetectRepositoryType_NotRepo(t *testing.T) {
	// Create a temp directory that's not a git repo
	tmpDir, err := os.MkdirTemp("", "gmc_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repoType, err := DetectRepositoryType(tmpDir)
	if err != nil {
		t.Fatalf("DetectRepositoryType() error = %v", err)
	}
	if repoType != RepoTypeUnknown {
		t.Errorf("DetectRepositoryType() = %v, want %v", repoType, RepoTypeUnknown)
	}
}

func TestFindBareRoot_NotFound(t *testing.T) {
	// Create a temp directory without .bare
	tmpDir, err := os.MkdirTemp("", "gmc_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = FindBareRoot(tmpDir)
	if err == nil {
		t.Error("Expected error when .bare not found")
	}
}

func TestFindBareRoot_Found(t *testing.T) {
	// Create a temp directory with .bare
	tmpDir, err := os.MkdirTemp("", "gmc_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	bareDir := filepath.Join(tmpDir, ".bare")
	if err := os.Mkdir(bareDir, 0755); err != nil {
		t.Fatal(err)
	}

	root, err := FindBareRoot(tmpDir)
	if err != nil {
		t.Fatalf("FindBareRoot() error = %v", err)
	}
	if root != tmpDir {
		t.Errorf("FindBareRoot() = %q, want %q", root, tmpDir)
	}
}

func TestGetWorktreeStatus(t *testing.T) {
	// For a non-existent path, should return unknown
	status := GetWorktreeStatus("/nonexistent/path")
	if status != "unknown" {
		t.Errorf("GetWorktreeStatus() = %q, want 'unknown'", status)
	}
}

func TestAddOptions(t *testing.T) {
	opts := AddOptions{
		BaseBranch: "main",
		Fetch:      true,
	}

	if opts.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want 'main'", opts.BaseBranch)
	}
	if !opts.Fetch {
		t.Error("Fetch should be true")
	}
}

func TestRemoveOptions(t *testing.T) {
	opts := RemoveOptions{
		Force:        true,
		DeleteBranch: true,
	}

	if !opts.Force {
		t.Error("Force should be true")
	}
	if !opts.DeleteBranch {
		t.Error("DeleteBranch should be true")
	}
}

func TestCloneOptions(t *testing.T) {
	opts := CloneOptions{
		Name:     "my-project",
		Upstream: "https://github.com/upstream/repo.git",
	}

	if opts.Name != "my-project" {
		t.Errorf("Name = %q, want 'my-project'", opts.Name)
	}
	if opts.Upstream != "https://github.com/upstream/repo.git" {
		t.Errorf("Upstream = %q", opts.Upstream)
	}
}
