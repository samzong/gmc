package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/exitcode"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/samzong/gmc/internal/workflow"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Contains(t, rootCmd.Short, "Parallel git worktrees")
	assert.Contains(t, rootCmd.Long, "parallel AI coding agents")
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

	for _, test := range []struct {
		err  error
		code int
	}{
		{fmt.Errorf("failed: %w", git.ErrNotGitRepo), exitcode.NotGitRepo},
		{fmt.Errorf("generation failed: %w", llm.ErrLLM), exitcode.LLMError},
		{workflow.ErrNoChanges, exitcode.NoStagedChanges},
	} {
		t.Run(test.err.Error(), func(t *testing.T) {
			err := handleErrors(test.err, false)
			var exitErr *exitcode.Error
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, test.code, exitErr.Code)
			assert.ErrorIs(t, err, test.err)
		})
	}

	t.Run("propagates generic error", func(t *testing.T) {
		expectedErr := errors.New("boom")
		err := handleErrors(expectedErr, false)
		assert.ErrorIs(t, err, expectedErr)
		assert.Contains(t, err.Error(), expectedErr.Error())
	})
}

func TestCommitFlow_DryRun(t *testing.T) {
	configureTestLLM()
	viper.Set("api_base", "http://127.0.0.1:1")

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
	for name, kind := range map[string]string{
		"config": "string", "no-verify": "bool", "no-signoff": "bool", "dry-run": "bool", "all": "bool",
		"issue": "string", "yes": "bool", "verbose": "bool", "branch": "string",
	} {
		t.Run(name, func(t *testing.T) {
			flag := rootCmd.Flags().Lookup(name)
			if name == "config" {
				flag = rootCmd.PersistentFlags().Lookup(name)
			}
			require.NotNil(t, flag)
			assert.Equal(t, kind, flag.Value.Type())
		})
	}
}

func TestConfigCommandStructure(t *testing.T) {
	assert.NotNil(t, configCmd)
	assert.Equal(t, "config", configCmd.Use)
	assert.Equal(t, "Manage gmc configuration", configCmd.Short)
}

func TestGenerateAndCommit(t *testing.T) {
	setTestValue(t, &branchDesc, "")
	setTestValue(t, &addAll, false)
	setTestValue(t, &verbose, false)

	configureTestLLM()

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
	assert.Equal(t, "Set the current role", configSetRoleCmd.Short)

	assert.NotNil(t, configSetModelCmd)
	assert.Equal(t, "model [Model Name]", configSetModelCmd.Use)
}

func TestRootCommandWithConfigError(t *testing.T) {
	setTestValue(t, &configErr, errors.New("test config error"))

	err := rootCmd.RunE(rootCmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration error")
	assert.Contains(t, err.Error(), "test config error")

	configErr = nil
}

func TestRootCommandSuccess(t *testing.T) {
	setTestValue(t, &configErr, nil)

	configureTestLLM()

	configErr = nil

	err := rootCmd.RunE(rootCmd, []string{})

	if err != nil {
		assert.NotContains(t, err.Error(), "configuration error")
	}
}

func TestExecute(t *testing.T) {
	assert.NotNil(t, Execute)

	configureTestLLM()

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

func configureTestLLM() {
	viper.Reset()
	viper.Set("api_key", "test-api-key")
	viper.Set("model", "gpt-3.5-turbo")
	viper.Set("role", "Developer")
}
