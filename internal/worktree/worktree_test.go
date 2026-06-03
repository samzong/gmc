package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/gitutil"
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
			err := gitutil.ValidateBranchName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBranchName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
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

func TestFindBareRoot_FromBareDir(t *testing.T) {
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

	root, err := FindBareRoot(bareDir)
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

func TestWorktreeDiffStat(t *testing.T) {
	repoDir := initTestRepo(t)
	runGit(t, repoDir, "checkout", "-b", "feature/diff-stat")
	writeFile(t, filepath.Join(repoDir, "feature.txt"), "one\ntwo\n")
	runGit(t, repoDir, "add", "feature.txt")
	runGit(t, repoDir, "commit", "-m", "add feature")

	writeFile(t, filepath.Join(repoDir, "README.md"), "initial\nupdated\n")

	client := NewClient(Options{})
	stat, err := client.WorktreeDiffStat(repoDir, "main")
	if err != nil {
		t.Fatalf("WorktreeDiffStat() error = %v", err)
	}

	if stat.Files != 2 {
		t.Errorf("Files = %d, want 2", stat.Files)
	}
	if stat.Insertions != 4 {
		t.Errorf("Insertions = %d, want 4", stat.Insertions)
	}
	if stat.Deletions != 1 {
		t.Errorf("Deletions = %d, want 1", stat.Deletions)
	}
}

func TestParseDiffNumstatHandlesRename(t *testing.T) {
	output := []byte("1\t0\t\x00old.txt\x00new.txt\x00")
	stat := parseDiffNumstat(output)

	if stat.Files != 1 {
		t.Errorf("Files = %d, want 1", stat.Files)
	}
	if stat.Insertions != 1 {
		t.Errorf("Insertions = %d, want 1", stat.Insertions)
	}
	if stat.Deletions != 0 {
		t.Errorf("Deletions = %d, want 0", stat.Deletions)
	}
}

func TestResolveDiffBaseForWorktree_UsesBranchUpstream(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "checkout", "-b", "feature/upstream-tracked")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")
	runGit(t, repoDir, "update-ref", "refs/heads/tracked-base", "HEAD")
	runGit(t, repoDir, "branch", "--set-upstream-to=tracked-base")

	client := NewClient(Options{})
	base, err := client.ResolveDiffBaseForWorktree(repoDir, "")
	if err != nil {
		t.Fatalf("ResolveDiffBaseForWorktree() error = %v", err)
	}
	if base != "tracked-base" {
		t.Errorf("ResolveDiffBaseForWorktree() = %q, want %q", base, "tracked-base")
	}
}

func TestResolveDiffBaseForWorktree_FallsBackToRemoteRef(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "checkout", "-b", "feature/no-upstream")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")

	client := NewClient(Options{})
	base, err := client.ResolveDiffBaseForWorktree(repoDir, "")
	if err != nil {
		t.Fatalf("ResolveDiffBaseForWorktree() error = %v", err)
	}
	if base != "origin/main" {
		t.Errorf("ResolveDiffBaseForWorktree() = %q, want %q", base, "origin/main")
	}
}

// Regression: in CI the client singleton's repoDir (the gmc checkout) is
// often on a detached PR ref with no `refs/heads/main`. The per-worktree
// resolver must derive the base from the worktree path itself, not the
// caller's repoDir.
func TestResolveDiffBaseForWorktree_DoesNotDependOnClientRepoDir(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "checkout", "-b", "feature/no-upstream")
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/main", "HEAD")

	client := NewClient(Options{})
	client.repoDir = t.TempDir()

	base, err := client.ResolveDiffBaseForWorktree(repoDir, "")
	if err != nil {
		t.Fatalf("ResolveDiffBaseForWorktree() error = %v", err)
	}
	if base != "origin/main" {
		t.Errorf("ResolveDiffBaseForWorktree() = %q, want %q", base, "origin/main")
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
	if _, err := client.Sync(SyncOptions{}); err != nil {
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
	if _, err := client.Prune(PruneOptions{}); err != nil {
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

func TestIsProtectedWorktree(t *testing.T) {
	repoDir := initTestRepo(t)

	featureDir := filepath.Join(repoDir, "feature-wt")
	runGit(t, repoDir, "worktree", "add", "-b", "feature", featureDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	worktrees, err := client.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	for _, wt := range worktrees {
		protected, err := client.IsProtectedWorktree(wt)
		if err != nil {
			t.Fatalf("IsProtectedWorktree() error = %v", err)
		}
		switch wt.Branch {
		case "main":
			if !protected {
				t.Errorf("main worktree should be protected, path=%s", wt.Path)
			}
		case "feature":
			if protected {
				t.Errorf("feature worktree should NOT be protected, path=%s", wt.Path)
			}
		}
	}
}

func TestRemoveProtectedWorktree(t *testing.T) {
	repoDir := initTestRepo(t)

	featureDir := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+"--feature")
	runGit(t, repoDir, "worktree", "add", "-b", "feature", featureDir, "main")
	defer os.RemoveAll(featureDir)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(featureDir); err != nil {
		t.Fatal(err)
	}

	repoName := filepath.Base(repoDir)
	client := NewClient(Options{})
	_, err = client.Remove(repoName, RemoveOptions{})
	if err == nil {
		t.Fatal("expected error when removing protected worktree")
	}
	if !strings.Contains(err.Error(), "cannot remove protected worktree") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPromoteRejectsSameWorktreeCandidate(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	client := NewClient(Options{})
	_, err := client.Promote(repoDir, PromoteOptions{})
	if err == nil {
		t.Fatal("expected error when promoting the current worktree")
	}
	if !strings.Contains(err.Error(), "must be different from the parent") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDupCopiesTaskFilesAndKeepsBareLayoutPath(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	chdir(t, mainDir)

	writeFile(t, filepath.Join(mainDir, "todo.md"), "task")

	client := NewClient(Options{})
	result, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1, TaskFiles: []string{"todo.md"}})
	if err != nil {
		t.Fatalf("Dup() error = %v", err)
	}
	if got, want := result.Worktrees[0], ".dup-1"; got != want {
		t.Fatalf("Worktrees[0] = %q, want %q", got, want)
	}
	dupDir := filepath.Join(repoDir, ".dup-1")
	if got, want := result.WorktreePaths[0], dupDir; !sameCleanPath(got, want) {
		t.Fatalf("WorktreePaths[0] = %q, want %q", got, want)
	}
	if got, want := readFile(t, filepath.Join(dupDir, "todo.md")), "task"; got != want {
		t.Fatalf("copied task = %q, want %q", got, want)
	}
}

func TestDupWithoutTaskWorksFromBareLayoutRoot(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	chdir(t, repoDir)

	client := NewClient(Options{})
	result, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1})
	if err != nil {
		t.Fatalf("Dup() error = %v", err)
	}
	dupDir := filepath.Join(repoDir, ".dup-1")
	if got, want := result.WorktreePaths[0], dupDir; !sameCleanPath(got, want) {
		t.Fatalf("WorktreePaths[0] = %q, want %q", got, want)
	}
	if got, want := result.RelativePaths[0], ".dup-1"; got != want {
		t.Fatalf("RelativePaths[0] = %q, want %q", got, want)
	}
}

func TestDupDefaultsToCurrentWorktreeBranchAndSiblingPath(t *testing.T) {
	repoDir := initTestRepo(t)
	featureDir := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+"--feature-current")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/current", featureDir, "main")
	var err error
	featureDir, err = filepath.EvalSymlinks(featureDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(featureDir) })

	writeFile(t, filepath.Join(featureDir, "feature.txt"), "feature")
	runGit(t, featureDir, "add", "feature.txt")
	runGit(t, featureDir, "commit", "-m", "feature")
	chdir(t, featureDir)

	client := NewClient(Options{})
	result, err := client.Dup(DupOptions{Count: 1})
	if err != nil {
		t.Fatalf("Dup() error = %v", err)
	}

	dupDir := filepath.Join(filepath.Dir(featureDir), ".dup-1")
	t.Cleanup(func() { _ = os.RemoveAll(dupDir) })
	if got, want := result.BaseBranch, "feature/current"; got != want {
		t.Fatalf("BaseBranch = %q, want %q", got, want)
	}
	if got, want := result.WorktreePaths[0], dupDir; !sameCleanPath(got, want) {
		t.Fatalf("WorktreePaths[0] = %q, want %q", got, want)
	}
	if got, want := result.RelativePaths[0], "../.dup-1"; got != want {
		t.Fatalf("RelativePaths[0] = %q, want %q", got, want)
	}
	if got, want := readFile(t, filepath.Join(dupDir, "feature.txt")), "feature"; got != want {
		t.Fatalf("feature.txt = %q, want %q", got, want)
	}
}

func TestDupRejectsTaskDirectory(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	chdir(t, mainDir)
	if err := os.Mkdir(filepath.Join(mainDir, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	_, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1, TaskFiles: []string{"tasks"}})
	if err == nil {
		t.Fatal("expected directory task error")
	}
	if !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDupAcceptsCanonicalTaskPath(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	writeFile(t, filepath.Join(mainDir, "todo.md"), "task")

	linkDir := filepath.Join(t.TempDir(), "main-link")
	if err := os.Symlink(mainDir, linkDir); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	chdir(t, mainDir)

	client := NewClient(Options{})
	if _, err := client.Dup(DupOptions{
		BaseBranch: "main",
		Count:      1,
		TaskFiles:  []string{filepath.Join(linkDir, "todo.md")},
	}); err != nil {
		t.Fatalf("Dup() error = %v", err)
	}
	assertFileContent(t, filepath.Join(repoDir, ".dup-1", "todo.md"), "task")
}

func TestPromoteAppliesCandidateChangesToCurrentParent(t *testing.T) {
	repoDir, mainDir, dupDir := createPromoteCandidate(t)

	writeFile(t, filepath.Join(dupDir, "committed.txt"), "committed")
	runGit(t, dupDir, "add", "committed.txt")
	runGit(t, dupDir, "commit", "-m", "candidate commit")
	writeFile(t, filepath.Join(dupDir, "staged.txt"), "staged")
	runGit(t, dupDir, "add", "staged.txt")
	writeFile(t, filepath.Join(dupDir, "README.md"), "candidate readme")
	writeFile(t, filepath.Join(dupDir, "untracked.txt"), "untracked")
	chdir(t, mainDir)

	client := NewClient(Options{})
	report, err := client.Promote(".dup-1", PromoteOptions{})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	if len(report.Events) == 0 {
		t.Fatal("expected report events")
	}

	assertFileContent(t, filepath.Join(mainDir, "committed.txt"), "committed")
	assertFileContent(t, filepath.Join(mainDir, "staged.txt"), "staged")
	assertFileContent(t, filepath.Join(mainDir, "README.md"), "candidate readme")
	assertFileContent(t, filepath.Join(mainDir, "untracked.txt"), "untracked")
	status := runGit(t, mainDir, "status", "--short")
	for _, file := range []string{"committed.txt", "staged.txt", "README.md", "untracked.txt"} {
		if !strings.Contains(status, file) {
			t.Fatalf("status %q does not contain %s in repo %s", status, file, repoDir)
		}
	}
}

func TestPromoteDryRunLeavesParentUnchanged(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	runGit(t, dupDir, "add", "candidate.txt")
	runGit(t, dupDir, "commit", "-m", "candidate")
	chdir(t, mainDir)

	client := NewClient(Options{})
	if _, err := client.Promote(".dup-1", PromoteOptions{DryRun: true}); err != nil {
		t.Fatalf("Promote(DryRun) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(mainDir, "candidate.txt")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created candidate.txt, stat err = %v", err)
	}
}

func TestPromoteRejectsDirtyParent(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	runGit(t, dupDir, "add", "candidate.txt")
	runGit(t, dupDir, "commit", "-m", "candidate")
	writeFile(t, filepath.Join(mainDir, "local.txt"), "local")
	chdir(t, mainDir)

	client := NewClient(Options{})
	if _, err := client.Promote(".dup-1", PromoteOptions{}); err == nil {
		t.Fatal("expected dirty parent error")
	} else if !strings.Contains(err.Error(), "clean it before promoting") {
		t.Fatalf("Promote() error = %v, want clean parent guidance", err)
	}
	if _, err := os.Stat(filepath.Join(mainDir, "candidate.txt")); !os.IsNotExist(err) {
		t.Fatalf("dirty parent promote created candidate.txt, stat err = %v", err)
	}
	assertFileContent(t, filepath.Join(mainDir, "local.txt"), "local")
}

func TestPromoteSkipsUnchangedCopiedTaskFile(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	chdir(t, mainDir)

	writeFile(t, filepath.Join(mainDir, "task.txt"), "task")

	client := NewClient(Options{})
	if _, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1, TaskFiles: []string{"task.txt"}}); err != nil {
		t.Fatalf("Dup() error = %v", err)
	}

	dupDir := filepath.Join(repoDir, ".dup-1")
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	runGit(t, dupDir, "add", "candidate.txt")
	runGit(t, dupDir, "commit", "-m", "candidate")

	report, err := client.Promote(".dup-1", PromoteOptions{})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}

	assertFileContent(t, filepath.Join(mainDir, "task.txt"), "task")
	assertFileContent(t, filepath.Join(mainDir, "candidate.txt"), "candidate")
	for _, event := range report.Events {
		if strings.Contains(event.Message, "task.txt") {
			t.Fatalf("report unexpectedly included unchanged task file: %q", event.Message)
		}
	}
}

func TestPromoteWorksInNormalRepositoryWithNestedCandidate(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	client := NewClient(Options{})
	if _, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1}); err != nil {
		t.Fatalf("Dup() error = %v", err)
	}

	dupDir := filepath.Join(filepath.Dir(repoDir), ".dup-1")
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	runGit(t, dupDir, "add", "candidate.txt")
	runGit(t, dupDir, "commit", "-m", "candidate")

	if _, err := client.Promote(".dup-1", PromoteOptions{}); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	assertFileContent(t, filepath.Join(repoDir, "candidate.txt"), "candidate")
}

func TestPromoteAppliesOntoAdvancedParent(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	runGit(t, dupDir, "add", "candidate.txt")
	runGit(t, dupDir, "commit", "-m", "candidate")
	writeFile(t, filepath.Join(mainDir, "parent.txt"), "parent")
	runGit(t, mainDir, "add", "parent.txt")
	runGit(t, mainDir, "commit", "-m", "parent")
	chdir(t, mainDir)

	client := NewClient(Options{})
	if _, err := client.Promote(".dup-1", PromoteOptions{}); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	assertFileContent(t, filepath.Join(mainDir, "candidate.txt"), "candidate")
	assertFileContent(t, filepath.Join(mainDir, "parent.txt"), "parent")
}

func TestPromoteTreatsAlreadyAppliedCandidateChangeAsNoop(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "same.txt"), "same")
	writeFile(t, filepath.Join(dupDir, "new.txt"), "new")
	runGit(t, dupDir, "add", "same.txt", "new.txt")
	runGit(t, dupDir, "commit", "-m", "candidate")

	writeFile(t, filepath.Join(mainDir, "same.txt"), "same")
	runGit(t, mainDir, "add", "same.txt")
	runGit(t, mainDir, "commit", "-m", "parent already has same")
	chdir(t, mainDir)

	client := NewClient(Options{})
	if _, err := client.Promote(".dup-1", PromoteOptions{}); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	assertFileContent(t, filepath.Join(mainDir, "same.txt"), "same")
	assertFileContent(t, filepath.Join(mainDir, "new.txt"), "new")
}

func TestPromotePreservesUntrackedSymlink(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "target.txt"), "target")
	if err := os.Symlink("target.txt", filepath.Join(dupDir, "link.txt")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	chdir(t, mainDir)

	client := NewClient(Options{})
	if _, err := client.Promote(".dup-1", PromoteOptions{}); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}

	target, err := os.Readlink(filepath.Join(mainDir, "link.txt"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if target != "target.txt" {
		t.Fatalf("link target = %q, want target.txt", target)
	}
	assertFileContent(t, filepath.Join(mainDir, "target.txt"), "target")
}

func TestPromoteConflictLeavesParentUnchanged(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "README.md"), "candidate")
	runGit(t, dupDir, "add", "README.md")
	runGit(t, dupDir, "commit", "-m", "candidate readme")
	writeFile(t, filepath.Join(mainDir, "README.md"), "parent")
	runGit(t, mainDir, "add", "README.md")
	runGit(t, mainDir, "commit", "-m", "parent readme")
	chdir(t, mainDir)

	client := NewClient(Options{})
	if _, err := client.Promote(".dup-1", PromoteOptions{}); err == nil {
		t.Fatal("expected conflict error")
	}
	assertFileContent(t, filepath.Join(mainDir, "README.md"), "parent")
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

func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readFile(%s) failed: %v", path, err)
	}
	return string(data)
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	if got := readFile(t, path); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func createPromoteCandidate(t *testing.T) (string, string, string) {
	t.Helper()
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	chdir(t, mainDir)

	client := NewClient(Options{})
	if _, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1}); err != nil {
		t.Fatalf("Dup() error = %v", err)
	}
	return repoDir, mainDir, filepath.Join(repoDir, ".dup-1")
}

func initBareLayoutRepo(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	bareDir := filepath.Join(tmpDir, ".bare")
	runGit(t, tmpDir, "init", "--bare", bareDir)
	runGit(t, bareDir, "config", "user.name", "Test User")
	runGit(t, bareDir, "config", "user.email", "test@example.com")

	mainDir := filepath.Join(tmpDir, "main")
	runGit(t, bareDir, "worktree", "add", mainDir, "-b", "main")
	writeFile(t, filepath.Join(mainDir, "README.md"), "init")
	runGit(t, mainDir, "add", ".")
	runGit(t, mainDir, "commit", "-m", "init")

	return tmpDir
}

func initBareLayoutRepoWithWorktreeConfig(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	bareDir := filepath.Join(tmpDir, ".bare")
	runGit(t, tmpDir, "init", "--bare", bareDir)
	runGit(t, bareDir, "config", "extensions.worktreeConfig", "true")
	runGit(t, bareDir, "config", "user.name", "Test User")
	runGit(t, bareDir, "config", "user.email", "test@example.com")

	mainDir := filepath.Join(tmpDir, "main")
	runGit(t, bareDir, "worktree", "add", mainDir, "-b", "main")
	runGit(t, mainDir, "config", "--worktree", "core.bare", "false")
	writeFile(t, filepath.Join(mainDir, "README.md"), "init")
	runGit(t, mainDir, "add", ".")
	runGit(t, mainDir, "commit", "-m", "init")

	return tmpDir
}

func TestClientCacheInitialization(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	repoDir, _ = filepath.EvalSymlinks(repoDir)
	mainDir := filepath.Join(repoDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})

	root, err := client.GetWorktreeRoot()
	if err != nil {
		t.Fatalf("GetWorktreeRoot() error = %v", err)
	}
	if root != repoDir {
		t.Errorf("GetWorktreeRoot() = %q, want %q", root, repoDir)
	}

	if !client.IsBareWorktree() {
		t.Error("IsBareWorktree() should return true for bare layout")
	}

	if client.bareRoot != repoDir {
		t.Errorf("bareRoot = %q, want %q", client.bareRoot, repoDir)
	}
	if client.worktreeRoot != repoDir {
		t.Errorf("worktreeRoot = %q, want %q", client.worktreeRoot, repoDir)
	}
	expectedRepoDir := filepath.Join(repoDir, ".bare")
	if client.repoDir != expectedRepoDir {
		t.Errorf("repoDir = %q, want %q", client.repoDir, expectedRepoDir)
	}
	if client.searchRoot != repoDir {
		t.Errorf("searchRoot = %q, want %q", client.searchRoot, repoDir)
	}

	root2, _ := client.GetWorktreeRoot()
	if root2 != root {
		t.Errorf("second GetWorktreeRoot() = %q, want %q (should be cached)", root2, root)
	}
}

func TestAddConfiguresBareLayoutWorktreeConfig(t *testing.T) {
	repoDir := initBareLayoutRepoWithWorktreeConfig(t)
	mainDir := filepath.Join(repoDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	if _, err := client.Add("feature-config", AddOptions{BaseBranch: "main"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	featureDir := filepath.Join(repoDir, "feature-config")
	status := runGit(t, featureDir, "status", "--short", "--branch")
	if !strings.Contains(status, "## feature-config") {
		t.Fatalf("new worktree status = %q, want feature-config branch", status)
	}

	got := strings.TrimSpace(runGit(t, featureDir, "config", "--worktree", "--bool", "core.bare"))
	if got != "false" {
		t.Fatalf("worktree core.bare = %q, want false", got)
	}
}

func TestAddDoesNotRequireWorktreeConfig(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	if _, err := client.Add("feature-no-config", AddOptions{BaseBranch: "main"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	featureDir := filepath.Join(repoDir, "feature-no-config")
	status := runGit(t, featureDir, "status", "--short", "--branch")
	if !strings.Contains(status, "## feature-no-config") {
		t.Fatalf("new worktree status = %q, want feature-no-config branch", status)
	}
}

func TestAddIgnoresGlobalWorktreeConfig(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	globalConfig := filepath.Join(t.TempDir(), "global.gitconfig")
	writeFile(t, globalConfig, "[extensions]\n\tworktreeConfig = true\n")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	if _, err := client.Add("feature-global-config", AddOptions{BaseBranch: "main"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	featureDir := filepath.Join(repoDir, "feature-global-config")
	status := runGit(t, featureDir, "status", "--short", "--branch")
	if !strings.Contains(status, "## feature-global-config") {
		t.Fatalf("new worktree status = %q, want feature-global-config branch", status)
	}
}

func TestDupConfiguresBareLayoutWorktreeConfig(t *testing.T) {
	repoDir := initBareLayoutRepoWithWorktreeConfig(t)
	mainDir := filepath.Join(repoDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	result, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1})
	if err != nil {
		t.Fatalf("Dup() error = %v", err)
	}
	if len(result.Worktrees) != 1 {
		t.Fatalf("Dup() worktrees = %d, want 1", len(result.Worktrees))
	}

	dupDir := filepath.Join(repoDir, result.Worktrees[0])
	status := runGit(t, dupDir, "status", "--short", "--branch")
	if !strings.Contains(status, "## "+result.Branches[0]) {
		t.Fatalf("new worktree status = %q, want %s branch", status, result.Branches[0])
	}

	got := strings.TrimSpace(runGit(t, dupDir, "config", "--worktree", "--bool", "core.bare"))
	if got != "false" {
		t.Fatalf("worktree core.bare = %q, want false", got)
	}
}

func TestListCacheInvalidation(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})

	list1, err := client.ListCached()
	if err != nil {
		t.Fatalf("ListCached() error = %v", err)
	}
	initialCount := len(list1)

	list2, err := client.ListCached()
	if err != nil {
		t.Fatalf("ListCached() second call error = %v", err)
	}
	if len(list2) != initialCount {
		t.Errorf("ListCached() returned different count: %d vs %d", len(list2), initialCount)
	}

	bareDir := filepath.Join(repoDir, ".bare")
	featureDir := filepath.Join(repoDir, "feature-test")
	runGit(t, bareDir, "worktree", "add", "-b", "feature-test", featureDir, "main")

	staleList, _ := client.ListCached()
	if len(staleList) != initialCount {
		t.Error("ListCached() should return stale data before invalidation")
	}

	client.InvalidateList()

	freshList, err := client.ListCached()
	if err != nil {
		t.Fatalf("ListCached() after invalidation error = %v", err)
	}
	if len(freshList) != initialCount+1 {
		t.Errorf("ListCached() after invalidation = %d worktrees, want %d", len(freshList), initialCount+1)
	}
}

func TestRemoveBatch(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	bareDir := filepath.Join(repoDir, ".bare")

	names := []string{"wt-a", "wt-b", "wt-c"}
	for _, name := range names {
		dir := filepath.Join(repoDir, name)
		runGit(t, bareDir, "worktree", "add", "-b", name, dir, "main")
	}
	client.InvalidateList()

	result := client.RemoveBatch(names, RemoveOptions{Force: true, DeleteBranch: true})

	if len(result.Failed) > 0 {
		t.Fatalf("RemoveBatch() had failures: %v", result.Failed)
	}
	if len(result.Succeeded) != 3 {
		t.Errorf("RemoveBatch() succeeded = %d, want 3", len(result.Succeeded))
	}

	for _, name := range names {
		dir := filepath.Join(repoDir, name)
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("worktree %s still exists", name)
		}
	}

	for _, name := range names {
		if _, err := client.runner.Run("-C", bareDir, "rev-parse", "--verify", "refs/heads/"+name); err == nil {
			t.Errorf("branch %s still exists", name)
		}
	}
}

func TestRemoveBatchPartialFailure(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	bareDir := filepath.Join(repoDir, ".bare")

	dir := filepath.Join(repoDir, "wt-good")
	runGit(t, bareDir, "worktree", "add", "-b", "wt-good", dir, "main")
	client.InvalidateList()

	result := client.RemoveBatch([]string{"wt-good", "wt-nonexistent"}, RemoveOptions{Force: true})

	if len(result.Failed) == 0 {
		t.Fatal("RemoveBatch() should have failures for nonexistent worktree")
	}
	if _, ok := result.Failed["wt-nonexistent"]; !ok {
		t.Error("expected wt-nonexistent in Failed map")
	}

	if len(result.Succeeded) != 0 {
		t.Error("RemoveBatch() should not have succeeded entries when validation fails")
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("wt-good should NOT be removed when batch validation fails (fail-fast)")
	}
}

func TestRemoveBatchDryRun(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(mainDir); err != nil {
		t.Fatal(err)
	}

	client := NewClient(Options{})
	bareDir := filepath.Join(repoDir, ".bare")

	dir := filepath.Join(repoDir, "wt-dry")
	runGit(t, bareDir, "worktree", "add", "-b", "wt-dry", dir, "main")
	client.InvalidateList()

	result := client.RemoveBatch([]string{"wt-dry"}, RemoveOptions{DryRun: true, DeleteBranch: true})

	if len(result.Failed) > 0 {
		t.Fatalf("RemoveBatch(DryRun) failures: %v", result.Failed)
	}
	if len(result.Succeeded) != 1 {
		t.Errorf("RemoveBatch(DryRun) succeeded = %d, want 1", len(result.Succeeded))
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("DryRun should not actually remove the worktree")
	}

	if len(result.Report.Events) == 0 {
		t.Error("DryRun should produce report events")
	}
}

func BenchmarkRemoveBatch(b *testing.B) {
	for range b.N {
		b.StopTimer()

		tmpDir := b.TempDir()
		bareDir := filepath.Join(tmpDir, ".bare")

		cmd := exec.Command("git", "init", "--bare", bareDir)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("git init --bare: %v\n%s", err, out)
		}

		mainDir := filepath.Join(tmpDir, "main")
		cmd = exec.Command("git", "-C", bareDir, "worktree", "add", mainDir, "-b", "main")
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("git worktree add main: %v\n%s", err, out)
		}

		cmd = exec.Command("git", "-C", mainDir, "config", "user.name", "Bench")
		if _, err := cmd.CombinedOutput(); err != nil {
			b.Fatal(err)
		}
		cmd = exec.Command("git", "-C", mainDir, "config", "user.email", "b@b.com")
		if _, err := cmd.CombinedOutput(); err != nil {
			b.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(mainDir, "f"), []byte("x"), 0644); err != nil {
			b.Fatal(err)
		}
		cmd = exec.Command("git", "-C", mainDir, "add", ".")
		if _, err := cmd.CombinedOutput(); err != nil {
			b.Fatal(err)
		}
		cmd = exec.Command("git", "-C", mainDir, "commit", "-m", "init")
		if _, err := cmd.CombinedOutput(); err != nil {
			b.Fatal(err)
		}

		names := make([]string, 5)
		for j := range 5 {
			name := fmt.Sprintf("bench-%d", j)
			names[j] = name
			dir := filepath.Join(tmpDir, name)
			cmd = exec.Command("git", "-C", bareDir, "worktree", "add", "-b", name, dir, "main")
			if out, err := cmd.CombinedOutput(); err != nil {
				b.Fatalf("add worktree %s: %v\n%s", name, err, out)
			}
		}

		origDir, _ := os.Getwd()
		if err := os.Chdir(mainDir); err != nil {
			b.Fatal(err)
		}

		client := NewClient(Options{})

		b.StartTimer()
		client.RemoveBatch(names, RemoveOptions{Force: true, DeleteBranch: true})
		b.StopTimer()

		_ = os.Chdir(origDir)
	}
}
