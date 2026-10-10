package cmd

import (
	"io"

	"github.com/samzong/gmc/internal/worktree"
)

func printWorktreeReport(report worktree.Report) {
	printWorktreeReportTo(report, outWriter())
}

func printWorktreeReportTo(report worktree.Report, infoOut io.Writer) {
	for _, event := range report.Events {
		if event.Level == worktree.EventWarn {
			_, _ = errWriter().Write([]byte(event.Message + "\n"))
			continue
		}
		_, _ = infoOut.Write([]byte(event.Message + "\n"))
	}
}
