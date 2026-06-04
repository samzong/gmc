package task

import "time"

// Task-level states (human-facing).
const (
	TaskIntake     = "intake"
	TaskPlanning   = "planning"
	TaskRunning    = "running"
	TaskNeedsHuman = "needs-human"
	TaskReviewing  = "reviewing"
	TaskVerifying  = "verifying"
	TaskReadyForPR = "ready-for-pr"
	TaskPROpen     = "pr-open"
	TaskDone       = "done"
	TaskArchived   = "archived"
)

// Attempt-level states.
const (
	AttemptCreated       = "created"
	AttemptRunning       = "running"
	AttemptWaitingHuman  = "waiting-human"
	AttemptHumanAttached = "human-attached"
	AttemptDone          = "done"
	AttemptFailed        = "failed"
	AttemptLost          = "lost"
	AttemptPromoted      = "promoted"
	AttemptArchived      = "archived"
)

// Run-level states.
const (
	RunQueued       = "queued"
	RunRunning      = "running"
	RunWaitingHuman = "waiting-human"
	RunPassed       = "passed"
	RunFailed       = "failed"
	RunCancelled    = "cancelled"
	RunLost         = "lost"
	RunArchived     = "archived"
)

// Run types.
const (
	RunTypeAgentSession = "agent-session"
	RunTypeAgentReview  = "agent-review"
	RunTypeCommandCheck = "command-check"
	RunTypeCommandFix   = "command-fix"
	RunTypeHumanAttach  = "human-attach"
)

// Runtime kinds.
const (
	RuntimeTmux     = "tmux"
	RuntimeHeadless = "headless"
)

// Record is the durable task snapshot.
type Record struct {
	ID        string    `yaml:"id"`
	Title     string    `yaml:"title,omitempty"`
	State     string    `yaml:"state"`
	Source    string    `yaml:"source"`
	Issue     string    `yaml:"issue,omitempty"`
	PR        string    `yaml:"pr,omitempty"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

// AttemptRecord is one solution path for a task.
type AttemptRecord struct {
	ID                string    `yaml:"id"`
	TaskID            string    `yaml:"task_id"`
	State             string    `yaml:"state"`
	Worktree          string    `yaml:"worktree,omitempty"`
	Branch            string    `yaml:"branch,omitempty"`
	Agent             string    `yaml:"agent,omitempty"`
	Model             string    `yaml:"model,omitempty"`
	Mode              string    `yaml:"mode,omitempty"`
	TmuxSession       string    `yaml:"tmux_session,omitempty"`
	TmuxSocket        string    `yaml:"tmux_socket,omitempty"`
	ReviewTmuxSession string    `yaml:"review_tmux_session,omitempty"`
	ReviewTmuxSocket  string    `yaml:"review_tmux_socket,omitempty"`
	ContextFile       string    `yaml:"context_file,omitempty"`
	CreatedAt         time.Time `yaml:"created_at"`
	UpdatedAt         time.Time `yaml:"updated_at"`
}

// RunRecord is one executable step inside an attempt.
type RunRecord struct {
	ID           string    `yaml:"id"`
	TaskID       string    `yaml:"task_id"`
	AttemptID    string    `yaml:"attempt_id"`
	Type         string    `yaml:"type"`
	Runtime      string    `yaml:"runtime"`
	State        string    `yaml:"state"`
	Command      []string  `yaml:"command,omitempty"`
	Workdir      string    `yaml:"workdir,omitempty"`
	ExitCode     *int      `yaml:"exit_code,omitempty"`
	TmuxSession  string    `yaml:"tmux_session,omitempty"`
	LogFile      string    `yaml:"log_file,omitempty"`
	ArtifactFile string    `yaml:"artifact_file,omitempty"`
	Summary      string    `yaml:"summary,omitempty"`
	StartedAt    time.Time `yaml:"started_at,omitempty"`
	FinishedAt   time.Time `yaml:"finished_at,omitempty"`
	CreatedAt    time.Time `yaml:"created_at"`
	UpdatedAt    time.Time `yaml:"updated_at"`
}

// Event is one append-only ledger fact.
type Event struct {
	Time    time.Time         `json:"time"`
	Type    string            `json:"type"`
	TaskID  string            `json:"task_id,omitempty"`
	Payload map[string]string `json:"payload,omitempty"`
}

// Summary is returned by list/show APIs.
type Summary struct {
	Task           Record          `json:"task"`
	Attempts       []AttemptRecord `json:"attempts,omitempty"`
	Runs           []RunRecord     `json:"runs,omitempty"`
	RuntimeStatus  RuntimeStatus   `json:"runtime_status,omitempty"`
	RuntimeMessage string          `json:"runtime_message,omitempty"`
	LastResult     *StageResult    `json:"last_result,omitempty"`
}

// StageResult is the latest review/verify outcome for show/list.
type StageResult struct {
	RunID        string `json:"run_id"`
	RunType      string `json:"run_type"`
	State        string `json:"state"`
	ExitCode     int    `json:"exit_code,omitempty"`
	ArtifactFile string `json:"artifact_file,omitempty"`
	Summary      string `json:"summary,omitempty"`
	LogFile      string `json:"log_file,omitempty"`
}

// GCItem describes one dry-run garbage-collection action.
type GCItem struct {
	Kind    string `json:"kind"`
	TaskID  string `json:"task_id,omitempty"`
	Detail  string `json:"detail"`
	Would   bool   `json:"would_remove"`
	Blocked string `json:"blocked_reason,omitempty"`
}

// GCReport aggregates dry-run output.
type GCReport struct {
	Items []GCItem `json:"items"`
}
