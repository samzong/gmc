package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	client := NewClient(Options{})

	// Create a temp directory that's not a git repo
	tmpDir, err := os.MkdirTemp("", "gmc_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repoType, err := client.DetectRepositoryType(tmpDir)
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
	client := NewClient(Options{})

	// For a non-existent path, should return unknown
	status := client.GetWorktreeStatus("/nonexistent/path")
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

func TestResolveBaseBranch_OriginPreferred(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	runGit(t, repoDir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runGit(t, repoDir, "update-ref", "refs/remotes/upstream/main", "HEAD")
	runGit(t, repoDir, "symbolic-ref", "refs/remotes/upstream/HEAD", "refs/remotes/upstream/main")

	client := NewClient(Options{})
	base, err := client.resolveBaseBranch(repoDir, "")
	if err != nil {
		t.Fatalf("resolveBaseBranch() error = %v", err)
	}
	if base != "origin/main" {
		t.Errorf("resolveBaseBranch() = %q, want %q", base, "origin/main")
	}
}

func TestResolveBaseBranch_UpstreamFallback(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "update-ref", "refs/remotes/upstream/main", "HEAD")
	runGit(t, repoDir, "symbolic-ref", "refs/remotes/upstream/HEAD", "refs/remotes/upstream/main")

	client := NewClient(Options{})
	base, err := client.resolveBaseBranch(repoDir, "")
	if err != nil {
		t.Fatalf("resolveBaseBranch() error = %v", err)
	}
	if base != "upstream/main" {
		t.Errorf("resolveBaseBranch() = %q, want %q", base, "upstream/main")
	}
}

func TestResolveBaseBranch_LocalFallback(t *testing.T) {
	repoDir := initTestRepo(t)

	client := NewClient(Options{})
	base, err := client.resolveBaseBranch(repoDir, "")
	if err != nil {
		t.Fatalf("resolveBaseBranch() error = %v", err)
	}
	if base != "main" {
		t.Errorf("resolveBaseBranch() = %q, want %q", base, "main")
	}
}

func TestResolveSyncBaseBranch_OriginPreferred(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	runGit(t, repoDir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runGit(t, repoDir, "update-ref", "refs/remotes/upstream/main", "HEAD")
	runGit(t, repoDir, "symbolic-ref", "refs/remotes/upstream/HEAD", "refs/remotes/upstream/main")

	client := NewClient(Options{})
	base, err := client.resolveSyncBaseBranch(repoDir, "")
	if err != nil {
		t.Fatalf("resolveSyncBaseBranch() error = %v", err)
	}
	if base != "origin/main" {
		t.Errorf("resolveSyncBaseBranch() = %q, want %q", base, "origin/main")
	}
}

func TestResolveSyncBaseBranch_UpstreamFallback(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "update-ref", "refs/remotes/upstream/main", "HEAD")
	runGit(t, repoDir, "symbolic-ref", "refs/remotes/upstream/HEAD", "refs/remotes/upstream/main")

	client := NewClient(Options{})
	base, err := client.resolveSyncBaseBranch(repoDir, "")
	if err != nil {
		t.Fatalf("resolveSyncBaseBranch() error = %v", err)
	}
	if base != "upstream/main" {
		t.Errorf("resolveSyncBaseBranch() = %q, want %q", base, "upstream/main")
	}
}

func TestResolveSyncBaseBranch_MainFallback(t *testing.T) {
	repoDir := initTestRepo(t)

	client := NewClient(Options{})
	base, err := client.resolveSyncBaseBranch(repoDir, "")
	if err != nil {
		t.Fatalf("resolveSyncBaseBranch() error = %v", err)
	}
	if base != "main" {
		t.Errorf("resolveSyncBaseBranch() = %q, want %q", base, "main")
	}
}

func TestResolveSyncBaseBranch_MasterFallback(t *testing.T) {
	repoDir := initTestRepoWithBranch(t, "master")

	client := NewClient(Options{})
	base, err := client.resolveSyncBaseBranch(repoDir, "")
	if err != nil {
		t.Fatalf("resolveSyncBaseBranch() error = %v", err)
	}
	if base != "master" {
		t.Errorf("resolveSyncBaseBranch() = %q, want %q", base, "master")
	}
}

func TestSelectSyncRemote_PrefersUpstream(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "remote", "add", "origin", "https://example.com/origin/repo.git")
	runGit(t, repoDir, "remote", "add", "upstream", "https://example.com/upstream/repo.git")

	client := NewClient(Options{})
	remote, err := client.selectSyncRemote(repoDir)
	if err != nil {
		t.Fatalf("selectSyncRemote() error = %v", err)
	}
	if remote != "upstream" {
		t.Errorf("selectSyncRemote() = %q, want %q", remote, "upstream")
	}
}

func TestSelectSyncRemote_OriginFallback(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "remote", "add", "origin", "https://example.com/origin/repo.git")

	client := NewClient(Options{})
	remote, err := client.selectSyncRemote(repoDir)
	if err != nil {
		t.Fatalf("selectSyncRemote() error = %v", err)
	}
	if remote != "origin" {
		t.Errorf("selectSyncRemote() = %q, want %q", remote, "origin")
	}
}

func TestSelectSyncRemote_None(t *testing.T) {
	repoDir := initTestRepo(t)

	client := NewClient(Options{})
	_, err := client.selectSyncRemote(repoDir)
	if err == nil {
		t.Fatal("selectSyncRemote() expected error, got nil")
	}
}

func TestSync_UpstreamFastForwardAndPushOrigin(t *testing.T) {
	repoDir := initTestRepo(t)
	upstreamDir := initBareRepo(t)
	originDir := initBareRepo(t)

	runGit(t, repoDir, "remote", "add", "upstream", upstreamDir)
	runGit(t, repoDir, "remote", "add", "origin", originDir)
	runGit(t, repoDir, "push", "upstream", "main:refs/heads/main")
	runGit(t, repoDir, "push", "origin", "main:refs/heads/main")

	runGit(t, repoDir, "fetch", "origin")
	runGit(t, repoDir, "fetch", "upstream")
	runGit(t, repoDir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	runGit(t, repoDir, "symbolic-ref", "refs/remotes/upstream/HEAD", "refs/remotes/upstream/main")

	advanceRepoDir := t.TempDir()
	runGit(t, advanceRepoDir, "clone", upstreamDir, ".")
	runGit(t, advanceRepoDir, "checkout", "-B", "main", "origin/main")
	runGit(t, advanceRepoDir, "config", "user.name", "Test User")
	runGit(t, advanceRepoDir, "config", "user.email", "test@example.com")
	writeFile(t, filepath.Join(advanceRepoDir, "upstream.txt"), "upstream")
	runGit(t, advanceRepoDir, "add", ".")
	runGit(t, advanceRepoDir, "commit", "-m", "upstream")
	runGit(t, advanceRepoDir, "push", "origin", "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if chdirErr := os.Chdir(cwd); chdirErr != nil {
			t.Fatalf("failed to restore cwd: %v", chdirErr)
		}
	}()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	client := NewClient(Options{})
	if err := client.Sync(SyncOptions{}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	localHash := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/main"))
	upstreamHash := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/remotes/upstream/main"))
	originHash := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/main"))

	if localHash != upstreamHash {
		t.Errorf("local main hash = %s, want %s", localHash, upstreamHash)
	}
	if originHash != upstreamHash {
		t.Errorf("origin main hash = %s, want %s", originHash, upstreamHash)
	}
}

func TestIsBranchMerged(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(repoDir, "feature.txt"), "feature")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "feature")
	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "merge", "--no-ff", "-m", "merge feature", "feature")

	client := NewClient(Options{})
	merged, err := client.isBranchMerged(repoDir, "feature", "main")
	if err != nil {
		t.Fatalf("isBranchMerged() error = %v", err)
	}
	if !merged {
		t.Error("Expected feature to be merged into main")
	}

	runGit(t, repoDir, "checkout", "-b", "wip")
	writeFile(t, filepath.Join(repoDir, "wip.txt"), "wip")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "wip")
	runGit(t, repoDir, "checkout", "main")

	merged, err = client.isBranchMerged(repoDir, "wip", "main")
	if err != nil {
		t.Fatalf("isBranchMerged() error = %v", err)
	}
	if merged {
		t.Error("Expected wip to be unmerged into main")
	}
}

func TestPrune_RemovesMergedWorktreeAndBranch(t *testing.T) {
	repoDir := initTestRepo(t)

	worktreeDir := filepath.Join(repoDir, "feature-wt")
	runGit(t, repoDir, "worktree", "add", "-b", "feature", worktreeDir, "main")
	writeFile(t, filepath.Join(worktreeDir, "feature.txt"), "feature")
	runGit(t, worktreeDir, "add", ".")
	runGit(t, worktreeDir, "commit", "-m", "feature")
	runGit(t, repoDir, "merge", "--no-ff", "-m", "merge feature", "feature")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if chdirErr := os.Chdir(cwd); chdirErr != nil {
			t.Fatalf("failed to restore cwd: %v", chdirErr)
		}
	}()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	client := NewClient(Options{})
	if err := client.Prune(PruneOptions{}); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if _, err := os.Stat(worktreeDir); !os.IsNotExist(err) {
		t.Errorf("Expected worktree to be removed, stat error = %v", err)
	}

	if _, err := client.runner.Run("-C", repoDir, "rev-parse", "--verify", "refs/heads/feature"); err == nil {
		t.Error("Expected feature branch to be deleted")
	}
}

func TestLocalBranchName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"origin/main", "main"},
		{"upstream/dev", "dev"},
		{"feature/login", "feature/login"},
		{"refs/remotes/origin/main", "main"},
		{"refs/remotes/upstream/release", "release"},
		{"refs/heads/feature/login", "feature/login"},
		{"main", "main"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := localBranchName(tt.input); got != tt.expected {
				t.Errorf("localBranchName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func initTestRepo(t *testing.T) string {
	return initTestRepoWithBranch(t, "main")
}

func initTestRepoWithBranch(t *testing.T, branch string) string {
	t.Helper()
	repoDir := t.TempDir()

	runGit(t, repoDir, "init", "-b", branch)
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	writeFile(t, filepath.Join(repoDir, "README.md"), "init")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "init")

	return repoDir
}

func initBareRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--bare")
	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
	return string(output)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile(%s) failed: %v", path, err)
	}
}
