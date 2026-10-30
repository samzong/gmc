package cmd

import (
	"bytes"
	"encoding/json"
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
