//go:build unix

package task

import (
	"os"
	"os/exec"
	"syscall"
)

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
