package rustcache

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrapperExecution(t *testing.T) {
	if os.Getenv("GMC_WRAPPER_TEST_CHILD") == "1" {
		err := RunWrapper([]string{os.Getenv("GMC_WRAPPER_TEST_COMPILER"), "argument with spaces"})
		require.NoError(t, err)
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix process replacement and executable fixture")
	}
	root := t.TempDir()
	adapter := filepath.Join(root, "adapter")
	require.NoError(t, os.Mkdir(adapter, 0700))
	exe, err := os.Executable()
	require.NoError(t, err)
	data, err := os.ReadFile(exe)
	require.NoError(t, err)
	helper := filepath.Join(adapter, "test-wrapper")
	require.NoError(t, os.WriteFile(helper, data, 0755))
	compiler := filepath.Join(root, "rustc")
	require.NoError(t, os.WriteFile(compiler, []byte("#!/bin/sh\nprintf 'native:%s' \"$1\"\n"), 0755))
	backend := filepath.Join(root, "kache")
	script := "#!/bin/sh\nprintf 'cache:%s:%s:%s' " +
		"\"${KACHE_REMOTE_ENDPOINT-unset}\" \"${kache_remote_endpoint-unset}\" \"$2\"\nexit 7\n"
	require.NoError(t, os.WriteFile(backend, []byte(script), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), nil, 0600))
	t.Setenv("GMC_WRAPPER_TEST_CHILD", "1")
	t.Setenv("GMC_WRAPPER_TEST_COMPILER", compiler)
	t.Setenv("KACHE_REMOTE_ENDPOINT", "must-not-reach-backend")
	t.Setenv("kache_remote_endpoint", "must-not-reach-backend")
	command := func() *exec.Cmd { return exec.Command(helper, "-test.run=^TestWrapperExecution$") }
	out, err := command().CombinedOutput()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 7, exitErr.ExitCode())
	require.Equal(t, "cache:unset:unset:argument with spaces", string(out))
	require.NoError(t, os.Remove(backend))
	out, err = command().CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "native:argument with spaces", string(out))
	require.NoError(t, os.WriteFile(backend, []byte(script), 0644))
	out, err = command().CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "native:argument with spaces", string(out))
	require.NoError(t, os.Chmod(backend, 0755))
	t.Setenv("CARGO_PRIMARY_PACKAGE", "1")
	out, err = command().CombinedOutput()
	require.NoError(t, err)
	require.Equal(t, "native:argument with spaces", string(out))
}
