// Package workflow provides the commit workflow orchestration logic.
package workflow

// GitClient abstracts git operations for testability.
type GitClient interface {
	IsGitRepository() bool
	CheckGitRepository() error
	AddAll() error
	StageFiles(files []string) error
	GetStagedDiff() (string, error)
	GetStagedDiffStats() (string, error)
	GetFilesDiff(files []string) (string, error)
	ParseStagedFiles() ([]string, error)
	ResolveFiles(paths []string) ([]string, error)
	CheckFileStatus(files []string) (staged, modified, untracked []string, err error)
	Commit(message string, args ...string) error
	CommitFiles(message string, files []string, args ...string) error
	CreateAndSwitchBranch(branchName string) error
}

// LLMClient abstracts LLM operations for testability.
type LLMClient interface {
	GenerateCommitMessage(prompt string, model string) (string, error)
}
