package task

// ReconcileAttempt updates attempt state from external runtime signals.
func (e *Engine) ReconcileAttempt(taskID string, attempt AttemptRecord) (AttemptRecord, error) {
	changed := false
	if attempt.TmuxSession != "" && !tmuxHasSessionForAttempt(attempt) {
		if attempt.State == AttemptRunning || attempt.State == AttemptHumanAttached {
			attempt.State = AttemptLost
			changed = true
		}
	}
	if attempt.Worktree != "" {
		if !worktreeExists(attempt.Worktree) {
			if attempt.State != AttemptArchived && attempt.State != AttemptPromoted {
				attempt.State = AttemptLost
				changed = true
			}
		}
	}
	if changed {
		if err := e.store.SaveAttempt(attempt); err != nil {
			return attempt, err
		}
		_ = e.store.appendEvent(taskID, Event{
			Type: EventGCOrphanDetected,
			Payload: map[string]string{
				"attempt_id": attempt.ID,
				"state":      attempt.State,
			},
		})
	}
	return attempt, nil
}

// ReconcileTask refreshes all attempts for a task.
func (e *Engine) ReconcileTask(taskRef string) error {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return err
	}
	attempts, err := e.store.ListAttempts(taskID)
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		if _, err := e.ReconcileAttempt(taskID, attempt); err != nil {
			return err
		}
	}
	return e.refreshTaskState(taskID)
}

func (e *Engine) refreshTaskState(taskID string) error {
	task, err := e.store.LoadTask(taskID)
	if err != nil {
		return err
	}
	attempts, err := e.store.ListAttempts(taskID)
	if err != nil {
		return err
	}
	if len(attempts) == 0 {
		return nil
	}
	next := taskStateFromAttempts(task.State, attempts)
	if next != task.State {
		task.State = next
		return e.store.writeTask(task)
	}
	return nil
}

func tmuxHasSessionForAttempt(attempt AttemptRecord) bool {
	profile, err := TmuxProfileForAttempt(attempt)
	if err != nil {
		return false
	}
	if tmuxHasSession(profile) {
		return true
	}
	if profile.Socket != "" {
		return tmuxHasSession(TmuxProfile{Session: attempt.TmuxSession})
	}
	return false
}

// taskStateFromAttempts derives running vs needs-human from attempt states only.
// Other task states (reviewing, done, …) are left unchanged unless a human gate applies.
func taskStateFromAttempts(current string, attempts []AttemptRecord) string {
	needsHuman := false
	hasActive := false
	for _, a := range attempts {
		switch a.State {
		case AttemptHumanAttached, AttemptWaitingHuman:
			needsHuman = true
		case AttemptRunning, AttemptCreated:
			hasActive = true
		}
	}
	if needsHuman {
		return TaskNeedsHuman
	}
	if current == TaskNeedsHuman && hasActive {
		return TaskRunning
	}
	return current
}
