package task

import "time"

const (
	TaskNew    = "new"
	TaskPlan   = "plan"
	TaskCode   = "code"
	TaskReview = "review"
	TaskShip   = "ship"
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

type Summary struct {
	Task    Record         `json:"task"`
	Attempt *AttemptRecord `json:"attempt,omitempty"`
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
