package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromoteRejectsSameWorktreeCandidate(t *testing.T) {
	repoDir := initTestRepo(t)
	t.Chdir(repoDir)

	client := NewClient(Options{})
	_, err := client.Promote(repoDir, PromoteOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be different from the parent")
}

func TestDupCopiesTaskFilesAndKeepsBareLayoutPath(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	t.Chdir(mainDir)

	writeFile(t, filepath.Join(mainDir, "todo.md"), "task")

	client := NewClient(Options{})
	result, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1, TaskFiles: []string{"todo.md"}})
	require.NoError(t, err)
	require.Equal(t, ".dup-1", result.Worktrees[0])
	dupDir := filepath.Join(repoDir, ".dup-1")
	require.True(t, sameCleanPath(result.WorktreePaths[0], dupDir))
	require.Equal(t, "task", readFile(t, filepath.Join(dupDir, "todo.md")))
}

func TestDupWithoutTaskWorksFromBareLayoutRoot(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	t.Chdir(repoDir)

	client := NewClient(Options{})
	result, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1})
	require.NoError(t, err)
	dupDir := filepath.Join(repoDir, ".dup-1")
	require.True(t, sameCleanPath(result.WorktreePaths[0], dupDir))
	require.Equal(t, ".dup-1", result.RelativePaths[0])
}

func TestDupDefaultsToCurrentWorktreeBranchAndSiblingPath(t *testing.T) {
	repoDir := initTestRepo(t)
	featureDir := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+"--feature-current")
	runGit(t, repoDir, "worktree", "add", "-b", "feature/current", featureDir, "main")
	var err error
	featureDir, err = filepath.EvalSymlinks(featureDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(featureDir) })

	writeFile(t, filepath.Join(featureDir, "feature.txt"), "feature")
	commitFiles(t, featureDir, "feature", "feature.txt")
	t.Chdir(featureDir)

	client := NewClient(Options{})
	result, err := client.Dup(DupOptions{Count: 1})
	require.NoError(t, err)

	dupDir := filepath.Join(filepath.Dir(featureDir), ".dup-1")
	t.Cleanup(func() { _ = os.RemoveAll(dupDir) })
	require.Equal(t, "feature/current", result.BaseBranch)
	require.True(t, sameCleanPath(result.WorktreePaths[0], dupDir))
	require.Equal(t, "../.dup-1", result.RelativePaths[0])
	require.Equal(t, "feature", readFile(t, filepath.Join(dupDir, "feature.txt")))
}

func TestDupRejectsTaskDirectory(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	t.Chdir(mainDir)
	require.NoError(t, os.Mkdir(filepath.Join(mainDir, "tasks"), 0o755))

	client := NewClient(Options{})
	_, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1, TaskFiles: []string{"tasks"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be a file")
}

func TestDupAcceptsCanonicalTaskPath(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	writeFile(t, filepath.Join(mainDir, "todo.md"), "task")

	linkDir := filepath.Join(t.TempDir(), "main-link")
	require.NoError(t, os.Symlink(mainDir, linkDir))
	t.Chdir(mainDir)

	client := NewClient(Options{})
	_, err := client.Dup(DupOptions{
		BaseBranch: "main",
		Count:      1,
		TaskFiles:  []string{filepath.Join(linkDir, "todo.md")},
	})
	require.NoError(t, err)
	assertFileContent(t, filepath.Join(repoDir, ".dup-1", "todo.md"), "task")
}

func TestPromoteAppliesCandidateChangesToCurrentParent(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)

	writeFile(t, filepath.Join(dupDir, "committed.txt"), "committed")
	commitFiles(t, dupDir, "candidate commit", "committed.txt")
	writeFile(t, filepath.Join(dupDir, "staged.txt"), "staged")
	runGit(t, dupDir, "add", "staged.txt")
	writeFile(t, filepath.Join(dupDir, "README.md"), "candidate readme")
	writeFile(t, filepath.Join(dupDir, "untracked.txt"), "untracked")
	t.Chdir(mainDir)

	client := NewClient(Options{})
	report, err := client.Promote(".dup-1", PromoteOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, report.Events)

	assertFileContent(t, filepath.Join(mainDir, "committed.txt"), "committed")
	assertFileContent(t, filepath.Join(mainDir, "staged.txt"), "staged")
	assertFileContent(t, filepath.Join(mainDir, "README.md"), "candidate readme")
	assertFileContent(t, filepath.Join(mainDir, "untracked.txt"), "untracked")
	status := runGit(t, mainDir, "status", "--short")
	for _, file := range []string{"committed.txt", "staged.txt", "README.md", "untracked.txt"} {
		require.Contains(t, status, file)
	}
}

func TestPromoteDryRunLeavesParentUnchanged(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	commitFiles(t, dupDir, "candidate", "candidate.txt")
	t.Chdir(mainDir)

	client := NewClient(Options{})
	_, err := client.Promote(".dup-1", PromoteOptions{DryRun: true})
	require.NoError(t, err)
	assertMissing(t, filepath.Join(mainDir, "candidate.txt"))
}

func TestPromoteRejectsDirtyParent(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	commitFiles(t, dupDir, "candidate", "candidate.txt")
	writeFile(t, filepath.Join(mainDir, "local.txt"), "local")
	t.Chdir(mainDir)

	client := NewClient(Options{})
	_, err := client.Promote(".dup-1", PromoteOptions{})
	require.ErrorContains(t, err, "clean it before promoting")
	assertMissing(t, filepath.Join(mainDir, "candidate.txt"))
	assertFileContent(t, filepath.Join(mainDir, "local.txt"), "local")
}

func TestPromoteSkipsUnchangedCopiedTaskFile(t *testing.T) {
	repoDir := initBareLayoutRepo(t)
	mainDir := filepath.Join(repoDir, "main")
	t.Chdir(mainDir)

	writeFile(t, filepath.Join(mainDir, "task.txt"), "task")

	client := NewClient(Options{})
	_, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1, TaskFiles: []string{"task.txt"}})
	require.NoError(t, err)

	dupDir := filepath.Join(repoDir, ".dup-1")
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	commitFiles(t, dupDir, "candidate", "candidate.txt")

	report, err := client.Promote(".dup-1", PromoteOptions{})
	require.NoError(t, err)

	assertFileContent(t, filepath.Join(mainDir, "task.txt"), "task")
	assertFileContent(t, filepath.Join(mainDir, "candidate.txt"), "candidate")
	for _, event := range report.Events {
		require.NotContains(t, event.Message, "task.txt")
	}
}

func TestPromoteWorksInNormalRepositoryWithNestedCandidate(t *testing.T) {
	repoDir := initTestRepo(t)
	t.Chdir(repoDir)

	client := NewClient(Options{})
	_, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1})
	require.NoError(t, err)

	dupDir := filepath.Join(filepath.Dir(repoDir), ".dup-1")
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	commitFiles(t, dupDir, "candidate", "candidate.txt")

	_, err = client.Promote(".dup-1", PromoteOptions{})
	require.NoError(t, err)
	assertFileContent(t, filepath.Join(repoDir, "candidate.txt"), "candidate")
}

func TestPromoteAppliesOntoAdvancedParent(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "candidate.txt"), "candidate")
	commitFiles(t, dupDir, "candidate", "candidate.txt")
	writeFile(t, filepath.Join(mainDir, "parent.txt"), "parent")
	commitFiles(t, mainDir, "parent", "parent.txt")
	t.Chdir(mainDir)

	client := NewClient(Options{})
	_, err := client.Promote(".dup-1", PromoteOptions{})
	require.NoError(t, err)
	assertFileContent(t, filepath.Join(mainDir, "candidate.txt"), "candidate")
	assertFileContent(t, filepath.Join(mainDir, "parent.txt"), "parent")
}

func TestPromoteTreatsAlreadyAppliedCandidateChangeAsNoop(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "same.txt"), "same")
	writeFile(t, filepath.Join(dupDir, "new.txt"), "new")
	commitFiles(t, dupDir, "candidate", "same.txt", "new.txt")

	writeFile(t, filepath.Join(mainDir, "same.txt"), "same")
	commitFiles(t, mainDir, "parent already has same", "same.txt")
	t.Chdir(mainDir)

	client := NewClient(Options{})
	_, err := client.Promote(".dup-1", PromoteOptions{})
	require.NoError(t, err)
	assertFileContent(t, filepath.Join(mainDir, "same.txt"), "same")
	assertFileContent(t, filepath.Join(mainDir, "new.txt"), "new")
}

func TestPromotePreservesUntrackedSymlink(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "target.txt"), "target")
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dupDir, "link.txt")))
	t.Chdir(mainDir)

	client := NewClient(Options{})
	_, err := client.Promote(".dup-1", PromoteOptions{})
	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(mainDir, "link.txt"))
	require.NoError(t, err)
	require.Equal(t, "target.txt", target)
	assertFileContent(t, filepath.Join(mainDir, "target.txt"), "target")
}

func TestPromoteConflictLeavesParentUnchanged(t *testing.T) {
	_, mainDir, dupDir := createPromoteCandidate(t)
	writeFile(t, filepath.Join(dupDir, "README.md"), "candidate")
	commitFiles(t, dupDir, "candidate readme", "README.md")
	writeFile(t, filepath.Join(mainDir, "README.md"), "parent")
	commitFiles(t, mainDir, "parent readme", "README.md")
	t.Chdir(mainDir)

	client := NewClient(Options{})
	_, err := client.Promote(".dup-1", PromoteOptions{})
	require.Error(t, err)
	assertFileContent(t, filepath.Join(mainDir, "README.md"), "parent")
}

func TestDupConfiguresBareLayoutWorktreeConfig(t *testing.T) {
	repoDir := initBareLayoutRepoWithWorktreeConfig(t)
	mainDir := filepath.Join(repoDir, "main")

	t.Chdir(mainDir)

	client := NewClient(Options{})
	result, err := client.Dup(DupOptions{BaseBranch: "main", Count: 1})
	require.NoError(t, err)
	require.Len(t, result.Worktrees, 1)

	dupDir := filepath.Join(repoDir, result.Worktrees[0])
	status := runGit(t, dupDir, "status", "--short", "--branch")
	require.Contains(t, status, "## "+result.Branches[0])

	got := strings.TrimSpace(runGit(t, dupDir, "config", "--worktree", "--bool", "core.bare"))
	require.Equal(t, "false", got)
}
