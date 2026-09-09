package rustcache

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func IsWrapper() bool {
	return filepath.Base(os.Args[0]) == executableName("gmc-rustc")
}

func RunWrapper(args []string) error {
	if len(args) == 0 {
		return errors.New("missing rustc executable")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(filepath.Dir(exe))
	backend := filepath.Join(dir, executableName("kache"))
	_, primary := os.LookupEnv("CARGO_PRIMARY_PACKAGE")
	if primary {
		return execute(args, os.Environ())
	}
	if info, err := os.Stat(backend); err != nil || !info.Mode().IsRegular() {
		return execute(args, os.Environ())
	}
	if _, err := os.Stat(filepath.Join(dir, "config.toml")); err != nil {
		return execute(args, os.Environ())
	}
	env := make([]string, 0, len(os.Environ())+5)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "KACHE_") {
			env = append(env, value)
		}
	}
	env = append(env,
		"KACHE_CONFIG="+filepath.Join(dir, "config.toml"),
		"KACHE_CACHE_DIR="+filepath.Join(dir, "store"),
		"KACHE_RUNTIME_DIR="+filepath.Join(dir, "runtime"),
		"KACHE_MAX_SIZE=10GiB", "KACHE_DAEMON_IDLE_TIMEOUT=60")
	err = execute(append([]string{backend}, args...), env)
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return execute(args, os.Environ())
	}
	return err
}
