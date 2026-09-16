package workflow

type GitClient interface {
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

type LLMClient interface {
	GenerateCommitMessage(prompt string, model string) (string, error)
}
