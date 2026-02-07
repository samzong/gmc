package cmd

import "github.com/samzong/gmc/internal/worktree"

func printWorktreeReport(report worktree.Report) {
	for _, event := range report.Events {
		if event.Level == worktree.EventWarn {
			_, _ = errWriter().Write([]byte(event.Message + "\n"))
			continue
		}
		_, _ = outWriter().Write([]byte(event.Message + "\n"))
	}
}
