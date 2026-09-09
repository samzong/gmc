//go:build !windows

package rustcache

import (
	"os/exec"
	"syscall"
)

func execute(args, env []string) error {
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, args, env)
}
