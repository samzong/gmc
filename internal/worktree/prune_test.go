package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGhPRStates_ParsesBatchResponse(t *testing.T) {
	prs := []ghPRInfo{
		{Number: 10, State: "MERGED", HeadRefName: "feat-a"},
		{Number: 11, State: "OPEN", HeadRefName: "feat-b"},
		{Number: 12, State: "CLOSED", HeadRefName: "feat-c"},
	}
	data, _ := json.Marshal(prs)

	stubPRResponse(t, data, nil)

	m, err := ghPRStates("/tmp")
	require.NoError(t, err)
	require.Len(t, m, 3)
	assert.Equal(t, prs[0], m["feat-a"])
	assert.Equal(t, "OPEN", m["feat-b"].State)
	assert.Equal(t, "CLOSED", m["feat-c"].State)
}

func TestGhPRStates_NormalizesCase(t *testing.T) {
	data := `[{"number":1,"state":"merged","headRefName":"br"}]`

	stubPRResponse(t, []byte(data), nil)

	m, err := ghPRStates("/tmp")
	require.NoError(t, err)
	assert.Equal(t, "MERGED", m["br"].State)
}

func TestGhPRStates_EmptyResponse(t *testing.T) {
	stubPRResponse(t, []byte("[]"), nil)

	m, err := ghPRStates("/tmp")
	require.NoError(t, err)
	assert.Len(t, m, 0)
}

func TestGhPRStates_FirstPRWins(t *testing.T) {
	data := `[{"number":1,"state":"OPEN","headRefName":"br"},{"number":2,"state":"MERGED","headRefName":"br"}]`

	stubPRResponse(t, []byte(data), nil)

	m, err := ghPRStates("/tmp")
	require.NoError(t, err)
	assert.Equal(t, 1, m["br"].Number)
}

func TestPrunePRAware_DecisionMatrix(t *testing.T) {
	repoDir := initTestRepo(t)

	branches := []struct {
		name    string
		prState string
	}{
		{"feat-merged", "MERGED"},
		{"feat-open", "OPEN"},
		{"feat-closed", "CLOSED"},
		{"feat-nopr", ""},
	}

	for _, b := range branches {
		wtDir := filepath.Join(repoDir, b.name)
		runGit(t, repoDir, "worktree", "add", "-b", b.name, wtDir, "main")
	}

	data, _ := json.Marshal([]ghPRInfo{
		{Number: 10, State: "MERGED", HeadRefName: "feat-merged"},
		{Number: 11, State: "OPEN", HeadRefName: "feat-open"},
		{Number: 12, State: "CLOSED", HeadRefName: "feat-closed"},
	})

	stubPRResponse(t, data, nil)

	t.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{DryRun: true, PRAware: true})
	require.NoError(t, err)

	entryMap := make(map[string]PruneEntry)
	for _, e := range result.PruneEntries {
		entryMap[e.Branch] = e
	}

	tests := []struct {
		branch     string
		wantAction string
		wantPRNum  int
	}{
		{"feat-merged", "would_remove", 10},
		{"feat-open", "skipped", 11},
		{"feat-closed", "skipped", 12},
		{"feat-nopr", "skipped", 0},
	}

	for _, tt := range tests {
		e, ok := entryMap[tt.branch]
		if !ok {
			t.Errorf("missing entry for %s", tt.branch)
			continue
		}
		assert.Equal(t, tt.wantAction, e.Action)
		assert.Equal(t, tt.wantPRNum, e.PRNum)
	}
}

func TestPrunePRAware_GhFailure(t *testing.T) {
	repoDir := initTestRepo(t)

	wtDir := filepath.Join(repoDir, "feat-x")
	runGit(t, repoDir, "worktree", "add", "-b", "feat-x", wtDir, "main")

	stubPRResponse(t, nil, errors.New("auth required"))

	t.Chdir(repoDir)

	client := NewClient(Options{})
	_, err := client.Prune(PruneOptions{PRAware: true})
	require.Error(t, err)
}

func TestPrunePRAware_DirtyWorktree(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			repo := initTestRepo(t)
			wt := filepath.Join(repo, "feat-dirty")
			runGit(t, repo, "worktree", "add", "-b", "feat-dirty", wt, "main")
			writeFile(t, filepath.Join(wt, "dirty.txt"), "staged")
			commitFiles(t, wt, "add file", "dirty.txt")
			writeFile(t, filepath.Join(wt, "dirty.txt"), "modified after commit")
			stubPRResponse(t, []byte(`[{"number":1,"state":"MERGED","headRefName":"feat-dirty"}]`), nil)
			t.Chdir(repo)
			client := NewClient(Options{})
			result, err := client.Prune(PruneOptions{PRAware: true, Force: force})
			require.NoError(t, err)
			require.Len(t, result.PruneEntries, 1)
			if force {
				assert.Equal(t, "removed", result.PruneEntries[0].Action)
				assertMissing(t, wt)
				assert.False(t, branchExists(t, client, repo, "feat-dirty"))
			} else {
				assert.Equal(t, "skipped", result.PruneEntries[0].Action)
				assert.DirExists(t, wt)
			}
		})
	}
}

func branchExists(t *testing.T, client *Client, repoDir, branch string) bool {
	t.Helper()
	_, err := client.runner.Run("-C", repoDir, "rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}

func setupOrphanBranchRepo(t *testing.T) string {
	t.Helper()
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "branch", "orphan-merged", "main")
	runGit(t, repoDir, "checkout", "-q", "orphan-merged")
	writeFile(t, filepath.Join(repoDir, "merged.txt"), "merged")
	commitFiles(t, repoDir, "merged work", ".")
	runGit(t, repoDir, "checkout", "-q", "main")
	runGit(t, repoDir, "merge", "--no-ff", "-m", "merge orphan", "orphan-merged")

	runGit(t, repoDir, "branch", "orphan-unmerged", "main")
	runGit(t, repoDir, "checkout", "-q", "orphan-unmerged")
	writeFile(t, filepath.Join(repoDir, "unmerged.txt"), "unmerged")
	commitFiles(t, repoDir, "unmerged work", ".")
	runGit(t, repoDir, "checkout", "-q", "main")

	return repoDir
}

func TestPruneBranches_Classic(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts PruneOptions
	}{
		{"remove merged orphan", PruneOptions{Branches: true}},
		{"dry run", PruneOptions{Branches: true, DryRun: true}},
		{"ignore orphans", PruneOptions{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupOrphanBranchRepo(t)
			t.Chdir(repo)
			client := NewClient(Options{})
			result, err := client.Prune(tc.opts)
			require.NoError(t, err)
			if tc.opts.Branches {
				require.Len(t, result.Candidates, 1)
				assert.Equal(t, "orphan-merged", result.Candidates[0].Branch)
				assert.Empty(t, result.Candidates[0].Name)
			} else {
				assert.Empty(t, result.Candidates)
			}
			assert.Equal(t, !tc.opts.Branches || tc.opts.DryRun, branchExists(t, client, repo, "orphan-merged"))
			assert.True(t, branchExists(t, client, repo, "orphan-unmerged"))
			assert.True(t, branchExists(t, client, repo, "main"))
		})
	}
}

func TestPruneBranches_PRAwareDeletesMergedOrphan(t *testing.T) {
	repoDir := setupOrphanBranchRepo(t)

	wtDir := filepath.Join(repoDir, "feat-wt")
	runGit(t, repoDir, "worktree", "add", "-b", "feat-wt", wtDir, "main")

	data, _ := json.Marshal([]ghPRInfo{
		{Number: 20, State: "MERGED", HeadRefName: "orphan-unmerged"},
		{Number: 21, State: "OPEN", HeadRefName: "feat-wt"},
	})
	stubPRResponse(t, data, nil)

	t.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{Branches: true, PRAware: true})
	require.NoError(t, err)

	entries := make(map[string]PruneEntry)
	for _, e := range result.PruneEntries {
		entries[e.Branch] = e
	}
	assert.Equal(t, "removed", entries["orphan-unmerged"].Action)
	assert.Empty(t, entries["orphan-unmerged"].Name)
	assert.Equal(t, "skipped", entries["orphan-merged"].Action)
	assert.Equal(t, "no PR found", entries["orphan-merged"].Reason)
	assert.Equal(t, "skipped", entries["feat-wt"].Action)
	assert.Equal(t, "feat-wt", entries["feat-wt"].Name)
	assert.False(t, branchExists(t, client, repoDir, "orphan-unmerged"))
	assert.True(t, branchExists(t, client, repoDir, "orphan-merged"))
}

func TestIsBranchMerged(t *testing.T) {
	repoDir := initTestRepo(t)

	runGit(t, repoDir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(repoDir, "feature.txt"), "feature")
	commitFiles(t, repoDir, "feature", ".")
	runGit(t, repoDir, "checkout", "main")
	runGit(t, repoDir, "merge", "--no-ff", "-m", "merge feature", "feature")

	client := NewClient(Options{})
	merged, err := client.isBranchMerged(repoDir, "feature", "main")
	require.NoError(t, err)
	assert.True(t, merged)

	runGit(t, repoDir, "checkout", "-b", "wip")
	writeFile(t, filepath.Join(repoDir, "wip.txt"), "wip")
	commitFiles(t, repoDir, "wip", ".")
	runGit(t, repoDir, "checkout", "main")

	merged, err = client.isBranchMerged(repoDir, "wip", "main")
	require.NoError(t, err)
	assert.False(t, merged)
}

func TestPrune_RemovesMergedWorktreeAndBranch(t *testing.T) {
	repoDir := initTestRepo(t)

	worktreeDir := filepath.Join(repoDir, "feature-wt")
	runGit(t, repoDir, "worktree", "add", "-b", "feature", worktreeDir, "main")
	writeFile(t, filepath.Join(worktreeDir, "feature.txt"), "feature")
	commitFiles(t, worktreeDir, "feature", ".")
	runGit(t, repoDir, "merge", "--no-ff", "-m", "merge feature", "feature")

	t.Chdir(repoDir)

	client := NewClient(Options{})
	_, err := client.Prune(PruneOptions{})
	require.NoError(t, err)

	assertMissing(t, worktreeDir)

	if _, err := client.runner.Run("-C", repoDir, "rev-parse", "--verify", "refs/heads/feature"); err == nil {
		t.Error("Expected feature branch to be deleted")
	}
}

func stubPRResponse(t *testing.T, data []byte, err error) {
	t.Helper()
	previous := ghRunFunc
	t.Cleanup(func() { ghRunFunc = previous })
	ghRunFunc = func(string, ...string) ([]byte, error) { return data, err }
}
