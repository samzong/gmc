package task

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
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
	BaseBranch string
	Workflow   string
	Command    string
}

type AdvanceOptions struct {
	TaskID string
	ToNode string
}

type RemoveOptions struct {
	Force bool
}

type RunOptions struct {
	TaskID  string
	Command []string
	Stdout  io.Writer
	Stderr  io.Writer
}

var storeAppendEvent = (*Store).AppendEvent

func NewEngine(store *Store, wt *worktree.Client) *Engine {
	return &Engine{store: store, wt: wt}
}

func (e *Engine) CreateTask(input string) (Record, []string, error) {
	source, sourceFile, err := loadTaskSource(input)
	if err != nil {
		return Record{}, nil, err
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
		return Record{}, nil, err
	}
	warnings := appendWarning(nil, storeAppendEvent(e.store, EventRecord{Type: EventTaskCreated, TaskID: rec.ID}))
	return rec, warnings, nil
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
	cfg, _, err := LoadWorkflowConfig()
	if err != nil {
		return Summary{}, err
	}
	workflow, err := SelectWorkflow(cfg, opts.Workflow)
	if err != nil {
		return Summary{}, err
	}
	node := workflow.Nodes[workflow.Start]
	agent, err := WorkflowNodeAgent(node, opts.Agent)
	if err != nil {
		return Summary{}, err
	}
	model := WorkflowNodeModel(node, opts.Model)

	attemptID := "attempt-1"
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
		Agent:     agent,
		Model:     model,
		CreatedAt: time.Now().UTC(),
	}
	rec.Workflow = workflow.Name
	rec.WorkflowSnapshot = workflow
	rec.CurrentNode = node.ID
	rec.State = node.ID
	rec.UpdatedAt = time.Now().UTC()
	contextFile, err := WriteTaskContextFile(wtPath, rec, attempt)
	if err != nil {
		return Summary{}, fmt.Errorf("write task brief: %w", err)
	}
	attempt.ContextFile = contextFile

	cmdNode := node
	if strings.TrimSpace(opts.Command) != "" {
		cmdNode.Command = strings.TrimSpace(opts.Command)
	}
	attempt, command, err := e.runWorkflowNode(attempt, cmdNode, BuildWorkflowNodePrompt(rec, node))
	if err != nil {
		return Summary{}, err
	}
	if err := e.store.SaveAttempt(attempt); err != nil {
		return Summary{}, err
	}
	if err := e.store.writeTask(rec); err != nil {
		return Summary{}, err
	}

	var warnings []string
	started := EventRecord{Type: EventTaskStarted, TaskID: taskID, AttemptID: attempt.ID, Node: node.ID}
	warnings = appendWarning(warnings, storeAppendEvent(e.store, started))
	warnings = append(warnings, e.recordAgentRun(attempt, node.ID, command)...)
	return e.loadSummaryWithWarnings(taskID, warnings)
}

func (e *Engine) Advance(opts AdvanceOptions) (Summary, error) {
	taskID, err := e.store.ResolveTaskID(opts.TaskID)
	if err != nil {
		return Summary{}, err
	}
	rec, err := e.store.LoadTask(taskID)
	if err != nil {
		return Summary{}, err
	}
	current := strings.TrimSpace(rec.CurrentNode)
	if current == "" {
		current = strings.TrimSpace(rec.State)
	}
	if current == "" || current == TaskNew {
		return Summary{}, fmt.Errorf("task %s has not started a workflow", taskID)
	}
	if current == "done" {
		return Summary{}, fmt.Errorf("task %s is already done", taskID)
	}
	workflow, err := workflowForTask(rec)
	if err != nil {
		return Summary{}, err
	}
	attempt, err := e.store.LoadAttempt(taskID)
	if err != nil {
		return Summary{}, err
	}
	node, ok := workflow.Nodes[current]
	if !ok {
		return Summary{}, fmt.Errorf("workflow %q has no current node %q", workflow.Name, current)
	}
	next := strings.TrimSpace(opts.ToNode)
	if next == "" {
		next = strings.TrimSpace(node.Next)
	}
	if next == "" {
		return Summary{}, fmt.Errorf("workflow node %q has no next node", current)
	}
	rec.CurrentNode = next
	rec.State = next
	rec.UpdatedAt = time.Now().UTC()
	advancedEvent := EventRecord{
		Type:      EventTaskAdvanced,
		TaskID:    taskID,
		AttemptID: attempt.ID,
		Node:      next,
		Message:   fmt.Sprintf("from %s to %s", current, next),
	}
	if next == "done" {
		if err := e.store.writeTask(rec); err != nil {
			return Summary{}, err
		}
		warnings := appendWarning(nil, storeAppendEvent(e.store, advancedEvent))
		return e.loadSummaryWithWarnings(taskID, warnings)
	}
	nextNode, ok := workflow.Nodes[next]
	if !ok {
		return Summary{}, fmt.Errorf("workflow %q node %q not found", workflow.Name, next)
	}
	agent, err := WorkflowNodeAgent(nextNode, attempt.Agent)
	if err != nil {
		return Summary{}, err
	}
	carriedModel := ""
	if agent == attempt.Agent {
		carriedModel = attempt.Model
	}
	model := WorkflowNodeModel(nextNode, carriedModel)
	attempt.Agent = agent
	attempt.Model = model
	attempt.UpdatedAt = time.Now().UTC()
	prompt := BuildWorkflowNodePrompt(rec, nextNode)
	attempt, command, err := e.runWorkflowNode(attempt, nextNode, prompt)
	if err != nil {
		return Summary{}, err
	}
	if err := e.store.SaveAttempt(attempt); err != nil {
		return Summary{}, err
	}
	if err := e.store.writeTask(rec); err != nil {
		return Summary{}, err
	}

	var warnings []string
	warnings = appendWarning(warnings, storeAppendEvent(e.store, advancedEvent))
	warnings = append(warnings, e.recordAgentRun(attempt, next, command)...)
	return e.loadSummaryWithWarnings(taskID, warnings)
}

func (e *Engine) Run(opts RunOptions) (RunResult, error) {
	if len(opts.Command) == 0 || strings.TrimSpace(opts.Command[0]) == "" {
		return RunResult{}, errors.New("command is required")
	}
	taskID, err := e.store.ResolveTaskID(opts.TaskID)
	if err != nil {
		return RunResult{}, err
	}
	rec, err := e.store.LoadTask(taskID)
	if err != nil {
		return RunResult{}, err
	}
	attempt, err := e.store.LoadAttempt(taskID)
	if err != nil {
		return RunResult{}, err
	}
	if strings.TrimSpace(attempt.Worktree) == "" {
		return RunResult{}, fmt.Errorf("task %s has no worktree", taskID)
	}
	if info, err := os.Stat(attempt.Worktree); err != nil || !info.IsDir() {
		return RunResult{}, fmt.Errorf("task %s worktree missing: %s", taskID, attempt.Worktree)
	}
	now := time.Now().UTC()
	run := RunRecord{
		ID:        NewRunID(now),
		TaskID:    taskID,
		AttemptID: attempt.ID,
		Node:      strings.TrimSpace(rec.CurrentNode),
		Kind:      RunKindCommand,
		Runtime:   RunRuntimeHeadless,
		Command:   append([]string(nil), opts.Command...),
		Cwd:       attempt.Worktree,
		PID:       os.Getpid(),
		Status:    RunStatusRunning,
		StartedAt: now,
	}
	run.Stdout = RunLogRelPath(run.ID, "stdout")
	run.Stderr = RunLogRelPath(run.ID, "stderr")
	stdoutPath, err := e.store.RunLogPath(taskID, run.Stdout)
	if err != nil {
		return RunResult{}, err
	}
	stderrPath, err := e.store.RunLogPath(taskID, run.Stderr)
	if err != nil {
		return RunResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(stdoutPath), 0o755); err != nil {
		return RunResult{}, err
	}
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return RunResult{}, err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return RunResult{}, err
	}
	defer stderrFile.Close()

	if err := e.store.SaveRun(run); err != nil {
		return RunResult{}, err
	}
	var warnings []string
	warnings = appendWarning(warnings, storeAppendEvent(e.store, EventRecord{
		Type:      EventRunStarted,
		TaskID:    taskID,
		AttemptID: attempt.ID,
		RunID:     run.ID,
		Node:      run.Node,
		Status:    RunStatusRunning,
	}))
	finish := func() RunResult {
		ended := time.Now().UTC()
		run.EndedAt = &ended
		warnings = appendWarning(warnings, e.store.SaveRun(run))
		warnings = appendWarning(warnings, storeAppendEvent(e.store, EventRecord{
			Type:      EventRunFinished,
			TaskID:    taskID,
			AttemptID: attempt.ID,
			RunID:     run.ID,
			Node:      run.Node,
			Status:    run.Status,
			ExitCode:  run.ExitCode,
			Message:   run.Error,
		}))
		return RunResult{Run: run, Warnings: warnings}
	}

	cmd := exec.Command(opts.Command[0], opts.Command[1:]...)
	cmd.Dir = attempt.Worktree
	cmd.Stdout = runLogWriter(stdoutFile, opts.Stdout)
	cmd.Stderr = runLogWriter(stderrFile, opts.Stderr)
	cmd.WaitDelay = 2 * time.Second

	sigCh := make(chan os.Signal, 3)
	signal.Notify(sigCh, runNotifySignals()...)
	defer signal.Stop(sigCh)

	if err := cmd.Start(); err != nil {
		run.Status = RunStatusFailed
		run.Error = err.Error()
		return finish(), nil
	}
	waitErr := cmd.Wait()
	switch {
	case waitErr == nil:
		run.Status = RunStatusPassed
	case errors.Is(waitErr, exec.ErrWaitDelay):
		if ps := cmd.ProcessState; ps != nil && ps.ExitCode() != 0 {
			run.Status = RunStatusFailed
			code := ps.ExitCode()
			run.ExitCode = &code
		} else {
			run.Status = RunStatusPassed
		}
	default:
		run.Status = RunStatusFailed
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			if code, sig, ok := signaledExit(exitErr); ok {
				exitCode := code
				run.ExitCode = &exitCode
				run.Error = "signal: " + sig
			} else if code := exitErr.ExitCode(); code >= 0 {
				run.ExitCode = &code
			} else {
				run.Error = waitErr.Error()
			}
		} else {
			run.Error = waitErr.Error()
		}
	}
	return finish(), nil
}

type ignoreWriteErrors struct {
	w io.Writer
}

func (w ignoreWriteErrors) Write(p []byte) (int, error) {
	_, _ = w.w.Write(p)
	return len(p), nil
}

func runLogWriter(logFile io.Writer, extra io.Writer) io.Writer {
	if extra == nil {
		return logFile
	}
	return io.MultiWriter(logFile, ignoreWriteErrors{extra})
}

func (e *Engine) RunLogPath(taskID, relPath string) (string, error) {
	return e.store.RunLogPath(taskID, relPath)
}

func (e *Engine) EventsPath(taskID string) (string, error) {
	return e.store.EventsPath(taskID)
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

func (e *Engine) runWorkflowNode(attempt AttemptRecord, node WorkflowNode,
	prompt string) (AttemptRecord, []string, error) {
	if attempt.Worktree == "" {
		return AttemptRecord{}, nil, errors.New("attempt has no worktree")
	}
	command, err := WorkflowNodeCommand(node, attempt.Agent, attempt.Model, prompt)
	if err != nil {
		return AttemptRecord{}, nil, err
	}
	session := TmuxSessionName(attempt.TaskID, attempt.ID, node.ID, strconv.Itoa(len(attempt.TmuxSessions)+1))
	profile, err := tmuxSessionStarter(session, attempt.Worktree, command)
	if err != nil {
		return AttemptRecord{}, nil, err
	}
	attempt = recordTmuxSession(attempt, node.ID, profile, command)
	return attempt, command, nil
}

func (e *Engine) recordAgentRun(attempt AttemptRecord, nodeID string, command []string) []string {
	run := RunRecord{
		ID:        NewRunID(time.Now().UTC()),
		TaskID:    attempt.TaskID,
		AttemptID: attempt.ID,
		Node:      nodeID,
		Kind:      RunKindAgent,
		Runtime:   RunRuntimeTmux,
		Command:   command,
		Cwd:       attempt.Worktree,
		Status:    RunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	if n := len(attempt.TmuxSessions); n > 0 {
		run.Session = attempt.TmuxSessions[n-1].Session
		run.Socket = attempt.TmuxSessions[n-1].Socket
	}
	if err := e.store.SaveRun(run); err != nil {
		return []string{fmt.Sprintf("record run %s: %v", run.ID, err)}
	}
	return appendWarning(nil, storeAppendEvent(e.store, EventRecord{
		Type:      EventRunStarted,
		TaskID:    attempt.TaskID,
		AttemptID: attempt.ID,
		RunID:     run.ID,
		Node:      nodeID,
		Status:    RunStatusRunning,
	}))
}

func appendWarning(warnings []string, err error) []string {
	if err != nil {
		return append(warnings, err.Error())
	}
	return warnings
}

func (e *Engine) loadSummaryWithWarnings(taskID string, warnings []string) (Summary, error) {
	sum, err := e.store.LoadSummary(taskID)
	if err != nil {
		return Summary{}, err
	}
	sum.Warnings = append(sum.Warnings, warnings...)
	return sum, nil
}

func recordTmuxSession(attempt AttemptRecord, nodeID string, profile TmuxProfile, command []string) AttemptRecord {
	attempt.TmuxSession = profile.Session
	attempt.TmuxSocket = profile.Socket
	attempt.TmuxSessions = append(attempt.TmuxSessions, TmuxSessionRecord{
		Node:      nodeID,
		Agent:     attempt.Agent,
		Command:   shellJoin(command),
		Session:   profile.Session,
		Socket:    profile.Socket,
		StartedAt: time.Now().UTC(),
	})
	return attempt
}

func workflowForTask(rec Record) (WorkflowDefinition, error) {
	if len(rec.WorkflowSnapshot.Nodes) > 0 {
		name := rec.Workflow
		if strings.TrimSpace(name) == "" {
			name = rec.WorkflowSnapshot.Name
		}
		return NormalizeWorkflowDefinition(name, rec.WorkflowSnapshot)
	}
	cfg := DefaultWorkflowConfig()
	return SelectWorkflow(cfg, rec.Workflow)
}

func (e *Engine) removeAttemptRuntime(attempt AttemptRecord, opts RemoveOptions) error {
	if attempt.Worktree != "" {
		if e.wt == nil {
			return errors.New("worktree client is required")
		}
		if _, err := e.wt.Remove(attempt.Worktree, worktree.RemoveOptions{
			Force:        opts.Force,
			DeleteBranch: true,
		}); err != nil && !isMissingWorktreeErr(err) {
			return fmt.Errorf("remove worktree %s: %w", attempt.Worktree, err)
		}
	}
	killAttemptTmuxSessions(attempt)
	return nil
}

func isMissingWorktreeErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "worktree not found")
}

func killAttemptTmuxSessions(attempt AttemptRecord) {
	seen := map[string]bool{}
	for _, session := range attempt.TmuxSessions {
		if session.Session == "" || seen[session.Session] {
			continue
		}
		_ = KillTmuxSession(TmuxProfile{Session: session.Session, Socket: session.Socket})
		seen[session.Session] = true
	}
	if attempt.TmuxSession != "" && !seen[attempt.TmuxSession] {
		_ = KillTmuxSession(TmuxProfile{Session: attempt.TmuxSession, Socket: attempt.TmuxSocket})
	}
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
