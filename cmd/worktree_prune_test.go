package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pruneAllFixture(t *testing.T) (string, []string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repositories := []string{}
	for _, name := range []string{"repo-a", "repo-b"} {
		repo := filepath.Join(home, name)
		require.NoError(t, os.Rename(initCmdTestRepo(t), repo))
		runGitCmd(t, repo, "worktree", "add", "-b", name, filepath.Join(repo, "feature"), "main")
		runGitCmd(t, repo, "branch", "orphan", "main")
		response := fmt.Sprintf(`[{"number":42,"state":"MERGED","headRefName":%q},`+
			`{"number":43,"state":"MERGED","headRefName":"orphan"}]`, name)
		require.NoError(t, os.WriteFile(filepath.Join(repo, ".git", "test-prs.json"), []byte(response), 0o600))
		repositories = append(repositories, repo)
	}
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(`#!/bin/sh
if [ "$PWD" = "$FAIL_REPO" ]; then
  exit 1
fi
cat .git/test-prs.json
`), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", home)
	t.Setenv("FAIL_REPO", "")
	t.Chdir(t.TempDir())
	return home, repositories
}

func executePrune(t *testing.T, format string, args ...string) (string, string, error) {
	t.Helper()
	var out, warnings bytes.Buffer
	withWriters(t, &out, &warnings)
	setTestValue(t, &outputFlag.value, format)
	cmd := newWorktreePruneCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&warnings)
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	return out.String(), warnings.String(), err
}

func TestWorktreePruneAll_JSON(t *testing.T) {
	for _, prAware := range []bool{false, true} {
		t.Run(fmt.Sprintf("pr-aware=%t", prAware), func(t *testing.T) {
			_, repositories := pruneAllFixture(t)
			args := []string{"--all", "--dry-run", "--branches"}
			if prAware {
				args = append(args, "--pr-aware")
			}
			out, warnings, err := executePrune(t, "json", args...)
			require.NoError(t, err)
			assert.NotContains(t, warnings, "cannot prune")
			var entries []map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &entries))
			require.Len(t, entries, 4)
			for i, repo := range repositories {
				assert.Equal(t, repo, entries[2*i]["repository"])
				assert.Equal(t, filepath.Base(repo), entries[2*i]["branch"])
				assert.Equal(t, repo, entries[2*i+1]["repository"])
				assert.Equal(t, "orphan", entries[2*i+1]["branch"])
				action := "would-remove"
				if prAware {
					action = "would_remove"
					assert.Equal(t, float64(42), entries[2*i]["pr_number"])
				}
				assert.Equal(t, action, entries[2*i]["action"])
				assert.DirExists(t, filepath.Join(repo, "feature"))
			}
		})
	}
}

func TestWorktreePruneAll_ContinuesAfterGhFailure(t *testing.T) {
	_, repositories := pruneAllFixture(t)
	t.Setenv("FAIL_REPO", repositories[0])
	out, warnings, err := executePrune(t, "json", "--all", "--pr-aware")
	require.NoError(t, err)
	assert.Contains(t, warnings, "Warning: cannot prune ~/repo-a: gh pr list failed")
	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, repositories[1], entries[0]["repository"])
	assert.Equal(t, "removed", entries[0]["action"])
	assert.DirExists(t, filepath.Join(repositories[0], "feature"))
	_, err = os.Stat(filepath.Join(repositories[1], "feature"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestWorktreePruneAll_ForceAndHumanGroups(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			_, repositories := pruneAllFixture(t)
			for _, repo := range repositories {
				require.NoError(t, os.WriteFile(filepath.Join(repo, "feature", "dirty"), []byte("keep"), 0o600))
			}
			args := []string{"--all", "--pr-aware", "--dry-run"}
			if force {
				args = append(args, "--force")
			}
			out, _, err := executePrune(t, "text", args...)
			require.NoError(t, err)
			assert.Contains(t, out, "~/repo-a\nNAME")
			assert.Contains(t, out, "\n\n~/repo-b\nNAME")
			if force {
				assert.Contains(t, out, "would_remove")
			} else {
				assert.NotContains(t, out, "would_remove")
				assert.Contains(t, out, "uncommitted changes")
			}
		})
	}
}

func TestWorktreePruneAll_RejectsBase(t *testing.T) {
	_, _, err := executePrune(t, "json", "--all", "--base", "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all base")
}

func TestWorktreePrune_SingleRepositoryJSONCompatibility(t *testing.T) {
	for _, prAware := range []bool{false, true} {
		t.Run(fmt.Sprintf("pr-aware=%t", prAware), func(t *testing.T) {
			_, repositories := pruneAllFixture(t)
			t.Chdir(repositories[0])
			args := []string{"--dry-run"}
			if prAware {
				args = append(args, "--pr-aware")
			}
			out, _, err := executePrune(t, "json", args...)
			require.NoError(t, err)
			var entries []map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &entries))
			require.Len(t, entries, 1)
			assert.NotContains(t, entries[0], "repository")
			assert.Equal(t, "repo-a", entries[0]["branch"])
		})
	}
}

func TestWorktreePruneAll_ScanFailureAndEmptyResult(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			home := t.TempDir()
			if missing {
				home = filepath.Join(home, "missing")
			}
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			out, _, err := executePrune(t, "json", "--all", "--dry-run", "--pr-aware")
			if missing {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.JSONEq(t, `[]`, out)
			}
		})
	}
}

func TestWorktreePruneAll_SkipsMisresolvedNestedRepository(t *testing.T) {
	home, repositories := pruneAllFixture(t)
	outer := filepath.Join(home, "outer")
	bare := filepath.Join(outer, ".bare")
	runGitCmd(t, home, "clone", "--bare", repositories[0], bare)
	runGitCmd(t, bare, "worktree", "add", filepath.Join(outer, "main"), "main")
	outerFeature := filepath.Join(outer, "outer-feature")
	runGitCmd(t, bare, "worktree", "add", "-b", "outer-feature", outerFeature, "main")
	inner := filepath.Join(outer, "inner")
	require.NoError(t, os.Rename(initCmdTestRepo(t), inner))
	innerFeature := filepath.Join(outer, "inner-feature")
	runGitCmd(t, inner, "worktree", "add", "-b", "inner-feature", innerFeature, "main")

	for _, dryRun := range []bool{true, false} {
		args := []string{"--all"}
		if dryRun {
			args = append(args, "--dry-run")
		}
		out, warnings, err := executePrune(t, "json", args...)
		require.NoError(t, err)
		assert.Contains(t, warnings, "Warning: cannot prune ~/outer/inner:")
		var entries []map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &entries))
		require.Len(t, entries, 3)
		assert.Equal(t, outer, entries[0]["repository"])
		assert.Equal(t, "outer-feature", entries[0]["branch"])
		for _, entry := range entries {
			assert.NotEqual(t, inner, entry["repository"])
		}
		assert.DirExists(t, innerFeature)
		if dryRun {
			assert.DirExists(t, outerFeature)
		} else {
			_, err = os.Stat(outerFeature)
			assert.ErrorIs(t, err, os.ErrNotExist)
		}
	}
}
