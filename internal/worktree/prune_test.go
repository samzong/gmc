package worktree

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGhPRStates_ParsesBatchResponse(t *testing.T) {
	prs := []ghPRInfo{
		{Number: 10, State: "MERGED", HeadRefName: "feat-a"},
		{Number: 11, State: "OPEN", HeadRefName: "feat-b"},
		{Number: 12, State: "CLOSED", HeadRefName: "feat-c"},
	}
	data, _ := json.Marshal(prs)

	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(repoDir string, args ...string) ([]byte, error) {
		return data, nil
	}

	m, err := ghPRStates("/tmp")
	if err != nil {
		t.Fatalf("ghPRStates() error = %v", err)
	}
	if len(m) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m))
	}
	if m["feat-a"].State != "MERGED" || m["feat-a"].Number != 10 {
		t.Errorf("feat-a = %+v", m["feat-a"])
	}
	if m["feat-b"].State != "OPEN" {
		t.Errorf("feat-b = %+v", m["feat-b"])
	}
	if m["feat-c"].State != "CLOSED" {
		t.Errorf("feat-c = %+v", m["feat-c"])
	}
}

func TestGhPRStates_NormalizesCase(t *testing.T) {
	data := `[{"number":1,"state":"merged","headRefName":"br"}]`

	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(repoDir string, args ...string) ([]byte, error) {
		return []byte(data), nil
	}

	m, err := ghPRStates("/tmp")
	if err != nil {
		t.Fatalf("ghPRStates() error = %v", err)
	}
	if m["br"].State != "MERGED" {
		t.Errorf("expected MERGED, got %q", m["br"].State)
	}
}

func TestGhPRStates_EmptyResponse(t *testing.T) {
	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(repoDir string, args ...string) ([]byte, error) {
		return []byte("[]"), nil
	}

	m, err := ghPRStates("/tmp")
	if err != nil {
		t.Fatalf("ghPRStates() error = %v", err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}

func TestGhPRStates_FirstPRWins(t *testing.T) {
	data := `[{"number":1,"state":"OPEN","headRefName":"br"},{"number":2,"state":"MERGED","headRefName":"br"}]`

	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(repoDir string, args ...string) ([]byte, error) {
		return []byte(data), nil
	}

	m, err := ghPRStates("/tmp")
	if err != nil {
		t.Fatalf("ghPRStates() error = %v", err)
	}
	if m["br"].Number != 1 {
		t.Errorf("expected first PR (#1), got #%d", m["br"].Number)
	}
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

	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(dir string, args ...string) ([]byte, error) {
		return data, nil
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{DryRun: true, PRAware: true})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

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
		if e.Action != tt.wantAction {
			t.Errorf("%s: action = %q, want %q", tt.branch, e.Action, tt.wantAction)
		}
		if e.PRNum != tt.wantPRNum {
			t.Errorf("%s: PRNum = %d, want %d", tt.branch, e.PRNum, tt.wantPRNum)
		}
	}
}

func TestPrunePRAware_GhFailure(t *testing.T) {
	repoDir := initTestRepo(t)

	wtDir := filepath.Join(repoDir, "feat-x")
	runGit(t, repoDir, "worktree", "add", "-b", "feat-x", wtDir, "main")

	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(dir string, args ...string) ([]byte, error) {
		return nil, errors.New("auth required")
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(repoDir)

	client := NewClient(Options{})
	_, err := client.Prune(PruneOptions{PRAware: true})
	if err == nil {
		t.Fatal("expected error when gh fails")
	}
}

func TestPrunePRAware_DirtyWorktreeSkipped(t *testing.T) {
	repoDir := initTestRepo(t)

	wtDir := filepath.Join(repoDir, "feat-dirty")
	runGit(t, repoDir, "worktree", "add", "-b", "feat-dirty", wtDir, "main")
	writeFile(t, filepath.Join(wtDir, "dirty.txt"), "staged")
	runGit(t, wtDir, "add", "dirty.txt")
	runGit(t, wtDir, "commit", "-m", "add file")
	writeFile(t, filepath.Join(wtDir, "dirty.txt"), "modified after commit")

	data := `[{"number":1,"state":"MERGED","headRefName":"feat-dirty"}]`
	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(dir string, args ...string) ([]byte, error) {
		return []byte(data), nil
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{PRAware: true})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(result.PruneEntries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.PruneEntries))
	}
	e := result.PruneEntries[0]
	if e.Action != "skipped" {
		t.Errorf("action = %q, want skipped", e.Action)
	}
	if _, err := os.Stat(wtDir); os.IsNotExist(err) {
		t.Error("worktree should NOT have been removed")
	}
}

func TestPrunePRAware_ForceRemovesDirty(t *testing.T) {
	repoDir := initTestRepo(t)

	wtDir := filepath.Join(repoDir, "feat-dirty2")
	runGit(t, repoDir, "worktree", "add", "-b", "feat-dirty2", wtDir, "main")
	writeFile(t, filepath.Join(wtDir, "dirty.txt"), "staged")
	runGit(t, wtDir, "add", "dirty.txt")
	runGit(t, wtDir, "commit", "-m", "add file")
	writeFile(t, filepath.Join(wtDir, "dirty.txt"), "modified after commit")

	data := `[{"number":1,"state":"MERGED","headRefName":"feat-dirty2"}]`
	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(dir string, args ...string) ([]byte, error) {
		return []byte(data), nil
	}

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{PRAware: true, Force: true})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(result.PruneEntries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.PruneEntries))
	}
	e := result.PruneEntries[0]
	if e.Action != "removed" {
		t.Errorf("action = %q, want removed", e.Action)
	}

	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Error("worktree should have been removed with --force")
	}

	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--verify", "refs/heads/feat-dirty2")
	if err := cmd.Run(); err == nil {
		t.Error("expected branch to be deleted")
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
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "merged work")
	runGit(t, repoDir, "checkout", "-q", "main")
	runGit(t, repoDir, "merge", "--no-ff", "-m", "merge orphan", "orphan-merged")

	runGit(t, repoDir, "branch", "orphan-unmerged", "main")
	runGit(t, repoDir, "checkout", "-q", "orphan-unmerged")
	writeFile(t, filepath.Join(repoDir, "unmerged.txt"), "unmerged")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "unmerged work")
	runGit(t, repoDir, "checkout", "-q", "main")

	return repoDir
}

func TestPruneBranches_ClassicDeletesMergedOrphanOnly(t *testing.T) {
	repoDir := setupOrphanBranchRepo(t)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{Branches: true})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(result.Candidates) != 1 || result.Candidates[0].Branch != "orphan-merged" {
		t.Fatalf("candidates = %+v, want only orphan-merged", result.Candidates)
	}
	if result.Candidates[0].Name != "" {
		t.Errorf("orphan candidate name = %q, want empty", result.Candidates[0].Name)
	}
	if branchExists(t, client, repoDir, "orphan-merged") {
		t.Error("orphan-merged should have been deleted")
	}
	if !branchExists(t, client, repoDir, "orphan-unmerged") {
		t.Error("orphan-unmerged should be kept")
	}
	if !branchExists(t, client, repoDir, "main") {
		t.Error("main should be kept")
	}
}

func TestPruneBranches_ClassicDryRunKeepsBranches(t *testing.T) {
	repoDir := setupOrphanBranchRepo(t)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{Branches: true, DryRun: true})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(result.Candidates) != 1 || result.Candidates[0].Branch != "orphan-merged" {
		t.Fatalf("candidates = %+v, want only orphan-merged", result.Candidates)
	}
	if !branchExists(t, client, repoDir, "orphan-merged") {
		t.Error("dry-run must not delete orphan-merged")
	}
}

func TestPrune_WithoutBranchesFlagIgnoresOrphans(t *testing.T) {
	repoDir := setupOrphanBranchRepo(t)

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %+v, want none", result.Candidates)
	}
	if !branchExists(t, client, repoDir, "orphan-merged") {
		t.Error("orphan-merged must survive prune without --branches")
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
	orig := ghRunFunc
	defer func() { ghRunFunc = orig }()
	ghRunFunc = func(string, ...string) ([]byte, error) { return data, nil }

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Prune(PruneOptions{Branches: true, PRAware: true})
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}

	entries := make(map[string]PruneEntry)
	for _, e := range result.PruneEntries {
		entries[e.Branch] = e
	}
	if e := entries["orphan-unmerged"]; e.Action != "removed" || e.Name != "" {
		t.Errorf("orphan-unmerged entry = %+v, want removed with empty name", e)
	}
	if e := entries["orphan-merged"]; e.Action != "skipped" || e.Reason != "no PR found" {
		t.Errorf("orphan-merged entry = %+v, want skipped (no PR found)", e)
	}
	if e := entries["feat-wt"]; e.Action != "skipped" || e.Name != "feat-wt" {
		t.Errorf("feat-wt entry = %+v, want skipped with worktree name", e)
	}
	if branchExists(t, client, repoDir, "orphan-unmerged") {
		t.Error("orphan-unmerged should have been deleted via PR MERGED")
	}
	if !branchExists(t, client, repoDir, "orphan-merged") {
		t.Error("orphan-merged has no PR and must be kept in pr-aware mode")
	}
}
