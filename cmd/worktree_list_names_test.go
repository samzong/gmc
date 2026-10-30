package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeListGlobalNames(t *testing.T) {
	for _, name := range []string{"unique", "sibling-collision", "alias-collision"} {
		t.Run(name, func(t *testing.T) {
			repo, _, client := cmdRootClient(t, true)
			created, err := client.Add("feature", worktree.AddOptions{})
			require.NoError(t, err)
			want := filepath.Base(created.Path)
			if name == "sibling-collision" {
				runGitCmd(t, repo, "worktree", "add", "-b", "sibling", filepath.Join(filepath.Dir(repo), want))
				want = created.Path
			}
			if name == "alias-collision" {
				_, err := client.Add(want, worktree.AddOptions{})
				require.NoError(t, err)
				want = created.Path
			}
			for _, format := range []string{"text", "json"} {
				t.Run(format, func(t *testing.T) {
					setTestValue(t, &outputFlag.value, format)
					var out, warnings bytes.Buffer
					withWriters(t, &out, &warnings)
					require.NoError(t, runWorktreeList(client, false))
					assert.Empty(t, warnings.String())
					if format == "text" {
						assert.Contains(t, out.String(), want+" ")
						if name == "unique" {
							assert.NotContains(t, out.String(), created.Path)
						}
						return
					}
					var items []WorktreeJSON
					require.NoError(t, json.Unmarshal(out.Bytes(), &items))
					found := false
					for _, item := range items {
						if item.Path != created.Path {
							continue
						}
						found = true
						assert.Equal(t, want, item.Name)
						report, err := client.Remove(item.Name, worktree.RemoveOptions{DryRun: true})
						require.NoError(t, err)
						assert.Contains(t, report.Warnings(), "Would remove worktree: "+item.Path)
					}
					assert.True(t, found)
				})
			}
		})
	}
}

func TestWorktreeListPreservesMixedStatuses(t *testing.T) {
	repo, _, client := cmdRootClient(t, true)
	global, err := client.Add("global", worktree.AddOptions{})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(global.Path, "untracked"), []byte("data"), 0o600))
	agent := filepath.Join(repo, ".claude", "worktrees", "agent")
	externalRoot, err := os.MkdirTemp("", "gmc-external-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(externalRoot)) })
	externalRoot, err = filepath.EvalSymlinks(externalRoot)
	require.NoError(t, err)
	external := filepath.Join(externalRoot, "external")
	missingRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	missing := filepath.Join(missingRoot, "missing")
	for branch, path := range map[string]string{"agent": agent, "external": external, "missing": missing} {
		runGitCmd(t, repo, "worktree", "add", "-b", branch, path)
	}
	require.NoError(t, os.RemoveAll(missing))
	listed, err := client.List()
	require.NoError(t, err)
	listed = filterBareWorktrees(listed)
	wantStatuses := make([]string, len(listed))
	for i, wt := range listed {
		wantStatuses[i] = resolveWorktreeStatus(client, getDisplayRoot(client), wt)
	}
	trace := filepath.Join(t.TempDir(), "trace.json")
	t.Setenv("GIT_TRACE2_EVENT", trace)
	setTestValue(t, &outputFlag.value, "json")
	var out, warnings bytes.Buffer
	withWriters(t, &out, &warnings)
	require.NoError(t, runWorktreeList(client, false))
	traceData, err := os.ReadFile(trace)
	require.NoError(t, err)
	statusCalls := make(map[string]int)
	for _, line := range bytes.Split(traceData, []byte("\n")) {
		var event struct {
			Argv []string `json:"argv"`
		}
		if json.Unmarshal(line, &event) == nil && len(event.Argv) >= 5 && event.Argv[3] == "status" {
			statusCalls[event.Argv[2]]++
		}
	}
	assert.Equal(t, map[string]int{repo: 1, global.Path: 1}, statusCalls)

	var items []WorktreeJSON
	require.NoError(t, json.Unmarshal(out.Bytes(), &items))
	require.Len(t, items, len(listed))
	for i, item := range items {
		assert.Equal(t, listed[i].Path, item.Path)
		assert.Equal(t, wantStatuses[i], item.Status)
		switch item.Path {
		case agent, external:
			assert.Equal(t, "agent", item.Status)
		case missing:
			assert.Equal(t, "missing", item.Status)
		case global.Path:
			assert.Equal(t, "1 untracked", item.Status)
		}
	}
}
