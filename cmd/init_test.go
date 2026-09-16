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

func TestRunInitWizard(t *testing.T) {
	for _, test := range []struct {
		name, input         string
		current, want       config.Config
		testedModel, output string
	}{
		{
			name: "requires key and uses defaults", input: "\nkey123\n\nhttps://proxy.example/v1\nn\n" + "n\n",
			current: config.Config{Model: "gpt-4.1-mini"},
			want:    config.Config{APIKey: "key123", Model: "gpt-4.1-mini", APIBase: "https://proxy.example/v1"},
			output:  "API key is required",
		},
		{
			name: "keeps key and tests connection", input: "\ngpt-4.2\n\ny\nn\n",
			current:     config.Config{APIKey: "existing-key", Model: "gpt-4.1-mini", APIBase: "https://proxy.example/v1"},
			want:        config.Config{APIKey: "existing-key", Model: "gpt-4.2", APIBase: "https://proxy.example/v1"},
			testedModel: "gpt-4.2",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SHELL", "/bin/zsh")
			var saved config.Config
			setTestValue(t, &saveConfigValues, func(apiKey, model, apiBase string) error {
				saved = config.Config{APIKey: apiKey, Model: model, APIBase: apiBase}
				return nil
			})
			var testedModel string
			var testCalled bool
			setTestValue(t, &testLLMConnection, func(model string) error {
				testedModel = model
				testCalled = true
				return nil
			})
			var output bytes.Buffer
			assert.NoError(t, runInitWizard(strings.NewReader(test.input), &output, &test.current))
			assert.Equal(t, test.want, saved)
			assert.Equal(t, test.testedModel, testedModel)
			assert.Equal(t, test.testedModel != "", testCalled)
			assert.Contains(t, output.String(), test.output)
		})
	}
}

func TestEnsureLLMConfigured(t *testing.T) {
	expectedErr := errors.New("init failed")
	for _, test := range []struct {
		name, key, input    string
		proceed, initCalled bool
		err                 error
	}{
		{"configured", "set", "n\n", true, false, nil},
		{"decline", "", "n\n", false, false, nil},
		{"accept", "", "y\n", true, true, nil},
		{"init error", "", "y\n", false, true, expectedErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			var initCalled bool
			proceed, err := ensureLLMConfigured(&config.Config{APIKey: test.key}, strings.NewReader(test.input), &output,
				func(_ io.Reader, _ io.Writer, _ *config.Config) error {
					initCalled = true
					return test.err
				})
			assert.ErrorIs(t, err, test.err)
			assert.Equal(t, test.proceed, proceed)
			assert.Equal(t, test.initCalled, initCalled)
			if test.key == "" {
				assert.Contains(t, output.String(), "gmc init")
			}
		})
	}
}

func TestDetectShell(t *testing.T) {
	assert.Equal(t, "zsh", detectShell("/bin/zsh"))
	assert.Equal(t, "bash", detectShell("/usr/local/bin/bash"))
	assert.Equal(t, "fish", detectShell("/opt/homebrew/bin/fish"))
	assert.Equal(t, "", detectShell(""))
	assert.Equal(t, "", detectShell("/bin/tcsh"))
}

func TestMaybeShellIntegration(t *testing.T) {
	for _, test := range []struct {
		name, shell, input string
		contains, excludes []string
	}{
		{
			name: "accept zsh", shell: "/bin/zsh", input: "y\n",
			contains: []string{"Shell integration", "~/.zshrc", `eval "$(gmc wt init zsh)"`},
		},
		{
			name: "decline fish", shell: "/usr/bin/fish", input: "n\n",
			contains: []string{"Set up shell integration for fish", "gmc wt init --help"},
			excludes: []string{"Add this to your"},
		},
		{name: "unknown shell EOF", contains: []string{"gmc wt init --help"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			readLine := newTrimmedLineReader(strings.NewReader(test.input))
			assert.NoError(t, maybeShellIntegration(&output, readLine, test.shell))
			for _, text := range test.contains {
				assert.Contains(t, output.String(), text)
			}
			for _, text := range test.excludes {
				assert.NotContains(t, output.String(), text)
			}
		})
	}
}
