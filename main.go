package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/samzong/gmc/cmd"
)

func main() {
	// Set up signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Pass context to command execution
	cmd.SetContext(ctx)

	if err := cmd.Execute(); err != nil {
		// Check if error was due to context cancellation
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "\nOperation cancelled")
			os.Exit(130) // Standard exit code for SIGINT
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
