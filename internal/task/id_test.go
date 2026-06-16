package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTaskID(t *testing.T) {
	id := NewTaskID(time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC))
	assert.Contains(t, id, "t-20260614-120000")
}

func TestDeriveTitle(t *testing.T) {
	assert.Equal(t, "Issue #76", DeriveTitle("76", "", "76"))
	assert.Equal(t, "todo.md", DeriveTitle("body", "/tmp/todo.md", ""))
	assert.Equal(t, "Fix the bug", DeriveTitle("Fix the bug\n\nmore", "", ""))
}

func TestWorktreeNames(t *testing.T) {
	assert.Equal(t, "task-t-20260614-120000-abcd-1", WorktreeDirName("t-20260614-120000-abcd", "attempt-1"))
	assert.Equal(t, "_task/t-20260614-120000-abcd/1", WorktreeBranchName("t-20260614-120000-abcd", "attempt-1"))
}

func TestTmuxSessionNameWithWorkflowNode(t *testing.T) {
	assert.Equal(t, "gmc-t-20260614-120000-abcd-attempt-1", TmuxSessionName("t-20260614-120000-abcd", "attempt-1"))
	assert.Equal(t,
		"gmc-t-20260614-120000-abcd-attempt-1-plan-1",
		TmuxSessionName("t-20260614-120000-abcd", "attempt-1", "plan", "1"),
	)
	assert.Equal(t,
		"gmc-t-20260614-120000-abcd-attempt-1-code-2",
		TmuxSessionName("t-20260614-120000-abcd", "attempt-1", "code", "2"),
	)
}
