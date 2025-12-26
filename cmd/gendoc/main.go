//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/samzong/gmc/cmd"
	"github.com/spf13/cobra/doc"
)

func main() {
	dir := "./docs/man"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	header := &doc.GenManHeader{
		Title:   "GMC",
		Section: "1",
		Source:  "gmc",
		Manual:  "GMC Manual",
	}

	rootCmd := cmd.RootCmd()
	if err := doc.GenManTree(rootCmd, header, dir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating man pages: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Man pages generated in %s\n", dir)
}
