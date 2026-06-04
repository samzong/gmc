package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// AdvanceOptions configures a stage transition.
type AdvanceOptions struct {
	TaskID  string
	ToState string // empty = default next state
}

// Advance moves the task along the state machine and runs stage tools when needed.
func (e *Engine) Advance(opts AdvanceOptions) (Summary, error) {
	taskID, err := e.store.ResolveTaskID(opts.TaskID)
	if err != nil {
		return Summary{}, err
	}
	task, err := e.store.LoadTask(taskID)
	if err != nil {
		return Summary{}, err
	}
	attempt, err := e.resolveAttempt(taskID, "")
	if err != nil {
		return Summary{}, err
	}

	target := opts.ToState
	if target == "" {
		var ok bool
		target, ok = NextTaskState(task.State)
		if !ok {
			return Summary{}, fmt.Errorf("task %s is %s; cannot advance (use start from intake)", taskID, task.State)
		}
	}

	fromState := task.State
	switch task.State {
	case TaskRunning:
		if target != TaskReviewing {
			return Summary{}, fmt.Errorf("from running only advancing to reviewing is supported (got %s)", target)
		}
		run, err := e.runReviewStage(task, attempt)
		if err != nil {
			return e.store.LoadSummary(taskID)
		}
		if run.State != RunPassed {
			return Summary{}, fmt.Errorf(
				"review run %s failed (exit %d); see log %s",
				run.ID, derefExit(run.ExitCode), run.LogFile,
			)
		}
		task.State = TaskReviewing
	case TaskReviewing:
		if target != TaskVerifying {
			return Summary{}, fmt.Errorf("from reviewing only advancing to verifying is supported (got %s)", target)
		}
		task.State = TaskVerifying
	case TaskVerifying:
		if target != TaskReadyForPR {
			return Summary{}, fmt.Errorf("from verifying only advancing to ready-for-pr is supported (got %s)", target)
		}
		run, err := e.runVerifyStage(task, attempt)
		if err != nil {
			return e.store.LoadSummary(taskID)
		}
		if run.State != RunPassed {
			return Summary{}, fmt.Errorf(
				"verify run %s failed (exit %d); see log %s",
				run.ID, derefExit(run.ExitCode), run.LogFile,
			)
		}
		task.State = TaskReadyForPR
	case TaskReadyForPR:
		if target != TaskDone {
			return Summary{}, fmt.Errorf("from ready-for-pr only advancing to done is supported (got %s)", target)
		}
		task.State = TaskDone
	default:
		return Summary{}, fmt.Errorf("task %s is %s; cannot advance", taskID, task.State)
	}

	if err := e.store.writeTask(task); err != nil {
		return Summary{}, err
	}
	_ = e.store.appendEvent(taskID, Event{
		Type: EventStageAdvanced,
		Payload: map[string]string{
			"from": fromState,
			"to":   task.State,
		},
	})
	return e.store.LoadSummary(taskID)
}

func derefExit(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

func (e *Engine) runReviewStage(task Record, attempt AttemptRecord) (RunRecord, error) {
	cmd, err := ResolveReviewCommand(attempt.Agent, attempt.Model, "")
	if err != nil {
		return RunRecord{}, err
	}
	session := TmuxStageSessionName(task.ID, attempt.ID, "review")
	run, err := e.executeTmuxStage(task.ID, attempt, session, cmd, RunTypeAgentReview)
	if err != nil {
		return run, err
	}
	attempt.ReviewTmuxSession = session
	attempt.ReviewTmuxSocket = gmcTmuxSocket
	attempt.UpdatedAt = time.Now().UTC()
	_ = e.store.SaveAttempt(attempt)
	if run.State == RunPassed {
		_ = e.store.appendEvent(task.ID, Event{
			Type: EventReviewFindingsRecorded,
			Payload: map[string]string{
				"run_id":        run.ID,
				"artifact_file": run.ArtifactFile,
			},
		})
	}
	return run, nil
}

func (e *Engine) runVerifyStage(task Record, attempt AttemptRecord) (RunRecord, error) {
	cmd, err := ResolveVerifyCommand(attempt.Agent)
	if err != nil {
		return RunRecord{}, err
	}
	session := TmuxStageSessionName(task.ID, attempt.ID, "verify")
	return e.executeTmuxStage(task.ID, attempt, session, cmd, RunTypeCommandCheck)
}

func (e *Engine) executeTmuxStage(
	taskID string,
	attempt AttemptRecord,
	session string,
	command []string,
	runType string,
) (RunRecord, error) {
	runs, err := e.store.ListRuns(taskID)
	if err != nil {
		return RunRecord{}, err
	}
	now := time.Now().UTC()
	runID := NewRunID(now, len(runs)+1)
	logPath := e.store.LogPath(taskID, runID)
	exitPath := logPath + ".exit"
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)

	run := RunRecord{
		ID:          runID,
		TaskID:      taskID,
		AttemptID:   attempt.ID,
		Type:        runType,
		Runtime:     RuntimeTmux,
		State:       RunRunning,
		Command:     command,
		Workdir:     attempt.Worktree,
		TmuxSession: session,
		LogFile:     logPath,
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := e.store.SaveRun(run); err != nil {
		return RunRecord{}, err
	}
	_ = e.store.appendEvent(taskID, Event{
		Type:    EventRunStarted,
		Payload: map[string]string{"run_id": runID, "tmux": session},
	})

	exit, err := RunTmuxToCompletion(session, attempt.Worktree, command, logPath, exitPath)
	finished := time.Now().UTC()
	run.FinishedAt = finished
	run.ExitCode = &exit
	if exit == 0 {
		run.State = RunPassed
		_ = e.store.appendEvent(taskID, Event{Type: EventRunCompleted, Payload: map[string]string{"run_id": runID}})
	} else {
		run.State = RunFailed
		_ = e.store.appendEvent(taskID, Event{Type: EventRunFailed, Payload: map[string]string{
			"run_id":    runID,
			"exit_code": strconv.Itoa(exit),
		}})
	}
	if rel, summary, aerr := e.store.WriteRunArtifact(taskID, runID, logPath); aerr == nil {
		run.ArtifactFile = rel
		run.Summary = summary
	}
	run.UpdatedAt = finished
	if saveErr := e.store.SaveRun(run); saveErr != nil {
		return run, saveErr
	}
	if err != nil && exit != 0 {
		return run, err
	}
	return run, nil
}
