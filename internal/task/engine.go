package task

import (
	"errors"
	"fmt"
	"os"
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
}

type AdvanceOptions struct {
	TaskID string
	ToNode string
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
	cfg, _, err := LoadWorkflowConfig()
	if err != nil {
		return Summary{}, err
	}
	workflow, err := SelectWorkflow(cfg, opts.Workflow)
	if err != nil {
		return Summary{}, err
	}
	node, err := workflowStartNode(workflow)
	if err != nil {
		return Summary{}, err
	}
	agent, err := WorkflowNodeAgent(node, opts.Agent)
	if err != nil {
		return Summary{}, err
	}
	model := WorkflowNodeModel(node, opts.Model)

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

	command, err := WorkflowNodeCommand(node, attempt.Agent, attempt.Model, BuildWorkflowNodePrompt(rec, node))
	if err != nil {
		return Summary{}, err
	}
	session := TmuxSessionName(taskID, attemptID, node.ID, "1")
	profile, err := StartTmuxSession(session, wtPath, command)
	if err != nil {
		return Summary{}, err
	}
	attempt = recordTmuxSession(attempt, node.ID, profile)
	if err := e.store.SaveAttempt(attempt); err != nil {
		return Summary{}, err
	}

	if err := e.store.writeTask(rec); err != nil {
		return Summary{}, err
	}
	return e.store.LoadSummary(taskID)
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
	if next == "done" {
		if err := e.store.writeTask(rec); err != nil {
			return Summary{}, err
		}
		return e.store.LoadSummary(taskID)
	}
	nextNode, ok := workflow.Nodes[next]
	if !ok {
		return Summary{}, fmt.Errorf("workflow %q node %q not found", workflow.Name, next)
	}
	agent, err := WorkflowNodeAgent(nextNode, attempt.Agent)
	if err != nil {
		return Summary{}, err
	}
	model := WorkflowNodeModel(nextNode, attempt.Model)
	attempt.Agent = agent
	attempt.Model = model
	attempt.UpdatedAt = time.Now().UTC()
	prompt := BuildWorkflowNodePrompt(rec, nextNode)
	attempt, err = e.runWorkflowNode(attempt, nextNode, prompt)
	if err != nil {
		return Summary{}, err
	}
	if err := e.store.SaveAttempt(attempt); err != nil {
		return Summary{}, err
	}
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

func (e *Engine) runWorkflowNode(attempt AttemptRecord, node WorkflowNode, prompt string) (AttemptRecord, error) {
	if attempt.Worktree == "" {
		return AttemptRecord{}, errors.New("attempt has no worktree")
	}
	command, err := WorkflowNodeCommand(node, attempt.Agent, attempt.Model, prompt)
	if err != nil {
		return AttemptRecord{}, err
	}
	session := TmuxSessionName(attempt.TaskID, attempt.ID, node.ID, strconv.Itoa(len(attempt.TmuxSessions)+1))
	profile, err := StartTmuxSession(session, attempt.Worktree, command)
	if err != nil {
		return AttemptRecord{}, err
	}
	attempt = recordTmuxSession(attempt, node.ID, profile)
	return attempt, nil
}

func recordTmuxSession(attempt AttemptRecord, nodeID string, profile TmuxProfile) AttemptRecord {
	attempt.TmuxSession = profile.Session
	attempt.TmuxSocket = profile.Socket
	attempt.TmuxSessions = append(attempt.TmuxSessions, TmuxSessionRecord{
		Node:      nodeID,
		Agent:     attempt.Agent,
		Session:   profile.Session,
		Socket:    profile.Socket,
		StartedAt: time.Now().UTC(),
	})
	return attempt
}

func workflowStartNode(wf WorkflowDefinition) (WorkflowNode, error) {
	node, ok := wf.Nodes[wf.Start]
	if !ok {
		return WorkflowNode{}, fmt.Errorf("workflow %q start node %q not found", wf.Name, wf.Start)
	}
	return node, nil
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
		}); err != nil {
			return fmt.Errorf("remove worktree %s: %w", attempt.Worktree, err)
		}
	}
	killAttemptTmuxSessions(attempt)
	return nil
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
