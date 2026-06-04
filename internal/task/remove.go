package task

import (
	"fmt"
	"path/filepath"

	"github.com/samzong/gmc/internal/worktree"
)

// RemoveOptions configures task removal.
type RemoveOptions struct {
	// Archive marks the task archived in the ledger (default).
	Archive bool
	// Force deletes the ledger directory and best-effort runtime cleanup.
	Force bool
}

// Remove archives or deletes a task.
func (e *Engine) Remove(taskRef string, opts RemoveOptions) error {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return err
	}
	if err := e.validateRemove(taskID, opts.Force); err != nil {
		return err
	}
	if opts.Force {
		return e.forceRemoveTask(taskID)
	}
	return e.archiveTask(taskID)
}

func (e *Engine) validateRemove(taskID string, force bool) error {
	attempts, err := e.store.ListAttempts(taskID)
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		if attempt.State != AttemptHumanAttached {
			continue
		}
		if force {
			continue
		}
		return fmt.Errorf(
			"attempt %s is human-attached; run 'gmc task detach %s' first, or 'gmc task rm %s --force' to delete anyway",
			attempt.ID, taskID, taskID,
		)
	}
	return nil
}

func (e *Engine) archiveTask(taskID string) error {
	task, err := e.store.LoadTask(taskID)
	if err != nil {
		return err
	}
	task.State = TaskArchived
	if err := e.store.writeTask(task); err != nil {
		return err
	}
	attempts, err := e.store.ListAttempts(taskID)
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		if attempt.State == AttemptArchived || attempt.State == AttemptPromoted {
			continue
		}
		attempt.State = AttemptArchived
		if err := e.store.SaveAttempt(attempt); err != nil {
			return err
		}
	}
	return e.store.appendEvent(taskID, Event{
		Type: EventTaskArchived,
		Payload: map[string]string{
			"task_id": taskID,
		},
	})
}

func (e *Engine) forceRemoveTask(taskID string) error {
	attempts, err := e.store.ListAttempts(taskID)
	if err != nil {
		return err
	}
	for i := range attempts {
		if attempts[i].State == AttemptHumanAttached {
			attempts[i].State = AttemptRunning
			_ = e.store.SaveAttempt(attempts[i])
			_ = e.store.appendEvent(taskID, Event{
				Type:    EventSessionDetached,
				Payload: map[string]string{"attempt_id": attempts[i].ID, "reason": "force-rm"},
			})
		}
	}
	var firstErr error
	for _, attempt := range attempts {
		if err := e.cleanupAttemptRuntime(attempt); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := e.store.RemoveTaskDir(taskID); err != nil {
		return err
	}
	return firstErr
}

func (e *Engine) cleanupAttemptRuntime(attempt AttemptRecord) error {
	if attempt.TmuxSession != "" {
		profile, err := TmuxProfileForAttempt(attempt)
		if err == nil && tmuxHasSession(profile) {
			if err := KillTmuxSession(profile); err != nil {
				return fmt.Errorf("kill tmux session %s: %w", attempt.TmuxSession, err)
			}
		}
	}
	if attempt.Worktree == "" {
		return nil
	}
	name := filepath.Base(attempt.Worktree)
	_, err := e.wt.Remove(name, worktree.RemoveOptions{
		Force:        true,
		DeleteBranch: false,
		DryRun:       false,
	})
	if err != nil {
		return fmt.Errorf("remove worktree %s: %w", name, err)
	}
	return nil
}
