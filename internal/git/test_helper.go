//go:build !prod

package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSafetyCheck ensures tests are not running in a real repository
// This should be called at the beginning of any test that performs git operations
func TestSafetyCheck(t *testing.T) {
	t.Helper()

	client := NewClient(Options{})

	// Check if we're in a temporary directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Temporary directories usually contain "tmp" or specific test patterns
	isTempDir := strings.Contains(cwd, "/tmp/") ||
		strings.Contains(cwd, "\\Temp\\") ||
		strings.Contains(cwd, "gmc_git_test") ||
		strings.Contains(cwd, "gmc_non_git_test") ||
		strings.HasPrefix(filepath.Base(cwd), "gmc_git_test")

	// If we're in a git repository and NOT in a temp directory, refuse to run
	if client.IsGitRepository() && !isTempDir {
		t.Fatal("SAFETY: Test is attempting to run git operations in a real repository. " +
			"Tests must run in isolated temporary directories. " +
			"Current directory: " + cwd)
	}
}

// CreateSafeTempRepo creates a temporary git repository for testing
// It ensures complete isolation from the real repository
func CreateSafeTempRepo(t *testing.T) (tempDir string, cleanup func()) {
	t.Helper()

	client := NewClient(Options{})

	// First, ensure we're not already in a git repo
	if client.IsGitRepository() {
		t.Fatal("SAFETY: Cannot create temp repo while in an existing git repository")
	}

	// Create temp directory with a unique pattern
	tempDir, err := os.MkdirTemp("", "gmc_git_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Save current directory
	originalDir, err := os.Getwd()
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Verify we're in the temp directory
	cwd, err := os.Getwd()
	if err != nil || cwd != tempDir {
		_ = os.RemoveAll(tempDir)
		t.Fatalf("Failed to verify temp directory change. Current: %s, Expected: %s", cwd, tempDir)
	}

	// Cleanup function
	cleanup = func() {
		// Always try to return to original directory first
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("Warning: Failed to return to original directory: %v", err)
		}
		// Then remove temp directory
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Warning: Failed to remove temp directory: %v", err)
		}
	}

	return tempDir, cleanup
}

// VerifyTestIsolation checks that a test is properly isolated
func VerifyTestIsolation(t *testing.T) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Check for common project files that shouldn't be in test directory
	dangerousFiles := []string{
		"go.mod",      // Project root indicator
		"Makefile",    // Build configuration
		".git/config", // Git configuration
		"README.md",   // Project documentation
		"main.go",     // Main entry point
	}

	foundDangerousFiles := []string{}
	for _, file := range dangerousFiles {
		if _, err := os.Stat(file); err == nil {
			foundDangerousFiles = append(foundDangerousFiles, file)
		}
	}

	// If we found dangerous files and we're not in a temp directory, fail
	if len(foundDangerousFiles) > 0 {
		isTempDir := strings.Contains(cwd, "/tmp/") ||
			strings.Contains(cwd, "\\Temp\\") ||
			strings.Contains(cwd, "gmc_git_test") ||
			strings.Contains(cwd, "gmc_non_git_test")

		if !isTempDir {
			t.Fatalf("SAFETY: Test appears to be running in the real project directory. "+
				"Found project files %v in %s", foundDangerousFiles, cwd)
		}
	}
}

// AssertNotInRealRepo ensures the test is not running in a real git repository
func AssertNotInRealRepo(t *testing.T) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Check if current directory contains the project name
	if strings.Contains(cwd, "samzong/gmc") && !strings.Contains(cwd, "/tmp/") {
		t.Fatal("SAFETY: Test is running in the real GMC project directory. " +
			"This is dangerous and could corrupt the repository. " +
			"Tests must run in temporary directories.")
	}
}
