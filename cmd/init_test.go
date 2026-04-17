package cmd

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestRunInitWizard_RequiresAPIKeyAndUsesDefaults(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	input := strings.NewReader("\nkey123\n\nhttps://proxy.example/v1\nn\nn\n")
	var output bytes.Buffer

	cfg := &config.Config{
		Model:   "gpt-4.1-mini",
		APIBase: "",
		APIKey:  "",
	}

	var savedAPIKey, savedModel, savedBase string
	origSave := saveConfigValues
	origTest := testLLMConnection
	defer func() {
		saveConfigValues = origSave
		testLLMConnection = origTest
	}()

	saveConfigValues = func(apiKey, model, apiBase string) error {
		savedAPIKey = apiKey
		savedModel = model
		savedBase = apiBase
		return nil
	}

	var testCalled bool
	testLLMConnection = func(_ string) error {
		testCalled = true
		return nil
	}

	err := runInitWizard(input, &output, cfg)
	assert.NoError(t, err)
	assert.Equal(t, "key123", savedAPIKey)
	assert.Equal(t, "gpt-4.1-mini", savedModel)
	assert.Equal(t, "https://proxy.example/v1", savedBase)
	assert.False(t, testCalled)
	assert.Contains(t, output.String(), "API key is required")
}

func TestRunInitWizard_KeepExistingKeyAndTestConnection(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	input := strings.NewReader("\ngpt-4.2\n\ny\nn\n")
	var output bytes.Buffer

	cfg := &config.Config{
		Model:   "gpt-4.1-mini",
		APIBase: "https://proxy.example/v1",
		APIKey:  "existing-key",
	}

	var savedAPIKey, savedModel, savedBase string
	origSave := saveConfigValues
	origTest := testLLMConnection
	defer func() {
		saveConfigValues = origSave
		testLLMConnection = origTest
	}()

	saveConfigValues = func(apiKey, model, apiBase string) error {
		savedAPIKey = apiKey
		savedModel = model
		savedBase = apiBase
		return nil
	}

	var testedModel string
	testLLMConnection = func(model string) error {
		testedModel = model
		return nil
	}

	err := runInitWizard(input, &output, cfg)
	assert.NoError(t, err)
	assert.Equal(t, "existing-key", savedAPIKey)
	assert.Equal(t, "gpt-4.2", savedModel)
	assert.Equal(t, "https://proxy.example/v1", savedBase)
	assert.Equal(t, "gpt-4.2", testedModel)
}

func TestEnsureLLMConfigured_WithAPIKey(t *testing.T) {
	cfg := &config.Config{APIKey: "set"}
	input := strings.NewReader("n\n")
	var output bytes.Buffer

	var initCalled bool
	proceed, err := ensureLLMConfigured(cfg, input, &output, func(_ io.Reader, _ io.Writer, _ *config.Config) error {
		initCalled = true
		return nil
	})
	assert.NoError(t, err)
	assert.True(t, proceed)
	assert.False(t, initCalled)
}

func TestEnsureLLMConfigured_MissingKeyDecline(t *testing.T) {
	cfg := &config.Config{APIKey: ""}
	input := strings.NewReader("n\n")
	var output bytes.Buffer

	var initCalled bool
	proceed, err := ensureLLMConfigured(cfg, input, &output, func(_ io.Reader, _ io.Writer, _ *config.Config) error {
		initCalled = true
		return nil
	})
	assert.NoError(t, err)
	assert.False(t, proceed)
	assert.False(t, initCalled)
	assert.Contains(t, output.String(), "gmc init")
}

func TestEnsureLLMConfigured_MissingKeyAccept(t *testing.T) {
	cfg := &config.Config{APIKey: ""}
	input := strings.NewReader("y\n")
	var output bytes.Buffer

	var initCalled bool
	proceed, err := ensureLLMConfigured(cfg, input, &output, func(_ io.Reader, _ io.Writer, _ *config.Config) error {
		initCalled = true
		return nil
	})
	assert.NoError(t, err)
	assert.True(t, proceed)
	assert.True(t, initCalled)
}

func TestDetectShell(t *testing.T) {
	assert.Equal(t, "zsh", detectShell("/bin/zsh"))
	assert.Equal(t, "bash", detectShell("/usr/local/bin/bash"))
	assert.Equal(t, "fish", detectShell("/opt/homebrew/bin/fish"))
	assert.Equal(t, "", detectShell(""))
	assert.Equal(t, "", detectShell("/bin/tcsh"))
}

func TestMaybeShellIntegration_AcceptZsh(t *testing.T) {
	input := strings.NewReader("y\n")
	var output bytes.Buffer
	readLine := newTrimmedLineReader(input)

	err := maybeShellIntegration(&output, readLine, "/bin/zsh")
	assert.NoError(t, err)
	got := output.String()
	assert.Contains(t, got, "Shell integration")
	assert.Contains(t, got, "~/.zshrc")
	assert.Contains(t, got, `eval "$(gmc wt init zsh)"`)
}

func TestMaybeShellIntegration_DeclineFish(t *testing.T) {
	input := strings.NewReader("n\n")
	var output bytes.Buffer
	readLine := newTrimmedLineReader(input)

	err := maybeShellIntegration(&output, readLine, "/usr/bin/fish")
	assert.NoError(t, err)
	got := output.String()
	assert.Contains(t, got, "Set up shell integration for fish")
	assert.Contains(t, got, "gmc wt init --help")
	assert.NotContains(t, got, "Add this to your")
}

func TestMaybeShellIntegration_UnknownShellEOF(t *testing.T) {
	input := strings.NewReader("")
	var output bytes.Buffer
	readLine := newTrimmedLineReader(input)

	err := maybeShellIntegration(&output, readLine, "")
	assert.NoError(t, err)
	assert.Contains(t, output.String(), "gmc wt init --help")
}

func TestEnsureLLMConfigured_InitError(t *testing.T) {
	cfg := &config.Config{APIKey: ""}
	input := strings.NewReader("y\n")
	var output bytes.Buffer

	expectedErr := errors.New("init failed")
	proceed, err := ensureLLMConfigured(cfg, input, &output, func(_ io.Reader, _ io.Writer, _ *config.Config) error {
		return expectedErr
	})
	assert.ErrorIs(t, err, expectedErr)
	assert.False(t, proceed)
}
