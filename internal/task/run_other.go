//go:build !unix

package task

import (
	"os"
	"os/exec"
)

func runNotifySignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func signaledExit(*exec.ExitError) (int, string, bool) {
	return 0, "", false
}
