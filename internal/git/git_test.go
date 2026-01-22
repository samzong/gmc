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

func TestParseCommitOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
		wantErr  bool
	}{
		{
			name: "Valid commit output",
			input: `abc1234|John Doe|2024-01-15|feat: add new feature
def5678|Jane Smith|2024-01-14|fix: resolve bug in parser`,
			expected: 2,
			wantErr:  false,
		},
		{
			name:     "Empty input",
			input:    "",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "Single commit",
			input:    `abc1234|John Doe|2024-01-15|feat: add new feature`,
			expected: 1,
			wantErr:  false,
		},
		{
			name: "Malformed line (should be skipped)",
			input: `abc1234|John Doe|2024-01-15|feat: add new feature
invalid-line
def5678|Jane Smith|2024-01-14|fix: resolve bug`,
			expected: 2,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commits, err := parseCommitOutput(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("parseCommitOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(commits) != tt.expected {
				t.Errorf("parseCommitOutput() got %d commits, want %d", len(commits), tt.expected)
				return
			}

			// Test first commit if exists
			if len(commits) > 0 {
				commit := commits[0]
				if commit.Hash == "" || commit.Author == "" || commit.Date == "" || commit.Message == "" {
					t.Errorf("parseCommitOutput() commit has empty fields: %+v", commit)
				}
			}
		})
	}
}

func TestCommitInfoFields(t *testing.T) {
	input := `abc1234|John Doe|2024-01-15|feat: add new feature`
	commits, err := parseCommitOutput(input)

	if err != nil {
		t.Fatalf("parseCommitOutput() error = %v", err)
	}

	if len(commits) != 1 {
		t.Fatalf("parseCommitOutput() got %d commits, want 1", len(commits))
	}

	commit := commits[0]

	if commit.Hash != "abc1234" {
		t.Errorf("commit.Hash = %q, want %q", commit.Hash, "abc1234")
	}

	if commit.Author != "John Doe" {
		t.Errorf("commit.Author = %q, want %q", commit.Author, "John Doe")
	}

	if commit.Date != "2024-01-15" {
		t.Errorf("commit.Date = %q, want %q", commit.Date, "2024-01-15")
	}

	if commit.Message != "feat: add new feature" {
		t.Errorf("commit.Message = %q, want %q", commit.Message, "feat: add new feature")
	}
}

// Test CommitInfo struct
func TestCommitInfo(t *testing.T) {
	commit := CommitInfo{
		Hash:    "abc123",
		Author:  "Test User",
		Date:    "2024-01-15",
		Message: "test commit message",
	}

	assert.Equal(t, "abc123", commit.Hash)
	assert.Equal(t, "Test User", commit.Author)
	assert.Equal(t, "2024-01-15", commit.Date)
	assert.Equal(t, "test commit message", commit.Message)
}

// Test ParseChangedFiles function signature
func TestParseChangedFiles(t *testing.T) {
	client := NewClient(Options{})

	// Test that ParseChangedFiles has correct signature
	// If we're in a git repo, it should succeed, otherwise error
	files, err := client.ParseChangedFiles()
	if client.IsGitRepository() {
		assert.NoError(t, err, "ParseChangedFiles should succeed in git repo")
		assert.NotNil(t, files, "Files slice should not be nil")
	} else {
		assert.Error(t, err, "ParseChangedFiles should error outside git repo")
		assert.Contains(t, err.Error(), "not in a git repository")
	}
}

func TestClientOptions_Verbose(t *testing.T) {
	client := NewClient(Options{Verbose: true})
	assert.True(t, client.verbose)
}

// Integration tests that require a real git repository
// These tests will be skipped if not in a git repository
func TestGitIntegration(t *testing.T) {
	client := NewClient(Options{})

	// Check if we're in a git repository
	if !client.IsGitRepository() {
		t.Skip("Not in a git repository, skipping integration tests")
	}

	t.Run("IsGitRepository", func(t *testing.T) {
		result := client.IsGitRepository()
		assert.True(t, result, "Should detect git repository")
	})

	t.Run("CheckGitRepository", func(t *testing.T) {
		err := client.CheckGitRepository()
		assert.NoError(t, err, "Should not error in git repository")
	})
}

// Test git functions with a temporary git repository
func TestWithTempGitRepo(t *testing.T) {
	client := NewClient(Options{})

	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test that performs real git operations; set RUN_INTEGRATION_TESTS=1 to enable")
	}

	// SAFETY CHECK: Ensure we're not in a real git repository
	if client.IsGitRepository() {
		t.Fatal("SAFETY: Refusing to run integration tests in an existing git repository. Tests must run in isolation.")
	}

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "gmc_git_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Save current directory
	currentDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		// Always return to original directory
		if err := os.Chdir(currentDir); err != nil {
			t.Errorf("Failed to return to original directory: %v", err)
		}
	}()

	// Change to temp directory
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// DOUBLE CHECK: Ensure we're in the temp directory
	cwd, err := os.Getwd()
	require.NoError(t, err)
	if cwd != tempDir {
		t.Fatalf("SAFETY: Failed to change to temp directory. Current: %s, Expected: %s", cwd, tempDir)
	}

	// Initialize git repository
	err = exec.Command("git", "init").Run()
	if err != nil {
		t.Skip("Git not available, skipping git integration tests")
		return
	}

	// Configure git user for tests
	err = exec.Command("git", "config", "user.name", "Test User").Run()
	require.NoError(t, err)
	err = exec.Command("git", "config", "user.email", "test@example.com").Run()
	require.NoError(t, err)

	t.Run("IsGitRepository_InTempRepo", func(t *testing.T) {
		result := client.IsGitRepository()
		assert.True(t, result)
	})

	t.Run("CheckGitRepository_InTempRepo", func(t *testing.T) {
		err := client.CheckGitRepository()
		assert.NoError(t, err)
	})

	// Create some test files
	testFile := "test.txt"
	err = os.WriteFile(testFile, []byte("Hello World"), 0644)
	require.NoError(t, err)

	t.Run("AddAll", func(t *testing.T) {
		err := client.AddAll()
		assert.NoError(t, err, "AddAll should succeed")
	})

	// Check staged files after add
	t.Run("ParseStagedFiles", func(t *testing.T) {
		files, err := client.ParseStagedFiles()
		assert.NoError(t, err)
		assert.Contains(t, files, "test.txt", "Should contain staged test file")
	})

	t.Run("GetStagedDiff", func(t *testing.T) {
		diff, err := client.GetStagedDiff()
		assert.NoError(t, err)
		assert.Contains(t, diff, "test.txt", "Staged diff should contain test file")
		assert.Contains(t, diff, "Hello World", "Staged diff should contain file content")
	})

	t.Run("GetStagedDiffStats", func(t *testing.T) {
		stats, err := client.GetStagedDiffStats()
		assert.NoError(t, err)
		assert.Contains(t, stats, "test.txt", "Stats should contain test file")
	})

	t.Run("Commit", func(t *testing.T) {
		// Double check we're in temp directory before committing
		cwd, _ := os.Getwd()
		if !strings.Contains(cwd, "gmc_git_test") {
			t.Fatal("SAFETY: Not in temporary test directory, refusing to commit")
		}

		err := client.Commit("test: safe commit in temp repo")
		assert.NoError(t, err, "Commit should succeed in temp repo")
	})

	// Test after commit
	t.Run("GetCommitHistory", func(t *testing.T) {
		commits, err := client.GetCommitHistory(10, false)
		assert.NoError(t, err)
		assert.Len(t, commits, 1, "Should have one commit")
		assert.Equal(t, "test: safe commit in temp repo", commits[0].Message)
		assert.Equal(t, "Test User", commits[0].Author)
	})

	t.Run("GetLatestTag_NoTags", func(t *testing.T) {
		tag, err := client.GetLatestTag()
		assert.NoError(t, err)
		assert.Equal(t, "", tag)
	})

	// Test with more changes
	t.Run("GetDiff_WithUnstagedChanges", func(t *testing.T) {
		// Modify the file
		err := os.WriteFile(testFile, []byte("Hello World\nSecond line"), 0644)
		require.NoError(t, err)

		diff, err := client.GetDiff()
		assert.NoError(t, err)
		assert.Contains(t, diff, "test.txt", "Diff should contain modified file")
		assert.Contains(t, diff, "+Second line", "Diff should show added line")
	})

	t.Run("CreateAndSwitchBranch", func(t *testing.T) {
		err := client.CreateAndSwitchBranch("feature/test-branch")
		assert.NoError(t, err, "Should create and switch to new branch")
	})

	t.Run("CreateAnnotatedTag", func(t *testing.T) {
		err := client.CreateAnnotatedTag("v0.1.0", "Release v0.1.0")
		assert.NoError(t, err)
	})

	t.Run("GetLatestTag_WithTag", func(t *testing.T) {
		tag, err := client.GetLatestTag()
		assert.NoError(t, err)
		assert.Equal(t, "v0.1.0", tag)
	})

	t.Run("GetCommitsSinceTag_NoNewCommits", func(t *testing.T) {
		commits, err := client.GetCommitsSinceTag("v0.1.0")
		assert.NoError(t, err)
		assert.Len(t, commits, 0)
	})

	t.Run("GetCommitsSinceTag_WithCommits", func(t *testing.T) {
		err := os.WriteFile("feature.txt", []byte("new feature"), 0644)
		require.NoError(t, err)

		err = client.AddAll()
		require.NoError(t, err)

		err = client.Commit("feat: add new capability", "-m", "Additional context for feature")
		assert.NoError(t, err)

		commits, err := client.GetCommitsSinceTag("v0.1.0")
		assert.NoError(t, err)
		require.Len(t, commits, 1)
		assert.Equal(t, "feat: add new capability", commits[0].Message)
		assert.Equal(t, "Additional context for feature", commits[0].Body)
	})

	t.Run("GetCommitsSinceTag_UnknownTag", func(t *testing.T) {
		commits, err := client.GetCommitsSinceTag("v9.9.9")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(commits), 2)
	})
}

func TestResolveFilesIncludesUntracked(t *testing.T) {
	client := NewClient(Options{})

	tempDir, err := os.MkdirTemp("", "gmc_git_dir_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	runGitCommand(t, tempDir, "init")
	runGitCommand(t, tempDir, "config", "user.email", "test@example.com")
	runGitCommand(t, tempDir, "config", "user.name", "gmc tester")

	pkgDir := filepath.Join(tempDir, "pkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))

	trackedFile := filepath.Join(pkgDir, "tracked.txt")
	require.NoError(t, os.WriteFile(trackedFile, []byte("tracked"), 0644))
	runGitCommand(t, tempDir, "add", ".")
	runGitCommand(t, tempDir, "commit", "-m", "initial commit")

	untrackedFile := filepath.Join(pkgDir, "draft.txt")
	require.NoError(t, os.WriteFile(untrackedFile, []byte("draft"), 0644))

	originalDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	require.NoError(t, os.Chdir(tempDir))
	AssertNotInRealRepo(t)

	files, err := client.ResolveFiles([]string{"pkg"})
	require.NoError(t, err)

	assert.Contains(t, files, "pkg/tracked.txt")
	assert.Contains(t, files, "pkg/draft.txt")
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}

// Test functions outside of git repository
func TestOutsideGitRepo(t *testing.T) {
	client := NewClient(Options{})

	// Create a temporary directory that's not a git repo
	tempDir, err := os.MkdirTemp("", "gmc_non_git_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Save current directory
	currentDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		// Always return to original directory
		if err := os.Chdir(currentDir); err != nil {
			t.Errorf("Failed to return to original directory: %v", err)
		}
	}()

	// Change to temp directory
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Verify we're in the temp directory
	// On macOS, /var is a symlink to /private/var, so we need to evaluate symlinks
	cwd, err := os.Getwd()
	require.NoError(t, err)
	evalCwd, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	evalTemp, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)
	if evalCwd != evalTemp {
		t.Fatalf("SAFETY: Failed to change to temp directory. Current: %s, Expected: %s", evalCwd, evalTemp)
	}

	t.Run("IsGitRepository_OutsideRepo", func(t *testing.T) {
		result := client.IsGitRepository()
		assert.False(t, result, "Should not detect git repository")
	})

	t.Run("CheckGitRepository_OutsideRepo", func(t *testing.T) {
		err := client.CheckGitRepository()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not in a git repository")
	})

	t.Run("GetDiff_OutsideRepo", func(t *testing.T) {
		_, err := client.GetDiff()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not in a git repository")
	})

	t.Run("GetStagedDiff_OutsideRepo", func(t *testing.T) {
		_, err := client.GetStagedDiff()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not in a git repository")
	})

	t.Run("AddAll_OutsideRepo", func(t *testing.T) {
		err := client.AddAll()
		assert.Error(t, err)
	})

	t.Run("ParseStagedFiles_OutsideRepo", func(t *testing.T) {
		_, err := client.ParseStagedFiles()
		assert.Error(t, err)
	})

	t.Run("Commit_OutsideRepo", func(t *testing.T) {
		err := client.Commit("test message")
		assert.Error(t, err)
	})

	t.Run("CreateAndSwitchBranch_OutsideRepo", func(t *testing.T) {
		err := client.CreateAndSwitchBranch("test-branch")
		assert.Error(t, err)
	})

	t.Run("GetCommitHistory_OutsideRepo", func(t *testing.T) {
		_, err := client.GetCommitHistory(10, false)
		assert.Error(t, err)
	})
}

// Test edge cases and error conditions
func TestEdgeCases(t *testing.T) {
	t.Run("ParseChangedFiles_EdgeCases", func(t *testing.T) {
		client := NewClient(Options{})

		// ParseChangedFiles calls git directly, test behavior based on repo status
		files, err := client.ParseChangedFiles()
		if client.IsGitRepository() {
			assert.NoError(t, err, "Should succeed in git repo")
			assert.NotNil(t, files, "Should return files slice")
		} else {
			assert.Error(t, err, "Should error outside git repo")
		}
	})

	t.Run("ParseCommitOutput_EdgeCases", func(t *testing.T) {
		// Test with malformed commit output
		malformedOutput := `incomplete|line
another|incomplete
valid|hash|User Name|2024-01-01|Valid message`

		commits, err := parseCommitOutput(malformedOutput)
		assert.NoError(t, err, "Should not error on malformed lines")
		assert.Len(t, commits, 1, "Should parse only valid lines")
		assert.Equal(t, "valid", commits[0].Hash)
	})
}

// Test commit validation and branch operations
func TestBranchValidation(t *testing.T) {
	// IMPORTANT: These tests must NOT execute real git operations
	// They should only test validation logic without side effects

	t.Run("CreateAndSwitchBranch_InvalidBranch", func(t *testing.T) {
		client := NewClient(Options{})

		// Skip if in a real git repo to avoid any operations
		if client.IsGitRepository() {
			t.Skip("Skipping test in real git repository for safety")
		}

		// This will fail because we're not in a git repo
		err := client.CreateAndSwitchBranch("invalid..branch..name")
		assert.Error(t, err, "Should error outside git repository")
	})

	// REMOVED dangerous Commit tests that could create real commits
	// These tests should ONLY run in isolated test environments
	// See TestWithTempGitRepo for proper isolated testing of Commit function
}
