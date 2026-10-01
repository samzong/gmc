package worktree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReportMergeDeduplicatesIdenticalWarnings(t *testing.T) {
	var report Report
	report.Warn("skipped a")
	report.Info("synced")

	var other Report
	other.Warn("skipped a")
	other.Info("synced")
	other.Warn("skipped b")
	report.Merge(other)

	assert.Equal(t, []Event{
		{Level: EventWarn, Message: "skipped a"},
		{Level: EventInfo, Message: "synced"},
		{Level: EventInfo, Message: "synced"},
		{Level: EventWarn, Message: "skipped b"},
	}, report.Events)
}
