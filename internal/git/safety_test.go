package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommitAllowsNonTempDirInTestEnv ensures Commit does not hardcode temp dir checks.
func TestCommitAllowsNonTempDirInTestEnv(t *testing.T) {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		t.Skip("User cache dir not available")
	}

	tempDir, err := os.MkdirTemp(cacheDir, "gmc_safe_repo_")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	// Ensure the directory wouldn't have matched previous hardcoded patterns.
	basePath := filepath.Clean(tempDir)
	if strings.Contains(basePath, "/tmp/") ||
		strings.Contains(basePath, "\\Temp\\") ||
		strings.Contains(basePath, "gmc_git_test") ||
		strings.Contains(basePath, "gmc_non_git_test") {
		t.Skipf("Temp dir %q matches previous safety patterns", basePath)
	}

	originalDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})
	require.NoError(t, os.Chdir(tempDir))

	t.Setenv("GO_TEST_ENV", "1")

	err = exec.Command("git", "init").Run()
	if err != nil {
		t.Skip("Git not available")
	}

	_ = exec.Command("git", "config", "user.name", "Test").Run()
	_ = exec.Command("git", "config", "user.email", "test@test.com").Run()

	require.NoError(t, os.WriteFile("test.txt", []byte("test"), 0644))
	require.NoError(t, exec.Command("git", "add", "test.txt").Run())

	err = Commit("test: safe commit outside temp patterns")
	assert.NoError(t, err, "Commit should succeed outside hardcoded temp patterns")
}
