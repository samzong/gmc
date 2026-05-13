package worktree

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestReviewProviderFromRemoteURL(t *testing.T) {
	t.Setenv("GITLAB_HOST", "gitlab.internal.example")

	tests := []struct {
		name      string
		remoteURL string
		want      string
	}{
		{"github https", "https://github.com/org/repo.git", reviewProviderGitHub},
		{"github ssh", "git@github.com:org/repo.git", reviewProviderGitHub},
		{"gitlab https", "https://gitlab.com/group/repo.git", reviewProviderGitLab},
		{"gitlab host env", "git@gitlab.internal.example:group/repo.git", reviewProviderGitLab},
		{"unsupported", "https://example.com/org/repo.git", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reviewProviderFromRemoteURL(tt.remoteURL); got != tt.want {
				t.Fatalf("reviewProviderFromRemoteURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReviewStates_GitHub(t *testing.T) {
	useTempReviewCache(t)
	worktrees := runReviewLookupTest(t, "https://github.com/org/repo.git", "gh", `[
		{
			"number": 42,
			"state": "OPEN",
			"headRefName": "feature/github",
			"headRefOid": "HEAD_COMMIT",
			"url": "https://github.com/org/repo/pull/42"
		}
	]`, "feature/github")

	result := NewClient(Options{}).ReviewStates(worktrees)
	if result.Warning != "" {
		t.Fatalf("Warning = %q, want empty", result.Warning)
	}
	review := result.Reviews["feature/github"]
	if review.Provider != reviewProviderGitHub || review.Number != 42 || review.State != "OPEN" {
		t.Fatalf("unexpected review: %+v", review)
	}
}

func TestReviewStates_GitHubOnlyMatchesPushedWorktreeBranches(t *testing.T) {
	useTempReviewCache(t)
	worktrees := runReviewLookupTest(t, "https://github.com/org/repo.git", "gh", `[
			{
				"number": 42,
				"state": "OPEN",
				"headRefName": "not-local",
				"url": "https://github.com/org/repo/pull/42"
			},
			{
				"number": 43,
				"state": "OPEN",
				"headRefName": "feature/github",
				"url": "https://github.com/org/repo/pull/43"
			}
		]`, "feature/github")

	result := NewClient(Options{}).ReviewStates(worktrees)
	if result.Warning != "" {
		t.Fatalf("Warning = %q, want empty", result.Warning)
	}
	if _, ok := result.Reviews["not-local"]; ok {
		t.Fatalf("Reviews[not-local] = %+v, want no non-worktree branch match", result.Reviews["not-local"])
	}
	review := result.Reviews["feature/github"]
	if review.Number != 43 {
		t.Fatalf("Reviews[feature/github].Number = %d, want 43", review.Number)
	}
}

func TestReviewStates_GitHubPrefersExactHeadCommit(t *testing.T) {
	useTempReviewCache(t)
	worktrees := runReviewLookupTest(t, "https://github.com/org/repo.git", "gh", `[
			{
				"number": 42,
				"state": "OPEN",
				"headRefName": "feature/github",
				"headRefOid": "old-commit",
				"url": "https://github.com/org/repo/pull/42"
			},
			{
				"number": 43,
				"state": "OPEN",
				"headRefName": "feature/github",
				"headRefOid": "HEAD_COMMIT",
				"url": "https://github.com/org/repo/pull/43"
			}
		]`, "feature/github")

	result := NewClient(Options{}).ReviewStates(worktrees)
	if result.Warning != "" {
		t.Fatalf("Warning = %q, want empty", result.Warning)
	}
	review := result.Reviews["feature/github"]
	if review.Number != 43 {
		t.Fatalf("Reviews[feature/github].Number = %d, want exact commit PR 43", review.Number)
	}
}

func TestReviewStates_GitLab(t *testing.T) {
	useTempReviewCache(t)
	worktrees := runReviewLookupTest(t, "https://gitlab.com/group/repo.git", "glab", `[
		{
			"iid": 7,
			"state": "opened",
			"source_branch": "feature/gitlab",
			"sha": "HEAD_COMMIT",
			"web_url": "https://gitlab.com/group/repo/-/merge_requests/7"
		}
	]`, "feature/gitlab")

	result := NewClient(Options{}).ReviewStates(worktrees)
	if result.Warning != "" {
		t.Fatalf("Warning = %q, want empty", result.Warning)
	}
	review := result.Reviews["feature/gitlab"]
	if review.Provider != reviewProviderGitLab || review.Number != 7 || review.State != "OPEN" {
		t.Fatalf("unexpected review: %+v", review)
	}
}

func TestReviewStates_UsesFreshCache(t *testing.T) {
	useTempReviewCache(t)
	worktrees := runReviewLookupTest(t, "https://github.com/org/repo.git", "gh", `[
		{
			"number": 42,
			"state": "OPEN",
			"headRefName": "feature/github",
			"headRefOid": "HEAD_COMMIT",
			"url": "https://github.com/org/repo/pull/42"
		}
	]`, "feature/github")

	first := NewClient(Options{}).ReviewStates(worktrees)
	if first.Warning != "" {
		t.Fatalf("Warning = %q, want empty", first.Warning)
	}

	oldRun := reviewRunFunc
	reviewRunFunc = func(repoDir string, tool string, args ...string) ([]byte, error) {
		t.Fatalf("review lookup should use fresh cache, got %s %v", tool, args)
		return nil, nil
	}
	t.Cleanup(func() { reviewRunFunc = oldRun })

	second := NewClient(Options{}).ReviewStates(worktrees)
	if second.Warning != "" {
		t.Fatalf("Warning = %q, want empty", second.Warning)
	}
	review := second.Reviews["feature/github"]
	if review.Number != 42 {
		t.Fatalf("Reviews[feature/github].Number = %d, want cached PR 42", review.Number)
	}
}

func TestReviewStates_GitLabUsesFreshCacheBeforeUserLookup(t *testing.T) {
	useTempReviewCache(t)
	worktrees := runReviewLookupTest(t, "https://gitlab.com/group/repo.git", "glab", `[
		{
			"iid": 7,
			"state": "opened",
			"source_branch": "feature/gitlab",
			"sha": "HEAD_COMMIT",
			"web_url": "https://gitlab.com/group/repo/-/merge_requests/7"
		}
	]`, "feature/gitlab")

	first := NewClient(Options{}).ReviewStates(worktrees)
	if first.Warning != "" {
		t.Fatalf("Warning = %q, want empty", first.Warning)
	}

	oldRun := reviewRunFunc
	reviewRunFunc = func(repoDir string, tool string, args ...string) ([]byte, error) {
		t.Fatalf("GitLab review lookup should use fresh cache before user lookup, got %s %v", tool, args)
		return nil, nil
	}
	t.Cleanup(func() { reviewRunFunc = oldRun })

	second := NewClient(Options{}).ReviewStates(worktrees)
	if second.Warning != "" {
		t.Fatalf("Warning = %q, want empty", second.Warning)
	}
	review := second.Reviews["feature/gitlab"]
	if review.Number != 7 {
		t.Fatalf("Reviews[feature/gitlab].Number = %d, want cached MR 7", review.Number)
	}
}

func TestReviewStates_SkipsUnpushedBranches(t *testing.T) {
	repoDir := initTestRepo(t)
	runGit(t, repoDir, "remote", "add", "origin", "https://github.com/org/repo.git")
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	oldRun := reviewRunFunc
	t.Cleanup(func() { reviewRunFunc = oldRun })
	reviewRunFunc = func(repoDir string, tool string, args ...string) ([]byte, error) {
		t.Fatalf("review lookup should not run for unpushed branch: %s %v", tool, args)
		return nil, nil
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	result := NewClient(Options{}).ReviewStates([]Info{{Branch: "feature/unpushed", Commit: head}})
	if result.Warning != "" {
		t.Fatalf("Warning = %q, want empty", result.Warning)
	}
	if len(result.Reviews) != 0 {
		t.Fatalf("Reviews = %+v, want empty", result.Reviews)
	}
}

func TestReviewStates_WarnsOnMissingCLI(t *testing.T) {
	useTempReviewCache(t)
	runReviewLookupFailureTest(t, func(repoDir string, tool string, args ...string) ([]byte, error) {
		return nil, missingReviewToolError{tool: tool}
	})

	result := NewClient(Options{}).ReviewStates([]Info{{Branch: "feature/github", Commit: ""}})
	if !strings.Contains(result.Warning, "gh CLI not found") {
		t.Fatalf("Warning = %q, want missing gh warning", result.Warning)
	}
	if result.Reviews == nil || len(result.Reviews) != 0 {
		t.Fatalf("Reviews = %+v, want empty non-nil map", result.Reviews)
	}
}

func TestReviewStates_WarnsOnAuthFailure(t *testing.T) {
	useTempReviewCache(t)
	runReviewLookupFailureTest(t, func(repoDir string, tool string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("%s failed: authentication required", tool)
	})

	result := NewClient(Options{}).ReviewStates([]Info{{Branch: "feature/github", Commit: ""}})
	if !strings.Contains(result.Warning, "check authentication") {
		t.Fatalf("Warning = %q, want authentication warning", result.Warning)
	}
}

func useTempReviewCache(t *testing.T) {
	t.Helper()
	old := reviewCacheDirFunc
	dir := t.TempDir()
	reviewCacheDirFunc = func() (string, error) {
		return dir, nil
	}
	t.Cleanup(func() { reviewCacheDirFunc = old })
}

func runReviewLookupTest(
	t *testing.T,
	remoteURL string,
	wantTool string,
	output string,
	branches ...string,
) []Info {
	t.Helper()
	repoDir := initTestRepo(t)
	runGit(t, repoDir, "remote", "add", "origin", remoteURL)
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	worktrees := make([]Info, 0, len(branches))
	for _, branch := range branches {
		runGit(t, repoDir, "update-ref", "refs/remotes/origin/"+branch, head)
		worktrees = append(worktrees, Info{Branch: branch, Commit: head})
	}

	oldRun := reviewRunFunc
	t.Cleanup(func() { reviewRunFunc = oldRun })
	callCount := 0
	reviewRunFunc = func(repoDir string, tool string, args ...string) ([]byte, error) {
		if tool != wantTool {
			t.Fatalf("tool = %q, want %s", tool, wantTool)
		}
		callCount++
		if tool == "glab" && hasReviewArg(args, "api", "user") {
			return []byte(`{"username":"test-user"}`), nil
		}
		if !hasReviewArg(args, "-R", remoteURL) {
			t.Fatalf("args = %v, want -R %s", args, remoteURL)
		}
		switch tool {
		case "gh":
			if !hasReviewArg(args, "--author", "@me") {
				t.Fatalf("args = %v, want --author @me", args)
			}
			return []byte(strings.ReplaceAll(output, "HEAD_COMMIT", head)), nil
		case "glab":
			if !hasReviewArg(args, "--author", "test-user") {
				t.Fatalf("args = %v, want --author test-user", args)
			}
			if callCount != 2 {
				t.Fatalf("glab mr list call count = %d, want 2 after user lookup", callCount)
			}
			return []byte(strings.ReplaceAll(output, "HEAD_COMMIT", head)), nil
		default:
			return []byte(output), nil
		}
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	return worktrees
}

func hasReviewArg(args []string, flag string, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func runReviewLookupFailureTest(
	t *testing.T,
	run func(repoDir string, tool string, args ...string) ([]byte, error),
) {
	t.Helper()
	repoDir := initTestRepo(t)
	runGit(t, repoDir, "remote", "add", "origin", "https://github.com/org/repo.git")
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	runGit(t, repoDir, "update-ref", "refs/remotes/origin/feature/github", head)

	oldRun := reviewRunFunc
	t.Cleanup(func() { reviewRunFunc = oldRun })
	reviewRunFunc = run

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
}
