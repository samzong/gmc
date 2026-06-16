package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadNodeHandoffMissing(t *testing.T) {
	dir := t.TempDir()
	content, ok, err := ReadNodeHandoff(dir, "plan")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, content)
}

func TestReadNodeHandoffPresent(t *testing.T) {
	dir := t.TempDir()
	handoffDir := filepath.Join(dir, ".gmc", "workflow")
	require.NoError(t, os.MkdirAll(handoffDir, 0o755))
	path := filepath.Join(handoffDir, "plan.md")
	require.NoError(t, os.WriteFile(path, []byte("plan handoff"), 0o644))

	content, ok, err := ReadNodeHandoff(dir, "plan")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "plan handoff", content)
}

func TestReadNodeHandoffRequiresPaths(t *testing.T) {
	content, ok, err := ReadNodeHandoff("", "plan")
	require.Error(t, err)
	assert.Empty(t, content)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "worktree path")

	content, ok, err = ReadNodeHandoff(t.TempDir(), "")
	require.Error(t, err)
	assert.Empty(t, content)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "node id")
}
