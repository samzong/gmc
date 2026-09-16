package worktree

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBaseBranchPolicies(t *testing.T) {
	for _, tc := range []struct {
		name, branch, want string
		remotes            []string
	}{
		{"origin preferred", "main", "origin/main", []string{"origin", "upstream"}},
		{"upstream fallback", "main", "upstream/main", []string{"upstream"}},
		{"local main", "main", "main", nil},
		{"local master", "master", "master", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repoDir := initTestRepoWithBranch(t, tc.branch)
			for _, remote := range tc.remotes {
				ref := "refs/remotes/" + remote + "/main"
				runGit(t, repoDir, "update-ref", ref, "HEAD")
				runGit(t, repoDir, "symbolic-ref", "refs/remotes/"+remote+"/HEAD", ref)
			}
			client := NewClient(Options{})
			resolvers := []func(string, string) (string, error){client.resolveBaseBranch, client.resolveSyncBaseBranch}
			for _, resolve := range resolvers {
				base, err := resolve(repoDir, "")
				require.NoError(t, err)
				assert.Equal(t, tc.want, base)
			}
		})
	}
}

func TestSelectSyncRemote(t *testing.T) {
	for _, remotes := range [][]string{{"origin", "upstream"}, {"origin"}, nil} {
		t.Run(strings.Join(remotes, "+"), func(t *testing.T) {
			repoDir := initTestRepo(t)
			for _, remote := range remotes {
				runGit(t, repoDir, "remote", "add", remote, "https://example.com/"+remote+"/repo.git")
			}
			remote, err := NewClient(Options{}).selectSyncRemote(repoDir)
			if len(remotes) == 0 {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, remotes[len(remotes)-1], remote)
		})
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
	commitFiles(t, advanceRepoDir, "upstream", ".")
	runGit(t, advanceRepoDir, "push", "origin", "main")

	t.Chdir(repoDir)

	client := NewClient(Options{})
	_, err := client.Sync(SyncOptions{})
	require.NoError(t, err)

	localHash := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/heads/main"))
	upstreamHash := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "refs/remotes/upstream/main"))
	originHash := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/main"))

	assert.Equal(t, upstreamHash, localHash)
	assert.Equal(t, upstreamHash, originHash)
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
			assert.Equal(t, tt.expected, localBranchName(tt.input))
		})
	}
}

func TestSyncDryRunKeepsLocalAndRemoteState(t *testing.T) {
	repo := initTestRepo(t)
	remote := initBareRepo(t)
	runGit(t, repo, "remote", "add", "origin", remote)
	runGit(t, repo, "push", "origin", "main")
	before := runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "commit", "--allow-empty", "-m", "remote advance")
	runGit(t, repo, "push", "origin", "main")
	runGit(t, repo, "reset", "--hard", "HEAD~1")
	t.Chdir(repo)
	report, err := NewClient(Options{}).Sync(SyncOptions{DryRun: true})
	require.NoError(t, err)
	require.Equal(t, before, runGit(t, repo, "rev-parse", "HEAD"))
	require.NotEqual(t, before, runGit(t, remote, "rev-parse", "main"))
	require.Contains(t, report.Events, Event{EventWarn, "Would fast-forward main to origin/main"})
	require.Contains(t, report.Events, Event{EventWarn, "Would update worktree: " + repo})
}
