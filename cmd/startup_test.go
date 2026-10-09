package cmd

import (
	runtimedebug "runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoTerminalQueryingDependencyAtStartup(t *testing.T) {
	info, ok := runtimedebug.ReadBuildInfo()
	require.True(t, ok)
	for _, dep := range info.Deps {
		assert.NotEqual(t, "github.com/charmbracelet/bubbletea", dep.Path)
		assert.NotEqual(t, "github.com/muesli/termenv", dep.Path)
	}
}
