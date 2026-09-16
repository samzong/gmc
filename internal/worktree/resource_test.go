package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncSharedResources(t *testing.T) {
	tempDir := physicalTempDir(t)
	err := os.Mkdir(filepath.Join(tempDir, ".bare"), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tempDir, ".env"), []byte("SECRET=123"), 0644)
	require.NoError(t, err)

	err = os.Mkdir(filepath.Join(tempDir, "models"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "models", "base.bin"), []byte("model-data"), 0644)
	require.NoError(t, err)

	sharedConfig := `
shared:
  - path: .env
    strategy: copy
  - path: models
    strategy: link
`
	err = os.WriteFile(filepath.Join(tempDir, ".gmc-shared.yml"), []byte(sharedConfig), 0644)
	require.NoError(t, err)

	wtName := "test-worktree"
	wtPath := filepath.Join(tempDir, wtName)
	err = os.Mkdir(wtPath, 0755)
	require.NoError(t, err)

	client := NewClient(Options{Verbose: true})

	t.Chdir(tempDir)

	_, err = client.syncSharedResourcesToPath(wtPath, true)
	require.NoError(t, err)

	envContent, err := os.ReadFile(filepath.Join(wtPath, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET=123", string(envContent))

	linkInfo, err := os.Lstat(filepath.Join(wtPath, "models"))
	require.NoError(t, err)
	assert.True(t, linkInfo.Mode()&os.ModeSymlink != 0, "models should be a symlink")

	target, err := os.Readlink(filepath.Join(wtPath, "models"))
	require.NoError(t, err)

	assert.Equal(t, "../models", target)

	modelContent, err := os.ReadFile(filepath.Join(wtPath, "models", "base.bin"))
	require.NoError(t, err)
	assert.Equal(t, "model-data", string(modelContent))
}
