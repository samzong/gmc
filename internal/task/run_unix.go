//go:build unix

package task

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

var processAlive = func(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func runNotifySignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE}
}

func signaledExit(err *exec.ExitError) (code int, name string, ok bool) {
	ws, isWaitStatus := err.Sys().(syscall.WaitStatus)
	if !isWaitStatus || !ws.Signaled() {
		return 0, "", false
	}
	sig := ws.Signal()
	return 128 + int(sig), sig.String(), true
}
