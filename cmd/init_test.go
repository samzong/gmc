package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestRunInitWizard_RequiresAPIKeyAndUsesDefaults(t *testing.T) {
	input := strings.NewReader("\nkey123\n\nhttps://proxy.example/v1\nn\n")
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
	input := strings.NewReader("\ngpt-4.2\n\ny\n")
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
	proceed, err := ensureLLMConfigured(cfg, input, &output, func(in io.Reader, out io.Writer, cfg *config.Config) error {
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

func TestHandleSelectiveCommitFlow_MissingKeyDeclineInit(t *testing.T) {
	originalStdin := os.Stdin
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	_, err = w.WriteString("n\n")
	assert.NoError(t, err)
	assert.NoError(t, w.Close())
	os.Stdin = r
	defer func() {
		os.Stdin = originalStdin
		_ = r.Close()
	}()

	viper.Reset()
	viper.Set("api_key", "")

	err = handleSelectiveCommitFlow("diff", []string{"file.go"})
	assert.NoError(t, err)
}
