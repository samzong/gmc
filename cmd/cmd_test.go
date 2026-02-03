package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/samzong/gmc/internal/workflow"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	assert.Equal(t, "dev", Version)
	assert.Equal(t, "unknown", BuildTime)

	assert.NotNil(t, versionCmd)
	assert.Equal(t, "version", versionCmd.Use)
	assert.Equal(t, "Show gmc version information", versionCmd.Short)
}

func TestRootCommand(t *testing.T) {
	assert.NotNil(t, rootCmd)
	assert.Equal(t, "gmc", rootCmd.Use)
	assert.Equal(t, "gmc - Git Message Assistant", rootCmd.Short)
	assert.Contains(t, rootCmd.Long, "gmc is a CLI tool")
	assert.False(t, rootCmd.SilenceErrors)
	assert.True(t, rootCmd.SilenceUsage)
}

func TestInitConfig(t *testing.T) {
	viper.Reset()

	cfgFile = ""
	initConfig()

	assert.NotPanics(t, func() {
		initConfig()
	})
}

func TestHandleErrors(t *testing.T) {
	t.Run("returns nil for nil error", func(t *testing.T) {
		assert.NoError(t, handleErrors(nil, false))
	})

	t.Run("propagates sentinel error", func(t *testing.T) {
		errWithHint := handleErrors(workflow.ErrNoChanges, false)
		errWithoutHint := handleErrors(workflow.ErrNoChanges, true)

		assert.NotEqual(t, errWithHint.Error(), errWithoutHint.Error())

		assert.ErrorIs(t, errWithHint, workflow.ErrNoChanges)
		assert.ErrorIs(t, errWithoutHint, workflow.ErrNoChanges)
	})

	t.Run("propagates generic error", func(t *testing.T) {
		expectedErr := errors.New("boom")
		err := handleErrors(expectedErr, false)
		assert.ErrorIs(t, err, expectedErr)
		assert.Contains(t, err.Error(), expectedErr.Error())
	})
}

func TestGlobalVariables(t *testing.T) {
	assert.IsType(t, "", cfgFile)
	assert.IsType(t, false, noVerify)
	assert.IsType(t, false, dryRun)
	assert.IsType(t, false, addAll)
	assert.IsType(t, "", issueNum)
	assert.IsType(t, false, autoYes)
	assert.IsType(t, false, verbose)
	assert.IsType(t, "", branchDesc)
}

func TestCommitFlow_DryRun(t *testing.T) {
	viper.Reset()
	viper.Set("api_key", "test-api-key")
	viper.Set("model", "gpt-3.5-turbo")
	viper.Set("api_base", "http://127.0.0.1:1")
	viper.Set("role", "Developer")

	cfg, err := config.GetConfig()
	assert.NoError(t, err)

	gitClient := git.NewClient(git.Options{})
	llmClient := llm.NewClient(llm.Options{Timeout: 250 * time.Millisecond})

	opts := workflow.CommitOptions{
		DryRun:    true,
		AutoYes:   true,
		ErrWriter: os.Stderr,
		OutWriter: os.Stdout,
	}

	flow := workflow.NewCommitFlow(gitClient, llmClient, cfg, opts)
	err = flow.Run([]string{})

	if err != nil {
		assert.True(t,
			errors.Is(err, workflow.ErrNoChanges) ||
				strings.Contains(err.Error(), "failed to generate commit message") ||
				strings.Contains(err.Error(), "failed to get git diff"),
			"Expected workflow-related error: %v", err)
	}
}

func TestCommandFlags(t *testing.T) {
	flags := rootCmd.Flags()

	persistentFlags := rootCmd.PersistentFlags()
	configFlag := persistentFlags.Lookup("config")
	assert.NotNil(t, configFlag)
	assert.Equal(t, "string", configFlag.Value.Type())

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
	assert.NotNil(t, configCmd)
	assert.Equal(t, "config", configCmd.Use)
	assert.Equal(t, "Manage gmc configuration", configCmd.Short)
	assert.Contains(t, configCmd.Long, "Manage gmc configuration")
}

func TestStringTrimming(t *testing.T) {
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
	originalErr := errors.New("original error")
	wrappedErr := fmt.Errorf("wrapped: %w", originalErr)

	assert.Contains(t, wrappedErr.Error(), "wrapped")
	assert.Contains(t, wrappedErr.Error(), "original error")

	formattedErr := fmt.Errorf("failed to %s: %w", "do something", originalErr)
	assert.Contains(t, formattedErr.Error(), "failed to do something")
	assert.Contains(t, formattedErr.Error(), "original error")
}

func TestGenerateAndCommit(t *testing.T) {
	originalBranchDesc := branchDesc
	originalAddAll := addAll
	originalVerbose := verbose
	defer func() {
		branchDesc = originalBranchDesc
		addAll = originalAddAll
		verbose = originalVerbose
	}()

	viper.Reset()
	viper.Set("api_key", "test-api-key")
	viper.Set("model", "gpt-3.5-turbo")
	viper.Set("role", "Developer")

	branchDesc = ""
	addAll = false
	verbose = false

	err := generateAndCommit(strings.NewReader(""), []string{})

	if err != nil {
		errorMsg := err.Error()
		assert.True(t,
			strings.Contains(errorMsg, "failed to get git diff") ||
				strings.Contains(errorMsg, "failed to generate commit message") ||
				strings.Contains(errorMsg, "no changes detected"),
			"Error should be related to git or LLM operations: %s", errorMsg)
	}
}

func TestConfigCommands(t *testing.T) {
	assert.NotNil(t, configSetCmd)
	assert.Equal(t, "set", configSetCmd.Use)
	assert.Equal(t, "Set configuration item", configSetCmd.Short)

	assert.NotNil(t, configSetRoleCmd)
	assert.Equal(t, "role [Role Name]", configSetRoleCmd.Use)
	assert.Equal(t, "Set Current Role", configSetRoleCmd.Short)

	assert.NotNil(t, configSetModelCmd)
	assert.Equal(t, "model [Model Name]", configSetModelCmd.Use)
}

func TestRootCommandWithConfigError(t *testing.T) {
	originalConfigErr := configErr
	defer func() { configErr = originalConfigErr }()

	configErr = errors.New("test config error")

	err := rootCmd.RunE(rootCmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration error")
	assert.Contains(t, err.Error(), "test config error")

	configErr = nil
}

func TestRootCommandSuccess(t *testing.T) {
	originalConfigErr := configErr
	defer func() { configErr = originalConfigErr }()

	viper.Reset()
	viper.Set("api_key", "test-api-key")
	viper.Set("model", "gpt-3.5-turbo")
	viper.Set("role", "Developer")

	configErr = nil

	err := rootCmd.RunE(rootCmd, []string{})

	if err != nil {
		assert.NotContains(t, err.Error(), "configuration error")
	}

	configErr = originalConfigErr
}

func TestExecute(t *testing.T) {
	assert.NotNil(t, Execute)

	viper.Reset()
	viper.Set("api_key", "test-api-key")
	viper.Set("model", "gpt-3.5-turbo")
	viper.Set("role", "Developer")

	assert.NotPanics(t, func() {
		_ = Execute()
	})
}

func TestExtractFilesFromDiff(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
diff --git a/cmd/root.go b/cmd/root.go
--- a/cmd/root.go
+++ b/cmd/root.go`

	files := workflow.ExtractFilesFromDiff(diff)
	assert.Contains(t, files, "main.go")
	assert.Contains(t, files, "cmd/root.go")
}
