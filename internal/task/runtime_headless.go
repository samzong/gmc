package task

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// HeadlessResult captures a finished headless command run.
type HeadlessResult struct {
	ExitCode int
	LogPath  string
}

// RunHeadless executes command in workdir and writes combined output to logPath.
func RunHeadless(workdir string, command []string, logPath string) (HeadlessResult, error) {
	if len(command) == 0 {
		return HeadlessResult{}, errors.New("empty command")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return HeadlessResult{}, err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return HeadlessResult{}, err
	}
	defer logFile.Close()

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = workdir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	_, _ = fmt.Fprintf(logFile, "# command: %s\n# workdir: %s\n# started: %s\n\n",
		shellJoin(command), workdir, time.Now().UTC().Format(time.RFC3339))

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
			_, _ = io.WriteString(logFile, "\n# error: "+err.Error()+"\n")
		}
	}
	_, _ = fmt.Fprintf(logFile, "\n# finished: %s exit=%d\n", time.Now().UTC().Format(time.RFC3339), exitCode)
	return HeadlessResult{ExitCode: exitCode, LogPath: logPath}, nil
}
