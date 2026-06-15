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
	ID         string    `json:"id" yaml:"id"`
	Title      string    `json:"title,omitempty" yaml:"title,omitempty"`
	State      string    `json:"state" yaml:"state"`
	Source     string    `json:"source" yaml:"source"`
	SourceFile string    `json:"source_file,omitempty" yaml:"source_file,omitempty"`
	Issue      string    `json:"issue,omitempty" yaml:"issue,omitempty"`
	CreatedAt  time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" yaml:"updated_at"`
}

type AttemptRecord struct {
	ID          string    `json:"id" yaml:"id"`
	TaskID      string    `json:"task_id" yaml:"task_id"`
	Worktree    string    `json:"worktree,omitempty" yaml:"worktree,omitempty"`
	Branch      string    `json:"branch,omitempty" yaml:"branch,omitempty"`
	Agent       string    `json:"agent,omitempty" yaml:"agent,omitempty"`
	Model       string    `json:"model,omitempty" yaml:"model,omitempty"`
	Mode        string    `json:"mode,omitempty" yaml:"mode,omitempty"`
	TmuxSession string    `json:"tmux_session,omitempty" yaml:"tmux_session,omitempty"`
	TmuxSocket  string    `json:"tmux_socket,omitempty" yaml:"tmux_socket,omitempty"`
	ContextFile string    `json:"context_file,omitempty" yaml:"context_file,omitempty"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" yaml:"updated_at"`
}

type Summary struct {
	Task    Record         `json:"task"`
	Attempt *AttemptRecord `json:"attempt,omitempty"`
}

func StateValues() []string {
	return []string{TaskNew, TaskPlan, TaskCode, TaskReview, TaskShip}
}

func ValidState(state string) bool {
	for _, candidate := range StateValues() {
		if state == candidate {
			return true
		}
	}
	return false
}
