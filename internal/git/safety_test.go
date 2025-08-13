package git

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCommitSafetyInRealRepo verifies that Commit refuses to run in real repo during tests
func TestCommitSafetyInRealRepo(t *testing.T) {
	// This test verifies the safety mechanism works
	// It should always fail to commit in the real repo during tests

	// Ensure we have the test environment flag set
	assert.Equal(t, "1", os.Getenv("GO_TEST_ENV"), "Test environment flag should be set")

	// If we're in a git repository (the real project repo)
	if IsGitRepository() {
		// Try to commit - this should ALWAYS fail due to safety check
		err := Commit("DANGER: This should never succeed")

		// We MUST get an error - if not, the safety check failed!
		assert.Error(t, err, "Commit MUST fail in real repository during tests")
		assert.Contains(t, err.Error(), "SAFETY", "Error should mention safety check")
	} else {
		t.Skip("Not in a git repository, safety check not applicable")
	}
}

// TestCommitSafetyInTempDir verifies that Commit works in temp directories
func TestCommitSafetyInTempDir(t *testing.T) {
	// Skip if not doing integration tests
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test")
	}

	// Create a safe temp repo
	tempDir, cleanup := CreateSafeTempRepo(t)
	defer cleanup()

	// Initialize git repo
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	err = exec.Command("git", "init").Run()
	if err != nil {
		t.Skip("Git not available")
	}

	// Configure git
	_ = exec.Command("git", "config", "user.name", "Test").Run()
	_ = exec.Command("git", "config", "user.email", "test@test.com").Run()

	// Create a file and stage it
	err = os.WriteFile("test.txt", []byte("test"), 0644)
	assert.NoError(t, err)

	err = exec.Command("git", "add", "test.txt").Run()
	assert.NoError(t, err)

	// This SHOULD work in temp directory
	err = Commit("test: safe commit in temp dir")
	assert.NoError(t, err, "Commit should succeed in temp directory")
}
