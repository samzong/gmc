//go:build !darwin

package taskweb

import "errors"

func LaunchTerminal(terminal, command, workdir string) error {
	_ = terminal
	_ = command
	_ = workdir
	return errors.New("terminal launch is only supported on macOS")
}
