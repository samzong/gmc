package task

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/worktree"
)

// Engine orchestrates task ledger operations and runtimes.
type Engine struct {
	store *Store
	wt    *worktree.Client
}

func NewEngine(store *Store, wt *worktree.Client) *Engine {
	return &Engine{store: store, wt: wt}
}

// CreateTask registers a new task from issue number or free text.
func (e *Engine) CreateTask(source string) (Record, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return Record{}, errors.New("task source is required")
	}
	now := time.Now().UTC()
	rec := Record{
		ID:        NewTaskID(now),
		Title:     DeriveTitle(source),
		State:     TaskIntake,
		Source:    source,
		Issue:     ParseIssueNumber(source),
		PR:        ParsePullNumber(source),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := e.store.CreateTask(rec); err != nil {
		return Record{}, err
	}
	_ = e.store.appendEvent(rec.ID, Event{Type: EventTaskCreated, Payload: map[string]string{
		"source": source,
		"title":  rec.Title,
		"issue":  rec.Issue,
		"pr":     rec.PR,
	}})
	return rec, nil
}

// ListTasks returns summaries for all tasks.
func (e *Engine) ListTasks(refresh bool) ([]Summary, error) {
	if refresh {
		ids, err := e.store.ListTaskIDs()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			_ = e.ReconcileTask(id)
		}
	}
	summaries, err := e.store.ListSummaries()
	if err != nil {
		return nil, err
	}
	return enrichSummaries(summaries), nil
}

// ShowTask returns one task summary, optionally after refresh.
func (e *Engine) ShowTask(taskRef string, refresh bool) (Summary, error) {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return Summary{}, err
	}
	if refresh {
		if err := e.ReconcileTask(taskID); err != nil {
			return Summary{}, err
		}
	}
	sum, err := e.store.LoadSummary(taskID)
	if err != nil {
		return Summary{}, err
	}
	return enrichSummary(sum), nil
}

func enrichSummaries(list []Summary) []Summary {
	out := make([]Summary, len(list))
	for i, sum := range list {
		out[i] = enrichSummary(sum)
	}
	return out
}

func enrichSummary(sum Summary) Summary {
	if len(sum.Attempts) == 0 {
		return sum
	}
	attempt := sum.Attempts[len(sum.Attempts)-1]
	inspect := InspectSessionRuntime(attempt, sum.Runs)
	sum.RuntimeStatus = inspect.Status
	sum.RuntimeMessage = inspect.UserMessage
	if r := LatestStageResult(sum.Runs); r != nil {
		res := &StageResult{
			RunID:        r.ID,
			RunType:      r.Type,
			State:        r.State,
			ArtifactFile: r.ArtifactFile,
			Summary:      r.Summary,
			LogFile:      r.LogFile,
		}
		if r.ExitCode != nil {
			res.ExitCode = *r.ExitCode
		}
		sum.LastResult = res
	}
	return sum
}

// SessionTarget selects coding vs review tmux for watch/attach.
type SessionTarget string

const (
	SessionCoding SessionTarget = "coding"
	SessionReview SessionTarget = "review"
)

func (e *Engine) resolveTmuxProfile(task Record, attempt AttemptRecord, target SessionTarget) (TmuxProfile, error) {
	useReview := target == SessionReview ||
		(target == "" && (task.State == TaskReviewing || task.State == TaskVerifying) && attempt.ReviewTmuxSession != "")
	if useReview && attempt.ReviewTmuxSession != "" {
		return TmuxProfileForReview(attempt)
	}
	return TmuxProfileForAttempt(attempt)
}

// StartOptions configures gmc task start.
type StartOptions struct {
	TaskID     string
	Agent      string
	Model      string
	Mode       string
	BaseBranch string
}

// Start creates the first attempt, worktree, and tmux agent session (Phase 1).
func (e *Engine) Start(opts StartOptions) (Summary, error) {
	taskID, err := e.store.ResolveTaskID(opts.TaskID)
	if err != nil {
		return Summary{}, err
	}
	opts.TaskID = taskID
	task, err := e.store.LoadTask(opts.TaskID)
	if err != nil {
		return Summary{}, err
	}
	attempts, err := e.store.ListAttempts(opts.TaskID)
	if err != nil {
		return Summary{}, err
	}
	if task.State != TaskIntake && task.State != TaskPlanning {
		return Summary{}, fmt.Errorf("task %s is %s; start only from intake", opts.TaskID, task.State)
	}
	if len(attempts) > 0 {
		return Summary{}, fmt.Errorf("task %s already has attempts; Phase 1 supports one attempt per task", opts.TaskID)
	}

	attemptID := NewAttemptID(0)
	wtDir := WorktreeDirName(opts.TaskID, attemptID)
	wtBranch := WorktreeBranchName(opts.TaskID, attemptID)
	addOpts := worktree.AddOptions{BaseBranch: opts.BaseBranch, Branch: wtBranch}
	report, err := e.wt.Add(wtDir, addOpts)
	if err != nil {
		return Summary{}, fmt.Errorf("create worktree: %w", err)
	}
	_ = report

	worktrees, err := e.wt.List()
	if err != nil {
		return Summary{}, err
	}
	var wtPath, branch string
	for _, info := range worktrees {
		if filepath.Base(info.Path) == wtDir {
			wtPath = info.Path
			branch = info.Branch
			break
		}
	}
	if wtPath == "" {
		return Summary{}, fmt.Errorf("worktree %q not found after creation", wtDir)
	}

	now := time.Now().UTC()
	previewAttempt := AttemptRecord{
		ID:       attemptID,
		TaskID:   opts.TaskID,
		Agent:    opts.Agent,
		Model:    opts.Model,
		Mode:     opts.Mode,
		Branch:   branch,
		Worktree: wtPath,
	}
	contextFile, err := WriteTaskContextFile(wtPath, task, previewAttempt)
	if err != nil {
		return Summary{}, fmt.Errorf("write task context: %w", err)
	}

	prompt := InitialAgentPrompt(task)
	command, err := AgentCommand(opts.Agent, opts.Model, opts.Mode, prompt)
	if err != nil {
		return Summary{}, err
	}

	session := TmuxSessionName(opts.TaskID, attemptID)
	tmuxProfile, err := StartTmuxSession(session, wtPath, command)
	if err != nil {
		return Summary{}, err
	}

	attempt := AttemptRecord{
		ID:          attemptID,
		TaskID:      opts.TaskID,
		State:       AttemptRunning,
		Worktree:    wtPath,
		Branch:      branch,
		Agent:       opts.Agent,
		Model:       opts.Model,
		Mode:        opts.Mode,
		TmuxSession: tmuxProfile.Session,
		TmuxSocket:  tmuxProfile.Socket,
		ContextFile: contextFile,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := e.store.SaveAttempt(attempt); err != nil {
		return Summary{}, err
	}
	_ = e.store.appendEvent(opts.TaskID, Event{
		Type: EventAttemptCreated,
		Payload: map[string]string{
			"attempt_id":   attemptID,
			"worktree":     wtPath,
			"context_file": contextFile,
		},
	})

	runs, _ := e.store.ListRuns(opts.TaskID)
	runID := NewRunID(now, len(runs)+1)
	run := RunRecord{
		ID:          runID,
		TaskID:      opts.TaskID,
		AttemptID:   attemptID,
		Type:        RunTypeAgentSession,
		Runtime:     RuntimeTmux,
		State:       RunRunning,
		Command:     command,
		Workdir:     wtPath,
		TmuxSession: session,
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := e.store.SaveRun(run); err != nil {
		return Summary{}, err
	}
	_ = e.store.appendEvent(opts.TaskID, Event{
		Type:    EventRunStarted,
		Payload: map[string]string{"run_id": runID, "attempt_id": attemptID},
	})

	task.State = TaskRunning
	if err := e.store.writeTask(task); err != nil {
		return Summary{}, err
	}
	return e.store.LoadSummary(opts.TaskID)
}

// RunOptions runs a headless command inside an attempt worktree.
type RunOptions struct {
	TaskID    string
	AttemptID string
	Command   []string
	RunType   string
}

// Run executes a headless command and records the run.
func (e *Engine) Run(opts RunOptions) (RunRecord, error) {
	taskID, err := e.store.ResolveTaskID(opts.TaskID)
	if err != nil {
		return RunRecord{}, err
	}
	opts.TaskID = taskID
	attempt, err := e.resolveAttempt(opts.TaskID, opts.AttemptID)
	if err != nil {
		return RunRecord{}, err
	}
	if attempt.Worktree == "" {
		return RunRecord{}, fmt.Errorf("attempt %s has no worktree", attempt.ID)
	}
	runType := opts.RunType
	if runType == "" {
		runType = RunTypeCommandCheck
	}

	runs, err := e.store.ListRuns(opts.TaskID)
	if err != nil {
		return RunRecord{}, err
	}
	now := time.Now().UTC()
	runID := NewRunID(now, len(runs)+1)
	logPath := e.store.LogPath(opts.TaskID, runID)

	run := RunRecord{
		ID:        runID,
		TaskID:    opts.TaskID,
		AttemptID: attempt.ID,
		Type:      runType,
		Runtime:   RuntimeHeadless,
		State:     RunRunning,
		Command:   opts.Command,
		Workdir:   attempt.Worktree,
		LogFile:   logPath,
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := e.store.SaveRun(run); err != nil {
		return RunRecord{}, err
	}
	_ = e.store.appendEvent(opts.TaskID, Event{
		Type:    EventRunStarted,
		Payload: map[string]string{"run_id": runID},
	})

	result, err := RunHeadless(attempt.Worktree, opts.Command, logPath)
	finished := time.Now().UTC()
	run.FinishedAt = finished
	run.ExitCode = &result.ExitCode
	if result.ExitCode == 0 {
		run.State = RunPassed
		_ = e.store.appendEvent(opts.TaskID, Event{Type: EventRunCompleted, Payload: map[string]string{"run_id": runID}})
	} else {
		run.State = RunFailed
		_ = e.store.appendEvent(opts.TaskID, Event{Type: EventRunFailed, Payload: map[string]string{
			"run_id":    runID,
			"exit_code": strconv.Itoa(result.ExitCode),
		}})
	}
	if err != nil && result.ExitCode != 0 {
		run.State = RunFailed
	}
	run.UpdatedAt = finished
	if saveErr := e.store.SaveRun(run); saveErr != nil {
		return run, saveErr
	}
	return run, nil
}

// Watch shows tmux session output read-only without updating task or attempt state.
func (e *Engine) Watch(taskRef, attemptID string, lines int, session SessionTarget) error {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return err
	}
	attempt, err := e.resolveAttempt(taskID, attemptID)
	if err != nil {
		return err
	}
	if attempt.TmuxSession == "" {
		return fmt.Errorf("attempt %s has no tmux session", attempt.ID)
	}
	task, err := e.store.LoadTask(taskID)
	if err != nil {
		return err
	}
	profile, err := e.resolveTmuxProfile(task, attempt, session)
	if err != nil {
		return err
	}
	runs, _ := e.store.ListRuns(taskID)
	inspect := InspectSessionRuntime(attempt, runs)
	return WatchTmuxSession(profile, lines, WatchHints{
		Title:          DisplayTitle(task),
		RuntimeMessage: inspect.UserMessage,
	})
}

// Attach connects the terminal to the attempt tmux session for human intervention.
func (e *Engine) Attach(taskRef, attemptID string, session SessionTarget) error {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return err
	}
	attempt, err := e.resolveAttempt(taskID, attemptID)
	if err != nil {
		return err
	}
	if attempt.TmuxSession == "" {
		return fmt.Errorf("attempt %s has no tmux session", attempt.ID)
	}
	attempt.State = AttemptHumanAttached
	if err := e.store.SaveAttempt(attempt); err != nil {
		return err
	}
	task, err := e.store.LoadTask(taskID)
	if err != nil {
		return err
	}
	task.State = TaskNeedsHuman
	if err := e.store.writeTask(task); err != nil {
		return err
	}
	contextPath, _ := e.EnsureTaskContextFile(taskID, attempt)
	_ = e.store.appendEvent(taskID, Event{
		Type:    EventSessionAttached,
		Payload: map[string]string{"attempt_id": attempt.ID},
	})
	runs, _ := e.store.ListRuns(taskID)
	inspect := InspectSessionRuntime(attempt, runs)
	profile, err := e.resolveTmuxProfile(task, attempt, session)
	if err != nil {
		return err
	}
	return AttachTmuxSession(profile, AttachHints{
		Title:          DisplayTitle(task),
		ContextPath:    contextPath,
		Prompt:         InitialAgentPrompt(task),
		RuntimeMessage: inspect.UserMessage,
		RuntimeStatus:  inspect.Status,
	})
}

// AttachHints is shown before connecting to tmux (stderr).
type AttachHints struct {
	Title          string
	ContextPath    string
	Prompt         string
	RuntimeMessage string
	RuntimeStatus  RuntimeStatus
}

// EnsureTaskContextFile writes .gmc/TASK.md when missing (e.g. tasks started before prompt injection).
func (e *Engine) EnsureTaskContextFile(taskID string, attempt AttemptRecord) (string, error) {
	if attempt.Worktree == "" {
		return "", nil
	}
	existing := attempt.ContextFile
	if existing == "" {
		existing = filepath.Join(attempt.Worktree, TaskContextRelPath)
	}
	if _, err := os.Stat(existing); err == nil {
		return existing, nil
	}
	task, err := e.store.LoadTask(taskID)
	if err != nil {
		return "", err
	}
	path, err := WriteTaskContextFile(attempt.Worktree, task, attempt)
	if err != nil {
		return "", err
	}
	attempt.ContextFile = path
	_ = e.store.SaveAttempt(attempt)
	return path, nil
}

// Detach marks the attempt as no longer human-attached (tmux detach is manual).
func (e *Engine) Detach(taskRef, attemptID string) error {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return err
	}
	attempt, err := e.resolveAttempt(taskID, attemptID)
	if err != nil {
		return err
	}
	if attempt.State == AttemptHumanAttached {
		attempt.State = AttemptRunning
		if err := e.store.SaveAttempt(attempt); err != nil {
			return err
		}
	}
	_ = e.store.appendEvent(taskID, Event{
		Type:    EventSessionDetached,
		Payload: map[string]string{"attempt_id": attempt.ID},
	})
	return e.refreshTaskState(taskID)
}

func (e *Engine) resolveAttempt(taskID, attemptID string) (AttemptRecord, error) {
	if attemptID != "" {
		return e.store.LoadAttempt(taskID, attemptID)
	}
	attempts, err := e.store.ListAttempts(taskID)
	if err != nil {
		return AttemptRecord{}, err
	}
	if len(attempts) == 0 {
		return AttemptRecord{}, fmt.Errorf("task %s has no attempts", taskID)
	}
	if len(attempts) > 1 {
		return AttemptRecord{}, fmt.Errorf("task %s has multiple attempts; pass --attempt", taskID)
	}
	return attempts[0], nil
}

func gitStatusPorcelain(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
