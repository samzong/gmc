package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/samzong/gmc/cmd"
	"github.com/samzong/gmc/internal/exitcode"
	"github.com/samzong/gmc/internal/rustcache"
)

func main() {
	if rustcache.IsWrapper() {
		if err := rustcache.RunWrapper(os.Args[1:]); err != nil {
			var processErr *exec.ExitError
			if errors.As(err, &processErr) {
				os.Exit(processErr.ExitCode())
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := cmd.Execute(); err != nil {
		var exitErr *exitcode.Error
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(exitcode.General)
	}
}
