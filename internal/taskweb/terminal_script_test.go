package taskweb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComposeShellCommand(t *testing.T) {
	assert.Equal(t, "gmc task attach t-1", composeShellCommand("", "gmc task attach t-1"))
	assert.Equal(t, "cd '/tmp/wt' && gmc task attach t-1", composeShellCommand("/tmp/wt", "gmc task attach t-1"))
	assert.Equal(t,
		"cd '/tmp/o'\\''reilly' && gmc task attach t-1",
		composeShellCommand("/tmp/o'reilly", "gmc task attach t-1"),
	)
}

func TestGhosttyScript(t *testing.T) {
	script := ghosttyScript("/repo", "/bin/gmc task attach t-1")
	assert.Contains(t, script, `tell application "Ghostty"`)
	assert.Contains(t, script, `set initial working directory of cfg to "/repo"`)
	assert.Contains(t, script, `input text "/bin/gmc task attach t-1" & return to t`)
	assert.NotContains(t, script, `set command of cfg`)
}

func TestITermScript(t *testing.T) {
	script := itermScript("gmc task attach t-1")
	assert.Contains(t, script, `tell application "iTerm"`)
	assert.Contains(t, script, `write text "gmc task attach t-1"`)
}

func TestTerminalAppScript(t *testing.T) {
	script := terminalAppScript("gmc task attach t-1")
	assert.Contains(t, script, `tell application "Terminal"`)
	assert.Contains(t, script, `do script "gmc task attach t-1"`)
}

func TestAppleScriptStringEscapesQuotes(t *testing.T) {
	got := appleScriptString(`say "hi"`)
	assert.True(t, strings.Contains(got, `\"`))
}
