package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/exitcode"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/samzong/gmc/internal/workflow"
	"github.com/samzong/gmc/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicRootFlags(t *testing.T) {
	for _, tt := range []struct {
		name, shorthand, kind, value string
		persistent                   bool
	}{
		{"config", "", "string", "", true}, {"debug", "", "bool", "false", true},
		{"output", "o", "string", "text", true}, {"version", "V", "bool", "false", false},
		{"no-verify", "", "bool", "false", false}, {"no-signoff", "", "bool", "false", false},
		{"dry-run", "", "bool", "false", false}, {"all", "a", "bool", "false", false},
		{"issue", "", "string", "", false}, {"yes", "y", "bool", "false", false},
		{"verbose", "v", "bool", "false", false}, {"branch", "b", "string", "", false},
		{"prompt", "p", "string", "", false}, {"timeout", "", "int", "30", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flags := RootCmd().Flags()
			if tt.persistent {
				flags = RootCmd().PersistentFlags()
			}
			flag := flags.Lookup(tt.name)
			require.NotNil(t, flag)
			assert.Equal(t, tt.shorthand, flag.Shorthand)
			assert.Equal(t, tt.kind, flag.Value.Type())
			assert.Equal(t, tt.value, flag.DefValue)
			assert.False(t, flag.Hidden)
		})
	}
}

func TestPublicWorktreeCommands(t *testing.T) {
	for _, path := range []string{"wt", "worktree", "wt add", "wt list", "wt ls", "wt remove", "wt rm",
		"wt clone", "wt dup", "wt promote", "wt prune", "wt pr-review", "wt init", "wt switch", "wt sync",
		"wt hook", "wt hook add", "wt hook remove", "wt hook rm", "wt hook list", "wt hook ls",
		"wt share", "wt share ls", "wt share rm", "wt share add", "wt share remove", "wt share list", "wt share sync",
		"wt share discover"} {
		t.Run(path, func(t *testing.T) {
			command, remaining, err := RootCmd().Find(strings.Fields(path))
			require.NoError(t, err)
			require.Empty(t, remaining)
			require.NotNil(t, command.RunE)
			assert.False(t, command.Hidden)
			for _, name := range []string{"config", "debug", "output"} {
				found := false
				for parent := command; parent != nil; parent = parent.Parent() {
					found = found || parent.PersistentFlags().Lookup(name) != nil
				}
				require.True(t, found, name)
			}
		})
	}
}

func TestPublicExitCodes(t *testing.T) {
	for _, tt := range []struct {
		err  error
		code int
	}{
		{workflow.ErrNoChanges, 10}, {git.ErrNotGitRepo, 11}, {llm.ErrLLM, 12}, {workflow.ErrNoCommitSubject, 12},
	} {
		var coded *exitcode.Error
		require.ErrorAs(t, handleErrors(tt.err, false), &coded)
		assert.Equal(t, tt.code, coded.Code)
	}
	assert.NoError(t, handleErrors(nil, false))
	assert.Equal(t, 0, exitcode.Success)
	assert.Equal(t, 1, exitcode.General)
	var coded *exitcode.Error
	assert.False(t, errors.As(handleErrors(errors.New("failure"), false), &coded))
}

func TestPublicWorktreeJSON(t *testing.T) {
	zero := 0
	bytesAllocated := uint64(0)
	for _, tt := range []struct {
		name string
		item any
		want string
	}{
		{"list required", WorktreeJSON{Name: "main", Path: "/repo", Branch: "main", Commit: "abc", Status: "clean"},
			`{"name":"main","path":"/repo","branch":"main","commit":"abc","status":"clean"}`},
		{"list optional", WorktreeJSON{Name: "main", Path: "/repo", Branch: "main", Commit: "abc", Status: "missing",
			Repository: "/root", Locked: true, Prunable: true, DiffBase: "origin/main", ChangedFiles: &zero,
			Insertions: &zero, Deletions: &zero,
			ReviewProvider: "github", ReviewNumber: 42, ReviewState: "OPEN", ReviewURL: "https://example.test/42",
			AllocatedBytes: &bytesAllocated,
			Issues:         []WorktreeIssueJSON{{Source: "size", Code: "not_found", Message: "missing"}}},
			`{
  "name": "main",
  "path": "/repo",
  "branch": "main",
  "commit": "abc",
  "status": "missing",
  "repository": "/root",
  "locked": true,
  "prunable": true,
  "diff_base": "origin/main",
  "changed_files": 0,
  "insertions": 0,
  "deletions": 0,
  "review_provider": "github",
  "review_number": 42,
  "review_state": "OPEN",
  "review_url": "https://example.test/42",
  "allocated_bytes": 0,
  "issues": [
    {
      "source": "size",
      "code": "not_found",
      "message": "missing"
    }
  ]
}`},
		{"prune", PruneJSON{Repository: "/root", Name: "feature", Branch: "feature", Status: "merged",
			Action: "would-remove"},
			`{"repository":"/root","name":"feature","branch":"feature","status":"merged","action":"would-remove"}`},
		{"pr-aware prune required", worktree.PruneEntry{Name: "feature", Branch: "feature",
			PRState: "", Action: "skipped", Reason: "no PR found"},
			`{"name":"feature","branch":"feature","pr_state":"","action":"skipped","reason":"no PR found"}`},
		{"pr-aware prune optional", worktree.PruneEntry{Repository: "/root", Name: "feature", Branch: "feature",
			PRNum: 42, PRState: "MERGED", Action: "would_remove", Reason: "PR merged"},
			`{"repository":"/root","name":"feature","branch":"feature","pr_number":42,
			"pr_state":"MERGED","action":"would_remove","reason":"PR merged"}`},
		{"share", ShareJSON{Path: ".env", Strategy: "copy", Origin: "repo", Disabled: false},
			`{"path":".env","strategy":"copy","origin":"repo","disabled":false}`},
		{"hook required", HookJSON{Index: 1, Cmd: "true", Origin: "global"},
			`{"index":1,"cmd":"true","origin":"global","disabled":false}`},
		{"hook optional", HookJSON{Index: 1, ID: "prepare", Cmd: "true", Desc: "Prepare", Origin: "repo", Disabled: true},
			`{"index":1,"id":"prepare","cmd":"true","desc":"Prepare","origin":"repo","disabled":true}`},
		{"discover required", worktree.DiscoverResult{Path: "node_modules", Reason: "dependencies", Status: "candidate",
			SizeBytes: 0},
			`{"path":"node_modules","reason":"dependencies","status":"candidate","size_bytes":0}`},
		{"discover optional", worktree.DiscoverResult{Path: "node_modules", Strategy: "link", Reason: "dependencies",
			Source: "/repo/node_modules",
			Status: "configured", SizeBytes: 42, SizeLimited: true, Project: "web", Ecosystem: "node",
			PackageManager: "pnpm", ConfiguredBy: "repo", TargetState: "linked", ProjectMarker: true},
			`{
  "path": "node_modules",
  "strategy": "link",
  "reason": "dependencies",
  "source": "/repo/node_modules",
  "status": "configured",
  "size_bytes": 42,
  "size_limited": true,
  "project": "web",
  "ecosystem": "node",
  "package_manager": "pnpm",
  "configured_by": "repo",
  "target_state": "linked"
}`},

		{"create success", WorktreeCreateJSON{Name: "feature", Path: "/repo--feature", Branch: "feature", Base: "main",
			Created: true, Warnings: []string{"warning"}},
			`{"name":"feature","path":"/repo--feature","branch":"feature","base":"main","created":true,"warnings":["warning"]}`},
		{"create failure", WorktreeCreateJSON{Name: "feature", Error: "failed"}, `{
  "name": "feature",
  "created": false,
  "error": "failed"
}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, printJSON(&out, []any{tt.item}))
			assert.JSONEq(t, "["+tt.want+"]", out.String())
		})
	}
}

func TestPublicWorktreeListEntry(t *testing.T) {
	repo, client, out := newWorktreeOutputTest(t)
	setTestValue(t, &outputFlag.value, "json")
	require.NoError(t, runWorktreeList(client, false))
	var items []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &items))
	require.Len(t, items, 1)
	for key, want := range map[string]string{"path": repo, "branch": "main", "status": "clean"} {
		var got string
		require.NoError(t, json.Unmarshal(items[0][key], &got))
		assert.Equal(t, want, got)
	}
	for _, key := range []string{"name", "commit"} {
		var got string
		require.NoError(t, json.Unmarshal(items[0][key], &got))
		assert.NotEmpty(t, got)
	}
	var size uint64
	require.NoError(t, json.Unmarshal(items[0]["allocated_bytes"], &size))
	assert.NotContains(t, out.String(), "\x1b")
}
