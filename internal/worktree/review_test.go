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
	runReviewLookupTest(t, "https://github.com/org/repo.git", "gh", `[
		{
			"number": 42,
			"state": "OPEN",
			"headRefName": "feature/github",
			"url": "https://github.com/org/repo/pull/42"
		}
	]`)

	result := NewClient(Options{}).ReviewStates()
	if result.Warning != "" {
		t.Fatalf("Warning = %q, want empty", result.Warning)
	}
	review := result.Reviews["feature/github"]
	if review.Provider != reviewProviderGitHub || review.Number != 42 || review.State != "OPEN" {
		t.Fatalf("unexpected review: %+v", review)
	}
}

func TestReviewStates_GitLab(t *testing.T) {
	runReviewLookupTest(t, "https://gitlab.com/group/repo.git", "glab", `[
		{
			"iid": 7,
			"state": "opened",
			"source_branch": "feature/gitlab",
			"web_url": "https://gitlab.com/group/repo/-/merge_requests/7"
		}
	]`)

	result := NewClient(Options{}).ReviewStates()
	if result.Warning != "" {
		t.Fatalf("Warning = %q, want empty", result.Warning)
	}
	review := result.Reviews["feature/gitlab"]
	if review.Provider != reviewProviderGitLab || review.Number != 7 || review.State != "OPEN" {
		t.Fatalf("unexpected review: %+v", review)
	}
}

func TestReviewStates_WarnsOnMissingCLI(t *testing.T) {
	runReviewLookupFailureTest(t, func(repoDir string, tool string, args ...string) ([]byte, error) {
		return nil, missingReviewToolError{tool: tool}
	})

	result := NewClient(Options{}).ReviewStates()
	if !strings.Contains(result.Warning, "gh CLI not found") {
		t.Fatalf("Warning = %q, want missing gh warning", result.Warning)
	}
	if result.Reviews == nil || len(result.Reviews) != 0 {
		t.Fatalf("Reviews = %+v, want empty non-nil map", result.Reviews)
	}
}

func TestReviewStates_WarnsOnAuthFailure(t *testing.T) {
	runReviewLookupFailureTest(t, func(repoDir string, tool string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("%s failed: authentication required", tool)
	})

	result := NewClient(Options{}).ReviewStates()
	if !strings.Contains(result.Warning, "check authentication") {
		t.Fatalf("Warning = %q, want authentication warning", result.Warning)
	}
}

func runReviewLookupTest(t *testing.T, remoteURL string, wantTool string, output string) {
	t.Helper()
	repoDir := initTestRepo(t)
	runGit(t, repoDir, "remote", "add", "origin", remoteURL)

	oldRun := reviewRunFunc
	t.Cleanup(func() { reviewRunFunc = oldRun })
	reviewRunFunc = func(repoDir string, tool string, args ...string) ([]byte, error) {
		if tool != wantTool {
			t.Fatalf("tool = %q, want %s", tool, wantTool)
		}
		if !hasReviewArg(args, "-R", remoteURL) {
			t.Fatalf("args = %v, want -R %s", args, remoteURL)
		}
		return []byte(output), nil
	}

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
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
