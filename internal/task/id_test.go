package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTaskID(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := NewTaskID(now)
	assert.Regexp(t, `^t-20260604-[0-9a-f]{4}$`, id)
}

func TestDeriveTitlePull(t *testing.T) {
	source := "review this pr https://github.com/clawwork-ai/ClawWork/pull/510"
	assert.Equal(t, "Review PR #510 (clawwork-ai/ClawWork)", DeriveTitle(source))
	assert.Equal(t, "510", ParsePullNumber(source))
	assert.Equal(t, "", ParseIssueNumber(source))
}

func TestDeriveTitleIssue(t *testing.T) {
	assert.Equal(t, "Issue #76", DeriveTitle("fix #76"))
	assert.Equal(t, "76", ParseIssueNumber("fix #76"))
}

func TestWorktreeDirName(t *testing.T) {
	name := WorktreeDirName("t-20260603-a1b2", "attempt-1")
	assert.Equal(t, ".task-t-20260603-a1b2-1", name)
}

func TestWorktreeBranchName(t *testing.T) {
	branch := WorktreeBranchName("t-20260603-a1b2", "attempt-1")
	assert.Equal(t, "_task/t-20260603-a1b2/1", branch)
}

func TestDisplayTitleFallback(t *testing.T) {
	rec := Record{Source: "short note"}
	assert.Equal(t, "short note", DisplayTitle(rec))
}
