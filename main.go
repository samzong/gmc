package main

import (
	"errors"
	"os"

	"github.com/samzong/gmc/cmd"
	"github.com/samzong/gmc/internal/exitcode"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var exitErr *exitcode.Error
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(exitcode.General)
	}
}
