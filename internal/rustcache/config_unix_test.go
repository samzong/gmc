//go:build !windows

package rustcache

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExclusiveWriteFailure(t *testing.T) {
	if path := os.Getenv("GMC_TEST_WRITE_LIMIT"); path != "" {
		signal.Ignore(syscall.SIGXFSZ)
		var previous syscall.Rlimit
		require.NoError(t, syscall.Getrlimit(syscall.RLIMIT_FSIZE, &previous))
		defer func() { require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &previous)) }()
		limit := previous
		limit.Cur = 512
		require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit))
		require.Error(t, exclusiveWrite(path, bytes.Repeat([]byte("x"), 4096)))
		require.NoFileExists(t, path)
		return
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(exe, "-test.run=^TestExclusiveWriteFailure$")
	command.Env = append(os.Environ(), "GMC_TEST_WRITE_LIMIT="+filepath.Join(t.TempDir(), "state"))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}
