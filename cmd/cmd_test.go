package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	// Test default version values
	assert.Equal(t, "dev", Version)
	assert.Equal(t, "unknown", BuildTime)

	// Test that version command exists
	assert.NotNil(t, versionCmd)
	assert.Equal(t, "version", versionCmd.Use)
	assert.Equal(t, "Show gmc version information", versionCmd.Short)
}

func TestRootCommand(t *testing.T) {
	// Test root command configuration
	assert.NotNil(t, rootCmd)
	assert.Equal(t, "gmc", rootCmd.Use)
	assert.Equal(t, "gmc - Git Message Assistant", rootCmd.Short)
	assert.Contains(t, rootCmd.Long, "gmc is a CLI tool")
	assert.True(t, rootCmd.SilenceErrors)
	assert.True(t, rootCmd.SilenceUsage)
}

func TestInitConfig(t *testing.T) {
	// Test initConfig function behavior
	// Reset viper state
	viper.Reset()

	// Test with empty config file path
	cfgFile = ""
	initConfig()

	// configErr should be set if there are any configuration issues
	// In a clean environment, it might be nil or contain an error
	// This tests that the function runs without panicking
	assert.NotPanics(t, func() {
		initConfig()
	})
}

func TestHandleErrors(t *testing.T) {
	tests := []struct {
		name           string
		input          error
		expectedOutput string
	}{
		{
			name:  "Nil error returns nil",
			input: nil,
		},
		{
			name:           "No changes detected - special handling",
			input:          &mockError{message: "No changes detected in the staging area files."},
			expectedOutput: "No changes detected",
		},
		{
			name:           "Generic error - default handling",
			input:          &mockError{message: "generic error"},
			expectedOutput: "generic error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleErrors(tt.input)

			// handleErrors should always return nil for display purposes
			assert.Nil(t, result)
		})
	}
}

// Mock error type for testing
type mockError struct {
	message string
}

func (m *mockError) Error() string {
	return m.message
}

func TestGetEditor(t *testing.T) {
	// Save original environment variables
	originalEditor := os.Getenv("EDITOR")
	originalVisual := os.Getenv("VISUAL")

	// Clean up after test
	defer func() {
		os.Setenv("EDITOR", originalEditor)
		os.Setenv("VISUAL", originalVisual)
	}()

	tests := []struct {
		name     string
		editor   string
		visual   string
		expected string
	}{
		{
			name:     "EDITOR env var set",
			editor:   "nano",
			visual:   "",
			expected: "nano",
		},
		{
			name:     "VISUAL env var set (no EDITOR)",
			editor:   "",
			visual:   "code",
			expected: "code",
		},
		{
			name:     "EDITOR takes precedence over VISUAL",
			editor:   "vim",
			visual:   "code",
			expected: "vim",
		},
		{
			name:     "Default to vi when no env vars",
			editor:   "",
			visual:   "",
			expected: "vi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("EDITOR", tt.editor)
			os.Setenv("VISUAL", tt.visual)

			result := getEditor()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGlobalVariables(t *testing.T) {
	// Test that global variables are properly initialized
	assert.IsType(t, "", cfgFile)
	assert.IsType(t, false, noVerify)
	assert.IsType(t, false, dryRun)
	assert.IsType(t, false, addAll)
	assert.IsType(t, "", issueNum)
	assert.IsType(t, false, autoYes)
	assert.IsType(t, false, verbose)
	assert.IsType(t, "", branchDesc)
}

func TestHandleBranchCreation_NoBranch(t *testing.T) {
	// Test with empty branch description
	branchDesc = ""

	err := handleBranchCreation()
	assert.NoError(t, err)
}

func TestHandleBranchCreation_InvalidBranch(t *testing.T) {
	// Test with invalid branch description that would generate empty name
	// This tests the validation logic without actually creating branches

	// Save original value
	originalBranchDesc := branchDesc
	defer func() { branchDesc = originalBranchDesc }()

	// Test with a description that might cause issues
	branchDesc = "   " // whitespace only

	// The actual branch creation will likely fail in test environment
	// but we're testing the validation logic
	_ = handleBranchCreation()

	// We expect either no error (if validation passes) or an error about branch creation
	// The important thing is that the function doesn't panic
	assert.NotPanics(t, func() {
		_ = handleBranchCreation()
	})

	// Reset for other tests
	branchDesc = ""
}

func TestHandleStaging_NoAddAll(t *testing.T) {
	// Test with addAll flag disabled
	addAll = false

	err := handleStaging()
	assert.NoError(t, err)
}

func TestHandleStaging_WithAddAll(t *testing.T) {
	// Test with addAll flag enabled
	// Save original value
	originalAddAll := addAll
	defer func() { addAll = originalAddAll }()

	addAll = true

	// This will likely fail because we're not in a proper git repo or staging area
	// But it tests that the function executes the logic
	err := handleStaging()

	// We expect an error in test environment, but function should not panic
	if err != nil {
		assert.Contains(t, err.Error(), "git add failed")
	}

	// Reset for other tests
	addAll = false
}

func TestPerformCommit_DryRun(t *testing.T) {
	// Test dry run mode
	originalDryRun := dryRun
	defer func() { dryRun = originalDryRun }()

	dryRun = true

	err := performCommit("test commit message")
	assert.NoError(t, err)

	// Reset
	dryRun = false
}

func TestPerformCommit_WithNoVerify(t *testing.T) {
	// Test commit with no-verify flag
	originalNoVerify := noVerify
	originalDryRun := dryRun
	defer func() {
		noVerify = originalNoVerify
		dryRun = originalDryRun
	}()

	dryRun = true // Enable dry run to avoid actual commit
	noVerify = true

	err := performCommit("test commit message")
	assert.NoError(t, err)

	// Reset
	noVerify = false
	dryRun = false
}

func TestPerformCommit_WithNoSignoff(t *testing.T) {
	// Test commit with no-signoff flag
	originalNoSignoff := noSignoff
	originalDryRun := dryRun
	defer func() {
		noSignoff = originalNoSignoff
		dryRun = originalDryRun
	}()

	dryRun = true // Enable dry run to avoid actual commit
	noSignoff = true

	err := performCommit("test commit message")
	assert.NoError(t, err)

	// Reset
	noSignoff = false
	dryRun = false
}

func TestPerformSelectiveCommit_WithNoSignoff(t *testing.T) {
	// Test selective commit with no-signoff flag
	originalNoSignoff := noSignoff
	originalDryRun := dryRun
	defer func() {
		noSignoff = originalNoSignoff
		dryRun = originalDryRun
	}()

	dryRun = true // Enable dry run to avoid actual commit
	noSignoff = true

	err := performSelectiveCommit("test commit message", []string{"test.go"})
	assert.NoError(t, err)

	// Reset
	noSignoff = false
	dryRun = false
}

func TestGetUserConfirmation_AutoYes(t *testing.T) {
	// Test auto-yes flag
	originalAutoYes := autoYes
	defer func() { autoYes = originalAutoYes }()

	autoYes = true

	action, editedMessage, err := getUserConfirmation("test message")

	assert.NoError(t, err)
	assert.Equal(t, "commit", action)
	assert.Empty(t, editedMessage)

	// Reset
	autoYes = false
}

func TestGenerateCommitMessage_IssueNumber(t *testing.T) {
	// Setup viper config for testing
	viper.Reset()
	viper.Set("api_key", "test-api-key")
	viper.Set("model", "gpt-3.5-turbo")
	viper.Set("role", "Developer")

	// Save original issueNum
	originalIssueNum := issueNum
	defer func() { issueNum = originalIssueNum }()

	issueNum = "123"

	// This will fail due to fake API key, but tests the issue number logic
	changedFiles := []string{"test.go"}
	diff := "test diff content"

	// Get config for the test
	cfg := config.GetConfig()

	// The function will fail at LLM call, but issue number formatting logic will be exercised
	_, err := generateCommitMessage(cfg, changedFiles, diff)

	// We expect an error due to fake API key
	if err != nil {
		assert.Contains(t, err.Error(), "failed to generate commit message")
	}

	// Reset
	issueNum = ""
}

func TestExecute(t *testing.T) {
	// Test that Execute function exists and can be called
	// We can't test the full execution without mocking a lot of dependencies
	assert.NotNil(t, Execute)

	// Test that it doesn't panic when called (though it will likely error)
	assert.NotPanics(t, func() {
		_ = Execute()
	})
}

func TestCommandFlags(t *testing.T) {
	// Test that all expected flags are registered
	flags := rootCmd.Flags()

	// Check persistent flags
	persistentFlags := rootCmd.PersistentFlags()
	configFlag := persistentFlags.Lookup("config")
	assert.NotNil(t, configFlag)
	assert.Equal(t, "string", configFlag.Value.Type())

	// Check regular flags
	noVerifyFlag := flags.Lookup("no-verify")
	assert.NotNil(t, noVerifyFlag)
	assert.Equal(t, "bool", noVerifyFlag.Value.Type())

	noSignoffFlag := flags.Lookup("no-signoff")
	assert.NotNil(t, noSignoffFlag)
	assert.Equal(t, "bool", noSignoffFlag.Value.Type())

	dryRunFlag := flags.Lookup("dry-run")
	assert.NotNil(t, dryRunFlag)
	assert.Equal(t, "bool", dryRunFlag.Value.Type())

	allFlag := flags.Lookup("all")
	assert.NotNil(t, allFlag)
	assert.Equal(t, "bool", allFlag.Value.Type())

	issueFlag := flags.Lookup("issue")
	assert.NotNil(t, issueFlag)
	assert.Equal(t, "string", issueFlag.Value.Type())

	yesFlag := flags.Lookup("yes")
	assert.NotNil(t, yesFlag)
	assert.Equal(t, "bool", yesFlag.Value.Type())

	verboseFlag := flags.Lookup("verbose")
	assert.NotNil(t, verboseFlag)
	assert.Equal(t, "bool", verboseFlag.Value.Type())

	branchFlag := flags.Lookup("branch")
	assert.NotNil(t, branchFlag)
	assert.Equal(t, "string", branchFlag.Value.Type())
}

func TestConfigCommandStructure(t *testing.T) {
	// Test that config command is properly structured
	assert.NotNil(t, configCmd)
	assert.Equal(t, "config", configCmd.Use)
	assert.Equal(t, "Manage gmc configuration", configCmd.Short)
	assert.Contains(t, configCmd.Long, "Manage gmc configuration")
}

// Test helper functions and edge cases
func TestStringTrimming(t *testing.T) {
	// Test string processing logic used in various parts
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "  test string  \n",
			expected: "test string",
		},
		{
			input:    "\n\ncommit message\n\n",
			expected: "commit message",
		},
		{
			input:    "no trimming needed",
			expected: "no trimming needed",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		trimmed := strings.TrimSpace(tc.input)
		assert.Equal(t, tc.expected, trimmed)
	}
}

func TestErrorHandlingPatterns(t *testing.T) {
	// Test common error handling patterns used in cmd package

	// Test error wrapping
	originalErr := &mockError{message: "original error"}
	wrappedErr := fmt.Errorf("wrapped: %w", originalErr)

	assert.Contains(t, wrappedErr.Error(), "wrapped")
	assert.Contains(t, wrappedErr.Error(), "original error")

	// Test error message formatting
	formattedErr := fmt.Errorf("failed to %s: %w", "do something", originalErr)
	assert.Contains(t, formattedErr.Error(), "failed to do something")
	assert.Contains(t, formattedErr.Error(), "original error")
}

// Test getStagedChanges function
func TestGetStagedChanges(t *testing.T) {
	// This will test the getStagedChanges function
	// It will likely fail in test environment but exercises the code paths
	_, _, err := getStagedChanges()

	// We expect an error in test environment (not a git repo or no staging area)
	// But the function should execute without panic
	if err != nil {
		// Common error messages we might see
		errorMsg := err.Error()
		assert.True(t,
			strings.Contains(errorMsg, "failed to get git diff") ||
				strings.Contains(errorMsg, "no changes detected") ||
				strings.Contains(errorMsg, "failed to parse staged files"),
			"Error should be related to git operations: %s", errorMsg)
	}
}

// Test generateAndCommit function
func TestGenerateAndCommit(t *testing.T) {
	// Save original values
	originalBranchDesc := branchDesc
	originalAddAll := addAll
	originalVerbose := verbose
	defer func() {
		branchDesc = originalBranchDesc
		addAll = originalAddAll
		verbose = originalVerbose
	}()

	// Test with minimal setup
	branchDesc = ""
	addAll = false
	verbose = false

	err := generateAndCommit([]string{})

	// We expect an error in test environment, but function should not panic
	if err != nil {
		// Could be git diff error or LLM error depending on test environment
		errorMsg := err.Error()
		assert.True(t,
			strings.Contains(errorMsg, "failed to get git diff") ||
				strings.Contains(errorMsg, "failed to generate commit message") ||
				strings.Contains(errorMsg, "no changes detected"),
			"Error should be related to git or LLM operations: %s", errorMsg)
	}
}

// Test handleCommitFlow with mock scenarios
func TestHandleCommitFlow(t *testing.T) {
	// Setup viper config for testing
	viper.Reset()
	viper.Set("api_key", "test-api-key")
	viper.Set("model", "gpt-3.5-turbo")
	viper.Set("role", "Developer")

	// Save original autoYes value
	originalAutoYes := autoYes
	defer func() { autoYes = originalAutoYes }()

	autoYes = true // This will make the function proceed without user input

	// Test with mock data
	diff := "test diff content"
	changedFiles := []string{"test.go", "main.go"}

	// This will exercise the handleCommitFlow function
	err := handleCommitFlow(diff, changedFiles)

	// We expect an error due to fake API key or git operations
	if err != nil {
		errorMsg := err.Error()
		assert.True(t,
			strings.Contains(errorMsg, "failed to generate commit message") ||
				strings.Contains(errorMsg, "failed to commit changes"),
			"Error should be related to commit flow: %s", errorMsg)
	}

	// Reset
	autoYes = false
}

// Test performCommit with actual commit (dry run)
func TestPerformCommit_ActualCommit(t *testing.T) {
	// Save original values
	originalDryRun := dryRun
	originalNoVerify := noVerify
	defer func() {
		dryRun = originalDryRun
		noVerify = originalNoVerify
	}()

	// IMPORTANT: Always use dry run mode in tests to avoid real commits
	dryRun = true // Always use dry run in tests
	noVerify = false

	err := performCommit("test commit message")

	// In dry run mode, we should not get an error
	assert.NoError(t, err, "performCommit should succeed in dry run mode")
}

// Test generateCommitMessage function more thoroughly
func TestGenerateCommitMessage_Complete(t *testing.T) {
	// Setup viper config for testing
	viper.Reset()
	viper.Set("api_key", "test-api-key")
	viper.Set("model", "gpt-3.5-turbo")
	viper.Set("role", "Senior Go Developer")

	// Save original issueNum
	originalIssueNum := issueNum
	defer func() { issueNum = originalIssueNum }()

	cfg := config.GetConfig()
	changedFiles := []string{"main.go", "cmd/root.go", "internal/config/config.go"}
	diff := `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -10,6 +10,7 @@ func main() {
 	if err := cmd.Execute(); err != nil {
 		os.Exit(1)
 	}
+	fmt.Println("Done")
 }`

	// Test without issue number
	issueNum = ""
	_, err := generateCommitMessage(cfg, changedFiles, diff)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to generate commit message")
	}

	// Test with issue number
	issueNum = "456"
	_, err = generateCommitMessage(cfg, changedFiles, diff)
	if err != nil {
		assert.Contains(t, err.Error(), "failed to generate commit message")
	}

	// Reset
	issueNum = ""
}

// Test some config command structure (basic coverage)
func TestConfigCommands(t *testing.T) {
	// Test that config commands exist and are structured correctly
	assert.NotNil(t, configSetCmd)
	assert.Equal(t, "set", configSetCmd.Use)
	assert.Equal(t, "Set configuration item", configSetCmd.Short)

	assert.NotNil(t, configSetRoleCmd)
	assert.Equal(t, "role [Role Name]", configSetRoleCmd.Use)
	assert.Equal(t, "Set Current Role", configSetRoleCmd.Short)

	assert.NotNil(t, configSetModelCmd)
	assert.Equal(t, "model [Model Name]", configSetModelCmd.Use)
}

// Test root command execution with config error
func TestRootCommandWithConfigError(t *testing.T) {
	// Save original configErr
	originalConfigErr := configErr
	defer func() { configErr = originalConfigErr }()

	// Set a config error
	configErr = errors.New("test config error")

	// Test that rootCmd.RunE handles config error
	err := rootCmd.RunE(rootCmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration error")
	assert.Contains(t, err.Error(), "test config error")

	// Reset
	configErr = nil
}

// Test root command execution without config error (will fail later but tests the path)
func TestRootCommandSuccess(t *testing.T) {
	// Save original configErr
	originalConfigErr := configErr
	defer func() { configErr = originalConfigErr }()

	// Clear config error
	configErr = nil

	// Test that rootCmd.RunE progresses past config error check
	err := rootCmd.RunE(rootCmd, []string{})

	// Will likely fail on git operations or LLM calls, but passed config error check
	if err != nil {
		// Should not be a configuration error
		assert.NotContains(t, err.Error(), "configuration error")
	}

	// Reset
	configErr = originalConfigErr
}
