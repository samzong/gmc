package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncSharedResources(t *testing.T) {
	// Create a temp directory for our "repo"
	tempDir, err := os.MkdirTemp("", "gmc-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create .bare directory to simulate bare root
	err = os.Mkdir(filepath.Join(tempDir, ".bare"), 0755)
	require.NoError(t, err)

	// Create a source file and directory
	err = os.WriteFile(filepath.Join(tempDir, ".env"), []byte("SECRET=123"), 0644)
	require.NoError(t, err)

	err = os.Mkdir(filepath.Join(tempDir, "models"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "models", "base.bin"), []byte("model-data"), 0644)
	require.NoError(t, err)

	// Create .gmc-shared.yml
	sharedConfig := `
shared:
  - path: .env
    strategy: copy
  - path: models
    strategy: link
`
	err = os.WriteFile(filepath.Join(tempDir, ".gmc-shared.yml"), []byte(sharedConfig), 0644)
	require.NoError(t, err)

	// Create a mock worktree directory
	wtName := "test-worktree"
	wtPath := filepath.Join(tempDir, wtName)
	err = os.Mkdir(wtPath, 0755)
	require.NoError(t, err)

	// Initialize client
	client := NewClient(Options{Verbose: true})

	// We need to change CWD to tempDir so GetWorktreeRoot finds it
	oldCwd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer func() { _ = os.Chdir(oldCwd) }()

	// Run sync
	_, err = client.SyncSharedResources(wtName)
	require.NoError(t, err)

	// Verify .env (copy)
	envContent, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET=123", string(envContent))

	// Verify models (link)
	linkInfo, err := os.Lstat(filepath.Join(wtPath, "models"))
	require.NoError(t, err)
	assert.True(t, linkInfo.Mode()&os.ModeSymlink != 0, "models should be a symlink")

	target, err := os.Readlink(filepath.Join(wtPath, "models"))
	require.NoError(t, err)
	// On Darwin/Linux it should be relative
	assert.Equal(t, "../models", target)

	modelContent, err := os.ReadFile(filepath.Join(wtPath, "models", "base.bin"))
	require.NoError(t, err)
	assert.Equal(t, "model-data", string(modelContent))
}
