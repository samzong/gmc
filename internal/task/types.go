package task

import "time"

const TaskNew = "new"

const (
	RunKindAgent   = "agent"
	RunKindCommand = "command"
)

const (
	RunRuntimeTmux     = "tmux"
	RunRuntimeHeadless = "headless"
)

const (
	RunStatusRunning = "running"
	RunStatusPassed  = "passed"
	RunStatusFailed  = "failed"
)

const (
	EventTaskCreated  = "task.created"
	EventTaskStarted  = "task.started"
	EventTaskAdvanced = "task.advanced"
	EventRunStarted   = "run.started"
	EventRunFinished  = "run.finished"
)

type Record struct {
	ID               string             `json:"id" yaml:"id"`
	Title            string             `json:"title,omitempty" yaml:"title,omitempty"`
	State            string             `json:"state" yaml:"state"`
	Source           string             `json:"source" yaml:"source"`
	SourceFile       string             `json:"source_file,omitempty" yaml:"source_file,omitempty"`
	Issue            string             `json:"issue,omitempty" yaml:"issue,omitempty"`
	Workflow         string             `json:"workflow,omitempty" yaml:"workflow,omitempty"`
	CurrentNode      string             `json:"current_node,omitempty" yaml:"current_node,omitempty"`
	WorkflowSnapshot WorkflowDefinition `json:"workflow_snapshot,omitempty" yaml:"workflow_snapshot,omitempty"`
	CreatedAt        time.Time          `json:"created_at" yaml:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" yaml:"updated_at"`
}

type AttemptRecord struct {
	ID           string              `json:"id" yaml:"id"`
	TaskID       string              `json:"task_id" yaml:"task_id"`
	Worktree     string              `json:"worktree,omitempty" yaml:"worktree,omitempty"`
	Branch       string              `json:"branch,omitempty" yaml:"branch,omitempty"`
	Agent        string              `json:"agent,omitempty" yaml:"agent,omitempty"`
	Model        string              `json:"model,omitempty" yaml:"model,omitempty"`
	TmuxSession  string              `json:"tmux_session,omitempty" yaml:"tmux_session,omitempty"`
	TmuxSocket   string              `json:"tmux_socket,omitempty" yaml:"tmux_socket,omitempty"`
	TmuxSessions []TmuxSessionRecord `json:"tmux_sessions,omitempty" yaml:"tmux_sessions,omitempty"`
	ContextFile  string              `json:"context_file,omitempty" yaml:"context_file,omitempty"`
	CreatedAt    time.Time           `json:"created_at" yaml:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at" yaml:"updated_at"`
}

type TmuxSessionRecord struct {
	Node      string    `json:"node,omitempty" yaml:"node,omitempty"`
	Agent     string    `json:"agent,omitempty" yaml:"agent,omitempty"`
	Command   string    `json:"command,omitempty" yaml:"command,omitempty"`
	Session   string    `json:"session,omitempty" yaml:"session,omitempty"`
	Socket    string    `json:"socket,omitempty" yaml:"socket,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty" yaml:"started_at,omitempty"`
}

type RunRecord struct {
	ID        string     `json:"id" yaml:"id"`
	TaskID    string     `json:"task_id" yaml:"task_id"`
	AttemptID string     `json:"attempt_id,omitempty" yaml:"attempt_id,omitempty"`
	Node      string     `json:"node,omitempty" yaml:"node,omitempty"`
	Kind      string     `json:"kind" yaml:"kind"`
	Runtime   string     `json:"runtime" yaml:"runtime"`
	Command   []string   `json:"command" yaml:"command"`
	Cwd       string     `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Session   string     `json:"session,omitempty" yaml:"session,omitempty"`
	Socket    string     `json:"socket,omitempty" yaml:"socket,omitempty"`
	PID       int        `json:"pid,omitempty" yaml:"pid,omitempty"`
	Status    string     `json:"status" yaml:"status"`
	ExitCode  *int       `json:"exit_code,omitempty" yaml:"exit_code,omitempty"`
	Error     string     `json:"error,omitempty" yaml:"error,omitempty"`
	StartedAt time.Time  `json:"started_at" yaml:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty" yaml:"ended_at,omitempty"`
	Stdout    string     `json:"stdout,omitempty" yaml:"stdout,omitempty"`
	Stderr    string     `json:"stderr,omitempty" yaml:"stderr,omitempty"`
}

type EventRecord struct {
	Time      time.Time `json:"ts"`
	Type      string    `json:"type"`
	TaskID    string    `json:"task_id"`
	AttemptID string    `json:"attempt_id,omitempty"`
	RunID     string    `json:"run_id,omitempty"`
	Node      string    `json:"node,omitempty"`
	Status    string    `json:"status,omitempty"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	Message   string    `json:"message,omitempty"`
}

type Summary struct {
	Task     Record         `json:"task"`
	Attempt  *AttemptRecord `json:"attempt,omitempty"`
	Runs     []RunRecord    `json:"runs,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

type WorkflowConfig struct {
	Version   int                           `json:"version" yaml:"version"`
	Default   string                        `json:"default,omitempty" yaml:"default,omitempty"`
	Start     string                        `json:"start,omitempty" yaml:"start,omitempty"`
	Nodes     map[string]WorkflowNode       `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Workflows map[string]WorkflowDefinition `json:"workflows" yaml:"workflows"`
}

type WorkflowDefinition struct {
	Name  string                  `json:"name,omitempty" yaml:"name,omitempty"`
	Start string                  `json:"start,omitempty" yaml:"start,omitempty"`
	Nodes map[string]WorkflowNode `json:"nodes" yaml:"nodes"`
}

type WorkflowNode struct {
	ID      string   `json:"id,omitempty" yaml:"id,omitempty"`
	Agent   string   `json:"agent,omitempty" yaml:"agent,omitempty"`
	Command string   `json:"command,omitempty" yaml:"command,omitempty"`
	Model   string   `json:"model,omitempty" yaml:"model,omitempty"`
	Prompt  string   `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Skill   string   `json:"skill,omitempty" yaml:"skill,omitempty"`
	Skills  []string `json:"skills,omitempty" yaml:"skills,omitempty"`
	Next    string   `json:"next,omitempty" yaml:"next,omitempty"`
}
