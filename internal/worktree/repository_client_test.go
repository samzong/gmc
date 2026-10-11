package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryClient_IgnoresRepositoryEnvironment(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(fmt.Sprintf("bare=%t", bare), func(t *testing.T) {
			root := initTestRepo(t)
			repo := root
			if bare {
				root = initBareLayoutRepo(t)
				repo = filepath.Join(root, ".bare")
			}
			feature := filepath.Join(root, "feature")
			runGit(t, repo, "worktree", "add", "-b", "feature", feature, "main")
			other := initTestRepo(t)
			otherFeature := filepath.Join(other, "other-feature")
			runGit(t, other, "worktree", "add", "-b", "other-feature", otherFeature, "main")
			t.Chdir(other)
			t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
			t.Setenv("GIT_WORK_TREE", other)
			t.Setenv("GIT_COMMON_DIR", filepath.Join(other, ".git"))
			t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
			t.Setenv("GIT_CONFIG_COUNT", "1")
			t.Setenv("GIT_CONFIG_KEY_0", "core.bare")
			t.Setenv("GIT_CONFIG_VALUE_0", "true")
			client, err := NewRepositoryClient(root, Options{Dir: other})
			require.NoError(t, err)
			resolved, err := client.GetWorktreeRoot()
			require.NoError(t, err)
			assert.Equal(t, root, resolved)
			result, err := client.Prune(PruneOptions{})
			require.NoError(t, err)
			require.Len(t, result.Candidates, 1)
			assert.Equal(t, "feature", result.Candidates[0].Branch)
			assertMissing(t, feature)
			assert.DirExists(t, otherFeature)
			actualCwd, err := os.Getwd()
			require.NoError(t, err)
			assert.Equal(t, other, actualCwd)
		})
	}
}

func TestRepositoryClient_RejectsMisresolvedNestedRepository(t *testing.T) {
	outer := initBareLayoutRepo(t)
	inner := filepath.Join(outer, "inner")
	require.NoError(t, os.Rename(initTestRepo(t), inner))
	feature := filepath.Join(outer, "inner-feature")
	runGit(t, inner, "worktree", "add", "-b", "inner-feature", feature, "main")
	client, err := NewRepositoryClient(inner, Options{})
	require.Error(t, err)
	assert.Nil(t, client)
	assert.DirExists(t, feature)
}

func TestRepositoryClient_AcceptsSymlinkRepository(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(fmt.Sprintf("bare=%t", bare), func(t *testing.T) {
			root := initTestRepo(t)
			if bare {
				root = initBareLayoutRepo(t)
			}
			alias := filepath.Join(physicalTempDir(t), "repository")
			require.NoError(t, os.Symlink(root, alias))
			client, err := NewRepositoryClient(alias, Options{})
			require.NoError(t, err)
			result, err := client.Prune(PruneOptions{DryRun: true})
			require.NoError(t, err)
			assert.Empty(t, result.Candidates)
		})
	}
}

func TestRepositoryClient_PreservesDefaultClientEnvironment(t *testing.T) {
	root := initTestRepo(t)
	other := initTestRepo(t)
	t.Chdir(root)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	commonDir, err := NewClient(Options{}).GetGitCommonDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(other, ".git"), commonDir)
}

func TestRepositoryClient_IsolatesGhRepositoryEnvironment(t *testing.T) {
	root := initTestRepo(t)
	feature := filepath.Join(root, "feature")
	runGit(t, root, "worktree", "add", "-b", "feature", feature, "main")
	bin := physicalTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(`#!/bin/sh
if [ "${GH_REPO+x}" = x ]; then
  exit 9
fi
if [ "$GH_HOST" != "github.example" ]; then
  exit 10
fi
printf '[{"number":42,"state":"MERGED","headRefName":"feature"}]'
`), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_REPO", "owner/other")
	t.Setenv("GH_HOST", "github.example")
	t.Chdir(root)
	client, err := NewRepositoryClient(root, Options{})
	require.NoError(t, err)
	result, err := client.Prune(PruneOptions{PRAware: true, DryRun: true})
	require.NoError(t, err)
	require.Len(t, result.PruneEntries, 1)
	assert.Equal(t, "feature", result.PruneEntries[0].Branch)
	assert.Equal(t, "would_remove", result.PruneEntries[0].Action)
	_, err = NewClient(Options{}).Prune(PruneOptions{PRAware: true, DryRun: true})
	require.Error(t, err)
	assert.DirExists(t, feature)
}
