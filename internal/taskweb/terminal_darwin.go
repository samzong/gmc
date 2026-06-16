//go:build darwin

package taskweb

import (
	"fmt"
	"os/exec"
	"strings"
)

func LaunchTerminal(terminal, command, workdir string) error {
	command = strings.TrimSpace(command)
	var script string
	switch strings.TrimSpace(terminal) {
	case "ghostty":
		script = ghosttyScript(workdir, command)
	case "iterm":
		script = itermScript(composeShellCommand(workdir, command))
	case "terminal":
		script = terminalAppScript(composeShellCommand(workdir, command))
	default:
		return fmt.Errorf("unsupported terminal %q", terminal)
	}
	return runAppleScript(script)
}

func runAppleScript(script string) error {
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
