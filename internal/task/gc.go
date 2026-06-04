package task

import (
	"errors"
	"fmt"
	"os"
)

// GCOptions configures garbage-collection dry-run behavior.
type GCOptions struct {
	DryRun bool
}

// GC performs conservative dry-run garbage collection analysis.
func (e *Engine) GC(opts GCOptions) (GCReport, error) {
	if !opts.DryRun {
		return GCReport{}, errors.New("only --dry-run is supported in Phase 1")
	}
	var report GCReport
	ids, err := e.store.ListTaskIDs()
	if err != nil {
		return report, err
	}
	for _, taskID := range ids {
		task, err := e.store.LoadTask(taskID)
		if err != nil {
			return report, err
		}
		attempts, err := e.store.ListAttempts(taskID)
		if err != nil {
			return report, err
		}
		for _, attempt := range attempts {
			e.gcAttempt(&report, task, attempt)
		}
	}
	return report, nil
}

func (e *Engine) gcAttempt(report *GCReport, task Record, attempt AttemptRecord) {
	if attempt.State == AttemptHumanAttached {
		report.Items = append(report.Items, GCItem{
			Kind:    "session",
			TaskID:  task.ID,
			Detail:  fmt.Sprintf("tmux session %s (human-attached)", attempt.TmuxSession),
			Would:   false,
			Blocked: "human-attached",
		})
		return
	}
	if attempt.State == AttemptRunning || attempt.State == AttemptWaitingHuman {
		report.Items = append(report.Items, GCItem{
			Kind:    "session",
			TaskID:  task.ID,
			Detail:  fmt.Sprintf("tmux session %s (%s)", attempt.TmuxSession, attempt.State),
			Would:   false,
			Blocked: "active attempt",
		})
	}

	if attempt.Worktree != "" {
		item := GCItem{Kind: "worktree", TaskID: task.ID, Detail: attempt.Worktree}
		if !worktreeExists(attempt.Worktree) {
			item.Would = true
			item.Detail += " (missing on disk — would mark lost)"
		} else if status, err := gitStatusPorcelain(attempt.Worktree); err == nil && status != "" {
			item.Would = false
			item.Blocked = "dirty worktree"
		} else if attempt.State == AttemptArchived || attempt.State == AttemptLost {
			item.Would = true
			item.Detail += " (terminal attempt — would remove worktree with --force in a later phase)"
		} else {
			item.Would = false
			item.Blocked = "worktree still active"
		}
		report.Items = append(report.Items, item)
	}

	if attempt.TmuxSession != "" && !tmuxHasSessionForAttempt(attempt) {
		report.Items = append(report.Items, GCItem{
			Kind:    "orphan",
			TaskID:  task.ID,
			Detail:  fmt.Sprintf("ledger references missing tmux session %s", attempt.TmuxSession),
			Would:   false,
			Blocked: "reconcile with gmc task refresh",
		})
		_ = e.store.appendEvent(task.ID, Event{
			Type:    EventGCOrphanDetected,
			Payload: map[string]string{"tmux_session": attempt.TmuxSession},
		})
	}
}

func worktreeExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
