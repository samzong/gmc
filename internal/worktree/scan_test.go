package worktree

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanFindsNestedAndLinkedWorktrees(t *testing.T) {
	home := initTestRepo(t)
	linked := filepath.Join(home, ".codex", "worktrees", "feature with space")
	runGit(t, home, "worktree", "add", "-b", "feature", linked)
	runGit(t, home, "worktree", "lock", "--reason", "in use", linked)
	writeFile(t, filepath.Join(linked, "README.md"), "changed")
	writeFile(t, filepath.Join(linked, "untracked.txt"), "new")

	nested := filepath.Join(home, ".local", "nested")
	runGit(t, home, "clone", home, nested)
	nestedLinked := filepath.Join(home, ".local", "nested--feature")
	runGit(t, nested, "worktree", "add", "-b", "feature", nestedLinked)

	bare := filepath.Join(home, "managed", ".bare")
	runGit(t, home, "clone", "--bare", home, bare)
	bareLinked := filepath.Join(home, "managed", "main")
	runGit(t, bare, "worktree", "add", bareLinked, "main")
	runGit(t, home, "clone", "--bare", home, filepath.Join(home, "cache", "repo.git"))

	missing := filepath.Join(home, "missing")
	runGit(t, home, "worktree", "add", "--detach", missing)
	require.NoError(t, os.RemoveAll(missing))
	runGit(t, home, "clone", home, filepath.Join(home, ".git", "hidden-repo"))
	external := initTestRepo(t)
	monitor := filepath.Join(external, "monitor")
	require.NoError(t, os.WriteFile(monitor, []byte("#!/bin/sh\ntouch \"$0.called\"\n"), 0o755))
	runGit(t, home, "config", "core.fsmonitor", monitor)
	require.NoError(t, os.Symlink(external, filepath.Join(home, "symlink")))
	require.NoError(t, os.Symlink(home, filepath.Join(home, "cycle")))
	t.Chdir(external)
	t.Setenv("GIT_DIR", filepath.Join(external, ".git"))
	t.Setenv("GIT_WORK_TREE", external)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(external, ".git", "index"))

	result, err := Scan(home)
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	byPath := make(map[string]Info)
	for _, wt := range result.Worktrees {
		_, duplicate := byPath[wt.Path]
		assert.False(t, duplicate, wt.Path)
		byPath[wt.Path] = wt
	}
	require.Len(t, byPath, 6)
	for _, path := range []string{home, linked, nested, nestedLinked, bareLinked, missing} {
		assert.Contains(t, byPath, path)
	}
	assert.Equal(t, home, byPath[linked].Repository)
	assert.True(t, byPath[linked].IsLocked)
	assert.Equal(t, "1 file changed, 1 untracked", byPath[linked].Status)
	assert.Equal(t, nested, byPath[nestedLinked].Repository)
	assert.Equal(t, filepath.Dir(bare), byPath[bareLinked].Repository)
	assert.True(t, byPath[missing].IsPrunable)
	assert.Equal(t, "missing", byPath[missing].Status)
	assertMissing(t, monitor+".called")
}

func TestScanContinuesPastUnreadableDirectories(t *testing.T) {
	home := initTestRepo(t)
	runGit(t, home, "worktree", "add", "--detach", filepath.Join(home, "linked"))
	unreadable := filepath.Join(home, ".hidden")
	require.NoError(t, os.Mkdir(unreadable, 0o700))
	require.NoError(t, os.Chmod(unreadable, 0))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o700) })
	if _, err := os.ReadDir(unreadable); err == nil {
		t.Skip("directory permissions are not enforced")
	}
	result, err := Scan(home)
	require.NoError(t, err)
	require.Len(t, result.Worktrees, 2)
	require.Len(t, result.Warnings, 1)
	assert.ErrorContains(t, result.Warnings[0], unreadable)
	_, err = Scan(unreadable)
	require.Error(t, err)
}

func TestScanSubmoduleUsesCheckoutPath(t *testing.T) {
	home := initTestRepo(t)
	source := initTestRepo(t)
	submodule := filepath.Join(home, "vendor", "module")
	runGit(t, home, "-c", "protocol.file.allow=always", "submodule", "add", source, submodule)
	linked := filepath.Join(home, ".local", "module-linked")
	runGit(t, submodule, "worktree", "add", "--detach", linked)
	result, err := Scan(home)
	require.NoError(t, err)
	require.Len(t, result.Worktrees, 2)
	assert.Equal(t, submodule, result.Worktrees[0].Path)
	assert.Equal(t, submodule, result.Worktrees[0].Repository)
	assert.Equal(t, linked, result.Worktrees[1].Path)
	t.Chdir(submodule)
	listed, err := NewClient(Options{}).List()
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, submodule, listed[0].Path)
}

func TestScanPrunesDependencyAndSystemDirectories(t *testing.T) {
	source := initTestRepo(t)
	root := physicalTempDir(t)
	family := func(parts ...string) string {
		repo := filepath.Join(append([]string{root}, parts...)...)
		runGit(t, root, "clone", source, repo)
		runGit(t, repo, "worktree", "add", "--detach", repo+"-linked")
		return repo
	}
	app := family("app")
	family("node_modules", "pkg")
	family("app", "node_modules", "dep")
	nestedLibrary := family("src", "Library", "lib")
	library := family("Library", "lib")
	trash := family(".Trash", "old")

	result, err := Scan(root)
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	repositories := make(map[string]int)
	for _, wt := range result.Worktrees {
		repositories[wt.Repository]++
	}
	want := map[string]int{app: 2, nestedLibrary: 2}
	if runtime.GOOS != "darwin" {
		want[library] = 2
		want[trash] = 2
	}
	assert.Equal(t, want, repositories)
}

func TestScanIgnoresRepositoryEnvironmentAndKeepsUserGitConfig(t *testing.T) {
	home := initTestRepo(t)
	linked := filepath.Join(home, "linked")
	runGit(t, home, "worktree", "add", "--detach", linked)
	writeFile(t, filepath.Join(linked, "untracked.txt"), "new")
	elsewhere := initBareRepo(t)
	globalConfig := filepath.Join(physicalTempDir(t), "gitconfig")
	writeFile(t, globalConfig, "[status]\n\tshowUntrackedFiles = no\n")
	t.Setenv("GIT_DIR", elsewhere)
	t.Setenv("GIT_COMMON_DIR", elsewhere)
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)

	result, err := Scan(home)
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	require.Len(t, result.Worktrees, 2)
	assert.Equal(t, home, result.Worktrees[0].Path)
	assert.Equal(t, home, result.Worktrees[0].Repository)
	assert.Equal(t, linked, result.Worktrees[1].Path)
	assert.Equal(t, "clean", result.Worktrees[1].Status)
}

func TestScanUsesCheckoutOfSeparateGitDir(t *testing.T) {
	home := physicalTempDir(t)
	checkout := filepath.Join(home, "work")
	gitDir := filepath.Join(home, "store", "work.git")
	require.NoError(t, os.Mkdir(filepath.Dir(gitDir), 0o755))
	runGit(t, home, "init", "-b", "main", "--separate-git-dir", gitDir, checkout)
	runGit(t, checkout, "config", "user.name", "Test User")
	runGit(t, checkout, "config", "user.email", "test@example.com")
	writeFile(t, filepath.Join(checkout, "README.md"), "init")
	commitFiles(t, checkout, "init", ".")
	linked := filepath.Join(home, "work-linked")
	runGit(t, checkout, "worktree", "add", "--detach", linked)
	writeFile(t, filepath.Join(checkout, "untracked.txt"), "new")

	result, err := Scan(home)
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	require.Len(t, result.Worktrees, 2)
	assert.Equal(t, checkout, result.Worktrees[0].Path)
	assert.Equal(t, checkout, result.Worktrees[0].Repository)
	assert.Equal(t, "1 untracked", result.Worktrees[0].Status)
	assert.Equal(t, linked, result.Worktrees[1].Path)
	assert.Equal(t, checkout, result.Worktrees[1].Repository)
}

func TestScanKeepsCoreWorktreeCheckout(t *testing.T) {
	home := physicalTempDir(t)
	meta := filepath.Join(home, "meta")
	checkout := filepath.Join(home, "q")
	require.NoError(t, os.Mkdir(checkout, 0o755))
	runGit(t, home, "init", "-b", "main", meta)
	metaGit := filepath.Join(meta, ".git")
	runGit(t, meta, "config", "core.worktree", checkout)
	runGit(t, meta, "config", "user.name", "Test User")
	runGit(t, meta, "config", "user.email", "test@example.com")
	writeFile(t, filepath.Join(checkout, "README.md"), "init")
	runGit(t, checkout, "--git-dir", metaGit, "add", ".")
	runGit(t, checkout, "--git-dir", metaGit, "commit", "-m", "init")
	linked := filepath.Join(home, "q-linked")
	runGit(t, checkout, "--git-dir", metaGit, "worktree", "add", "--detach", linked)

	result, err := Scan(home)
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	require.Len(t, result.Worktrees, 2)
	assert.Equal(t, checkout, result.Worktrees[0].Path)
	assert.Equal(t, checkout, result.Worktrees[0].Repository)
	assert.Equal(t, linked, result.Worktrees[1].Path)
}

func TestScanWarningIncludesGitReason(t *testing.T) {
	home := initTestRepo(t)
	runGit(t, home, "worktree", "add", "--detach", filepath.Join(home, "linked"))
	broken := filepath.Join(home, "broken")
	require.NoError(t, os.Mkdir(broken, 0o755))
	writeFile(t, filepath.Join(broken, ".git"), "gitdir: "+filepath.Join(home, "absent")+"\n")

	result, err := Scan(home)
	require.NoError(t, err)
	require.Len(t, result.Worktrees, 2)
	require.Len(t, result.Warnings, 1)
	assert.ErrorContains(t, result.Warnings[0], "cannot inspect repository "+broken)
	assert.ErrorContains(t, result.Warnings[0], "not a git repository")
}

func TestScanSkipsRepositoriesWithoutLinkedWorktrees(t *testing.T) {
	home := initTestRepo(t)
	writeFile(t, filepath.Join(home, "README.md"), "changed")
	for _, name := range []string{"existing", "missing"} {
		snapshot := filepath.Join(home, "snapshots", name)
		runGit(t, home, "init", "--bare", snapshot)
		runGit(t, snapshot, "config", "core.bare", "false")
		checkout := home
		if name == "missing" {
			checkout = filepath.Join(home, "no-checkout")
		}
		runGit(t, snapshot, "config", "core.worktree", checkout)
	}
	result, err := Scan(home)
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	require.Empty(t, result.Worktrees)
}
