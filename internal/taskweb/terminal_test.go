package taskweb

import (
	"runtime"
	"testing"

	"github.com/samzong/gmc/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLaunchTerminalUnsupported(t *testing.T) {
	err := LaunchTerminal("warp", "gmc task attach t-1", "")
	if runtime.GOOS == "darwin" {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported terminal")
		return
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "macOS")
}

func TestNewServerSetsDefaultTerminalLauncher(t *testing.T) {
	engine := task.NewEngine(task.NewStore(t.TempDir()), nil)
	srv, err := New(engine, t.TempDir(), Options{GMCBinary: "/bin/gmc"})
	require.NoError(t, err)
	assert.NotNil(t, srv.opts.TerminalLauncher)
}
