package gitcmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner executes git commands with shared logging and output handling.
type Runner struct {
	Verbose bool
	Dir     string
	Env     []string
	Logger  io.Writer
}

// Result contains captured stdout/stderr for a git command.
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

func (r Runner) withDefaults() Runner {
	if r.Logger == nil {
		r.Logger = os.Stderr
	}
	return r
}

func (r Runner) command(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	if r.Dir != "" {
		cmd.Dir = r.Dir
	}
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}
	return cmd
}

func (r Runner) log(args []string) {
	if !r.Verbose {
		return
	}
	r = r.withDefaults()
	fmt.Fprintf(r.Logger, "Running: git %s\n", strings.Join(args, " "))
}

func (r Runner) prepare(args []string, log bool) *exec.Cmd {
	r = r.withDefaults()
	if log {
		r.log(args)
	}
	return r.command(args...)
}

// Run executes a git command and captures stdout/stderr.
func (r Runner) Run(args ...string) (Result, error) {
	return r.run(args, false)
}

// RunLogged executes a git command, logs when verbose, and captures stdout/stderr.
func (r Runner) RunLogged(args ...string) (Result, error) {
	return r.run(args, true)
}

// RunStreaming executes a git command with stdout/stderr streamed to the terminal.
func (r Runner) RunStreaming(args ...string) error {
	return r.runWithWriters(args, false, os.Stdout, os.Stderr)
}

// RunStreamingLogged executes a git command with stdout/stderr streamed and logs when verbose.
func (r Runner) RunStreamingLogged(args ...string) error {
	return r.runWithWriters(args, true, os.Stdout, os.Stderr)
}

// RunWithWriters executes a git command, optionally logs, and uses provided writers.
func (r Runner) RunWithWriters(log bool, stdout io.Writer, stderr io.Writer, args ...string) error {
	return r.runWithWriters(args, log, stdout, stderr)
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

func (r Runner) runWithWriters(args []string, log bool, stdout io.Writer, stderr io.Writer) error {
	cmd := r.prepare(args, log)
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}

	return cmd.Run()
}
