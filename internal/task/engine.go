package task

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/worktree"
)

type Engine struct {
	store *Store
	wt    *worktree.Client
}

type StartOptions struct {
	TaskID     string
	Agent      string
	Model      string
	Mode       string
	BaseBranch string
}

type RemoveOptions struct {
	Force bool
}

func NewEngine(store *Store, wt *worktree.Client) *Engine {
	return &Engine{store: store, wt: wt}
}

func (e *Engine) CreateTask(input string) (Record, error) {
	source, sourceFile, err := loadTaskSource(input)
	if err != nil {
		return Record{}, err
	}
	issue := ParseIssueNumber(input)
	now := time.Now().UTC()
	rec := Record{
		ID:         NewTaskID(now),
		Title:      DeriveTitle(source, sourceFile, issue),
		State:      TaskNew,
		Source:     source,
		SourceFile: sourceFile,
		Issue:      issue,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := e.store.CreateTask(rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

func (e *Engine) ListTasks() ([]Summary, error) {
	return e.store.ListSummaries()
}

func (e *Engine) ShowTask(taskRef string) (Summary, error) {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return Summary{}, err
	}
	return e.store.LoadSummary(taskID)
}

func (e *Engine) Start(opts StartOptions) (Summary, error) {
	if e.wt == nil {
		return Summary{}, errors.New("worktree client is required")
	}
	taskID, err := e.store.ResolveTaskID(opts.TaskID)
	if err != nil {
		return Summary{}, err
	}
	rec, err := e.store.LoadTask(taskID)
	if err != nil {
		return Summary{}, err
	}
	if rec.State != TaskNew {
		return Summary{}, fmt.Errorf("task %s is %s; start only from new", taskID, rec.State)
	}
	if _, err := e.store.LoadAttempt(taskID); err == nil {
		return Summary{}, fmt.Errorf("task %s already started", taskID)
	}

	attemptID := NewAttemptID()
	wtDir := WorktreeDirName(taskID, attemptID)
	wtBranch := WorktreeBranchName(taskID, attemptID)
	if _, err := e.wt.Add(wtDir, worktree.AddOptions{BaseBranch: opts.BaseBranch, Branch: wtBranch}); err != nil {
		return Summary{}, fmt.Errorf("create worktree: %w", err)
	}

	wtPath, branch, err := e.findWorktree(wtDir, wtBranch)
	if err != nil {
		return Summary{}, err
	}
	attempt := AttemptRecord{
		ID:        attemptID,
		TaskID:    taskID,
		Worktree:  wtPath,
		Branch:    branch,
		Agent:     NormalizeTaskAgent(opts.Agent),
		Model:     opts.Model,
		Mode:      opts.Mode,
		CreatedAt: time.Now().UTC(),
	}
	contextFile, err := WriteTaskContextFile(wtPath, rec, attempt)
	if err != nil {
		return Summary{}, fmt.Errorf("write task brief: %w", err)
	}
	attempt.ContextFile = contextFile

	command, err := AgentCommand(attempt.Agent, attempt.Model, attempt.Mode, InitialAgentPrompt(rec))
	if err != nil {
		return Summary{}, err
	}
	session := TmuxSessionName(taskID, attemptID)
	profile, err := StartTmuxSession(session, wtPath, command)
	if err != nil {
		return Summary{}, err
	}
	attempt.TmuxSession = profile.Session
	attempt.TmuxSocket = profile.Socket
	if err := e.store.SaveAttempt(attempt); err != nil {
		return Summary{}, err
	}

	rec.State = TaskPlan
	if err := e.store.writeTask(rec); err != nil {
		return Summary{}, err
	}
	return e.store.LoadSummary(taskID)
}

func (e *Engine) Mark(taskRef, state string) (Summary, error) {
	state = strings.TrimSpace(state)
	if !ValidState(state) {
		return Summary{}, fmt.Errorf("invalid task state %q (use: %s)", state, strings.Join(StateValues(), ", "))
	}
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return Summary{}, err
	}
	rec, err := e.store.LoadTask(taskID)
	if err != nil {
		return Summary{}, err
	}
	rec.State = state
	if err := e.store.writeTask(rec); err != nil {
		return Summary{}, err
	}
	return e.store.LoadSummary(taskID)
}

func (e *Engine) Attach(taskRef string) error {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return err
	}
	attempt, err := e.store.LoadAttempt(taskID)
	if err != nil {
		return err
	}
	if attempt.TmuxSession == "" {
		return fmt.Errorf("task %s has no tmux session", taskID)
	}
	return AttachTmuxSession(TmuxProfile{Session: attempt.TmuxSession, Socket: attempt.TmuxSocket})
}

func (e *Engine) Remove(taskRef string, opts RemoveOptions) error {
	taskID, err := e.store.ResolveTaskID(taskRef)
	if err != nil {
		return err
	}
	attempt, attemptErr := e.store.LoadAttempt(taskID)
	if attemptErr == nil {
		if err := e.removeAttemptRuntime(attempt, opts); err != nil {
			return err
		}
	}
	return e.store.RemoveTask(taskID)
}

func (e *Engine) findWorktree(dirName, branchName string) (string, string, error) {
	worktrees, err := e.wt.List()
	if err != nil {
		return "", "", err
	}
	for _, info := range worktrees {
		if filepath.Base(info.Path) == dirName || info.Branch == branchName {
			return info.Path, info.Branch, nil
		}
	}
	return "", "", fmt.Errorf("worktree %q not found after creation", dirName)
}

func (e *Engine) removeAttemptRuntime(attempt AttemptRecord, opts RemoveOptions) error {
	if attempt.Worktree != "" {
		if e.wt == nil {
			return errors.New("worktree client is required")
		}
		if _, err := e.wt.Remove(attempt.Worktree, worktree.RemoveOptions{
			Force:        opts.Force,
			DeleteBranch: true,
		}); err != nil {
			return fmt.Errorf("remove worktree %s: %w", attempt.Worktree, err)
		}
	}
	if attempt.TmuxSession != "" {
		_ = KillTmuxSession(TmuxProfile{Session: attempt.TmuxSession, Socket: attempt.TmuxSocket})
	}
	return nil
}

func loadTaskSource(input string) (source string, sourceFile string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errors.New("task source is required")
	}
	info, statErr := os.Stat(input)
	if statErr == nil {
		if info.IsDir() {
			return "", "", fmt.Errorf("task source must be a file, not a directory: %s", input)
		}
		abs, err := filepath.Abs(input)
		if err != nil {
			return "", "", err
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return "", "", err
		}
		return strings.TrimSpace(string(data)), abs, nil
	}
	return input, "", nil
}
