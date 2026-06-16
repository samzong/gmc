package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/samzong/gmc/internal/taskweb"
	"github.com/spf13/cobra"
)

var taskWebuiCmd = &cobra.Command{
	Use:   "webui",
	Short: "Start local kanban WebUI for tasks",
	Long: `Start a local HTTP server with a kanban WebUI for managing tasks in the current repository.

Opens your browser to a drag-and-drop task board. Attach uses Ghostty, iTerm2, or Terminal.app on macOS.`,
	Args:    cobra.NoArgs,
	Example: `  gmc task webui`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return startTaskWebUI(taskweb.Options{})
	},
}

func init() {
	taskCmd.AddCommand(taskWebuiCmd)
}

func startTaskWebUI(opts taskweb.Options) error {
	engine, err := newTaskEngine()
	if err != nil {
		return err
	}
	repoPath, err := os.Getwd()
	if err != nil {
		return err
	}
	srv, err := taskweb.New(engine, repoPath, opts)
	if err != nil {
		return err
	}
	url, preferred, err := srv.ListenLoopback(taskweb.PreferredWebUIPort)
	if err != nil {
		return err
	}
	fmt.Fprintf(errWriter(), "Project: %s\n", srv.ProjectPath())
	fmt.Fprintf(errWriter(), "WebUI:   %s\n", url)
	if !preferred {
		fmt.Fprintf(errWriter(), "Note:    port %d in use, picked an available port\n", taskweb.PreferredWebUIPort)
	}
	fmt.Fprintf(errWriter(), "Press Ctrl-C to stop\n")
	if err := srv.OpenBrowser(); err != nil {
		fmt.Fprintf(errWriter(), "Warning: failed to open browser: %v\n", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		fmt.Fprintf(errWriter(), "\nStopping WebUI (%s)...\n", sig)
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
