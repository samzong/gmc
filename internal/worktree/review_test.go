package worktree

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
			require.Equal(t, tt.want, reviewProviderFromRemoteURL(tt.remoteURL))
		})
	}
}

func TestReviewStates_ProvidersAndFreshCache(t *testing.T) {
	for _, tc := range []struct {
		provider, remote, tool, branch, output string
		number                                 int
	}{
		{reviewProviderGitHub, "https://github.com/org/repo.git", "gh", "feature/github", `[
			{
				"number": 42,
				"state": "OPEN",
				"headRefName": "feature/github",
				"headRefOid": "HEAD_COMMIT",
				"url": "https://github.com/org/repo/pull/42"
			}
		]`, 42},
		{reviewProviderGitLab, "https://gitlab.com/group/repo.git", "glab", "feature/gitlab", `[
			{
				"iid": 7,
				"state": "opened",
				"source_branch": "feature/gitlab",
				"sha": "HEAD_COMMIT",
				"web_url": "https://gitlab.com/group/repo/-/merge_requests/7"
			}
		]`, 7},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			useTempReviewCache(t)
			worktrees := runReviewLookupTest(t, tc.remote, tc.tool, tc.output, tc.branch)
			first := NewClient(Options{}).ReviewStates(worktrees)
			require.Empty(t, first.Warning)
			review := first.Reviews[tc.branch]
			require.Equal(t, tc.provider, review.Provider)
			require.Equal(t, tc.number, review.Number)
			require.Equal(t, "OPEN", review.State)
			reviewRunFunc = func(repoDir string, tool string, args ...string) ([]byte, error) {
				t.Fatalf("review lookup should use fresh cache before user lookup: %s %v", tool, args)
				return nil, nil
			}
			require.Equal(t, first, NewClient(Options{}).ReviewStates(worktrees))
		})
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
	require.Equal(t, "", result.Warning)
	require.NotContains(t, result.Reviews, "not-local")
	review := result.Reviews["feature/github"]
	require.Equal(t, 43, review.Number)
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
	require.Equal(t, "", result.Warning)
	review := result.Reviews["feature/github"]
	require.Equal(t, 43, review.Number)
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

	t.Chdir(repoDir)

	result := NewClient(Options{}).ReviewStates([]Info{{Branch: "feature/unpushed", Commit: head}})
	require.Equal(t, "", result.Warning)
	require.Len(t, result.Reviews, 0)
}

func TestReviewStates_WarnsOnMissingCLI(t *testing.T) {
	useTempReviewCache(t)
	runReviewLookupFailureTest(t, func(repoDir string, tool string, args ...string) ([]byte, error) {
		return nil, missingReviewToolError{tool: tool}
	})

	result := NewClient(Options{}).ReviewStates([]Info{{Branch: "feature/github", Commit: ""}})
	require.Contains(t, result.Warning, "gh CLI not found")
	require.Equal(t, map[string]ReviewInfo{}, result.Reviews)
}

func TestReviewStates_WarnsOnAuthFailure(t *testing.T) {
	useTempReviewCache(t)
	runReviewLookupFailureTest(t, func(repoDir string, tool string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("%s failed: authentication required", tool)
	})

	result := NewClient(Options{}).ReviewStates([]Info{{Branch: "feature/github", Commit: ""}})
	require.Contains(t, result.Warning, "check authentication")
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
		require.Equal(t, wantTool, tool)
		callCount++
		if tool == "glab" && hasReviewArg(args, "api", "user") {
			return []byte(`{"username":"test-user"}`), nil
		}
		require.True(t, hasReviewArg(args, "-R", remoteURL))
		switch tool {
		case "gh":
			require.True(t, hasReviewArg(args, "--author", "@me"))
			return []byte(strings.ReplaceAll(output, "HEAD_COMMIT", head)), nil
		case "glab":
			require.True(t, hasReviewArg(args, "--author", "test-user"))
			require.Equal(t, 2, callCount)
			return []byte(strings.ReplaceAll(output, "HEAD_COMMIT", head)), nil
		default:
			return []byte(output), nil
		}
	}

	t.Chdir(repoDir)
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

	t.Chdir(repoDir)
}
