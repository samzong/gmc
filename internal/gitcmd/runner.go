package gitcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Runner struct {
	Verbose bool
	Dir     string
	Env     []string
	Logger  io.Writer
}

type Result struct {
	Stdout []byte
	Stderr []byte
}

func (r Result) StdoutString(trim bool) string {
	output := string(r.Stdout)
	if trim {
		return strings.TrimSpace(output)
	}
	return output
}

func (r Result) StderrString(trim bool) string {
	output := string(r.Stderr)
	if trim {
		return strings.TrimSpace(output)
	}
	return output
}

func (r Runner) command(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}
	return cmd
}

func (r Runner) log(args []string) {
	if !r.Verbose {
		return
	}
	logger := r.Logger
	if logger == nil {
		logger = os.Stderr
	}
	fmt.Fprintf(logger, "Running: git %s\n", strings.Join(args, " "))
}

func (r Runner) prepare(args []string, log bool) *exec.Cmd {
	if log {
		r.log(args)
	}
	return r.command(args...)
}

func (r Runner) Run(args ...string) (Result, error) {
	return r.run(args, false)
}

func (r Runner) RunLogged(args ...string) (Result, error) {
	return r.run(args, true)
}

func (r Runner) RunStreamingLogged(args ...string) error {
	cmd := r.prepare(args, true)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r Runner) run(args []string, log bool) (Result, error) {
	cmd := r.prepare(args, log)
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	return Result{Stdout: outBuf.Bytes(), Stderr: errBuf.Bytes()}, err
}
