package main

import (
	"os"

	"github.com/samzong/gmc/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
