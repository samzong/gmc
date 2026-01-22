package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitcmd"
	"github.com/samzong/gmc/internal/gitutil"
	"github.com/samzong/gmc/internal/stringsutil"
)

type Options struct {
	Verbose bool
}

type Client struct {
	runner  gitcmd.Runner
	verbose bool
}

func NewClient(opts Options) *Client {
	return &Client{
		runner:  gitcmd.Runner{Verbose: opts.Verbose},
		verbose: opts.Verbose,
	}
}

func (c *Client) logVerboseOutput(label string, data []byte) {
	if c == nil || !c.verbose || len(data) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, label, string(data))
}

// CommitInfo represents information about a single commit
type CommitInfo struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
	Body    string `json:"body"`
}

// IsGitRepository checks if the current directory is a git repository
func (c *Client) IsGitRepository() bool {
	_, err := c.runner.Run("rev-parse", "--is-inside-work-tree")
	return err == nil
}

func (c *Client) CheckGitRepository() error {
	if !c.IsGitRepository() {
		return errors.New("not in a git repository. Please run this command in a git repository directory")
	}
	return nil
}

func (c *Client) GetDiff() (string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return "", err
	}

	runner := c.runner
	result, err := runner.Run("diff")
	if err != nil {
		return "", fmt.Errorf("failed to run git diff: %w", err)
	}

	unstaged := string(result.Stdout)

	result, err = runner.Run("diff", "--cached")
	if err != nil {
		return "", fmt.Errorf("failed to run git diff --cached: %w", err)
	}

	staged := string(result.Stdout)

	diff := unstaged + staged
	return diff, nil
}

func (c *Client) GetStagedDiff() (string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return "", err
	}

	result, err := c.runner.RunLogged("diff", "--cached", "-U1")
	if err != nil {
		c.logVerboseOutput("Git stderr:", result.Stderr)
		return "", fmt.Errorf("failed to run git diff --cached: %w", err)
	}

	return string(result.Stdout), nil
}

// GetStagedDiffStats returns staged diff stats for budgeted truncation.
func (c *Client) GetStagedDiffStats() (string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return "", err
	}

	result, err := c.runner.RunLogged("diff", "--cached", "--numstat", "--summary")
	if err != nil {
		c.logVerboseOutput("Git stderr:", result.Stderr)
		return "", fmt.Errorf("failed to run git diff --cached --numstat --summary: %w", err)
	}

	return string(result.Stdout), nil
}

func (c *Client) ParseChangedFiles() ([]string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, err
	}

	runner := c.runner
	result, err := runner.Run("diff", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("failed to run git diff --name-only: %w", err)
	}

	unstaged := strings.Split(result.StdoutString(true), "\n")

	result, err = runner.Run("diff", "--cached", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("failed to run git diff --cached --name-only: %w", err)
	}

	staged := strings.Split(result.StdoutString(true), "\n")

	files := make([]string, 0, len(unstaged)+len(staged))
	for _, file := range unstaged {
		if file != "" {
			files = append(files, file)
		}
	}

	for _, file := range staged {
		if file != "" {
			files = append(files, file)
		}
	}

	return stringsutil.UniqueStrings(files), nil
}

func (c *Client) ParseStagedFiles() ([]string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, err
	}

	runResult, err := c.runner.RunLogged("diff", "--cached", "--name-only")
	if err != nil {
		c.logVerboseOutput("Git stderr:", runResult.Stderr)
		return nil, fmt.Errorf("failed to run git diff --cached --name-only: %w", err)
	}

	stagedFiles := strings.Split(runResult.StdoutString(true), "\n")

	var files []string
	for _, file := range stagedFiles {
		if file != "" {
			files = append(files, file)
		}
	}

	return files, nil
}

func (c *Client) AddAll() error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}

	result, err := c.runner.RunLogged("add", ".")
	if err != nil {
		c.logVerboseOutput("Git stderr:", result.Stderr)
		return fmt.Errorf("failed to run git add .: %w", err)
	}

	c.logVerboseOutput("Git output:", result.Stderr)

	return nil
}

func (c *Client) Commit(message string, args ...string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}

	commitArgs := append([]string{"commit", "-m", message}, args...)
	result, err := c.runner.RunLogged(commitArgs...)

	// Always show output in verbose mode
	if c.verbose {
		c.logVerboseOutput("Git output:", result.Stdout)
		c.logVerboseOutput("Git stderr:", result.Stderr)
	}

	if err != nil {
		// Include git error output in the error message
		return gitutil.WrapGitError("Failed to run git commit", result, err)
	}

	return nil
}

func (c *Client) CreateAndSwitchBranch(branchName string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}

	if err := validateBranchName(branchName); err != nil {
		return err
	}

	if exists, err := c.branchExists(branchName); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("branch '%s' already exists", branchName)
	}

	return c.createAndSwitchBranch(branchName)
}

func validateBranchName(branchName string) error {
	if branchName == "" {
		return errors.New("branch name cannot be empty")
	}

	if strings.Contains(branchName, "..") || strings.HasPrefix(branchName, "-") {
		return fmt.Errorf("invalid branch name: '%s'", branchName)
	}

	return nil
}

func (c *Client) branchExists(branchName string) (bool, error) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, "Checking if branch exists: git rev-parse --verify %s\n", branchName)
	}

	_, err := c.runner.Run("rev-parse", "--verify", branchName)
	return err == nil, nil
}

func (c *Client) createAndSwitchBranch(branchName string) error {
	if c.verbose {
		fmt.Fprintf(os.Stderr, "Creating and switching to branch: git checkout -b %s\n", branchName)
	}

	result, err := c.runner.Run("checkout", "-b", branchName)
	if err != nil {
		return gitutil.WrapGitError(fmt.Sprintf("failed to create and switch to branch '%s'", branchName), result, err)
	}

	if c.verbose && len(result.Stdout) > 0 {
		fmt.Fprintln(os.Stderr, "Git output:", string(result.Stdout))
	}

	return nil
}

// GetLatestTag returns the most recently created tag in the repository.
func (c *Client) GetLatestTag() (string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return "", err
	}

	result, err := c.runner.RunLogged("tag", "--sort=-creatordate")
	if err != nil {
		return "", fmt.Errorf("failed to list tags: %w", err)
	}

	output := result.StdoutString(true)
	if output == "" {
		return "", nil
	}

	tags := strings.Split(output, "\n")
	return strings.TrimSpace(tags[0]), nil
}

// GetCommitsSinceTag returns the commits between the given tag (exclusive) and HEAD.
// If the tag is empty or not found, all commits up to HEAD are returned.
func (c *Client) GetCommitsSinceTag(tag string) ([]CommitInfo, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, err
	}

	format := "%H%x1f%an%x1f%ad%x1f%s%x1f%b%x1e"
	args := []string{"log", "--pretty=format:" + format, "--date=short"}

	if tag != "" {
		exists, err := c.tagExists(tag)
		if err != nil {
			return nil, err
		}
		if exists {
			args = append(args, tag+"..HEAD")
		}
	}

	result, err := c.runner.RunLogged(args...)
	if err != nil {
		if len(result.Stderr) > 0 {
			return nil, fmt.Errorf("failed to run git log: %s", strings.TrimSpace(string(result.Stderr)))
		}
		return nil, fmt.Errorf("failed to run git log: %w", err)
	}

	data := result.Stdout
	if len(bytes.TrimSpace(data)) == 0 {
		return []CommitInfo{}, nil
	}

	records := bytes.Split(data, []byte{0x1e})
	commits := make([]CommitInfo, 0, len(records))

	for _, record := range records {
		record = bytes.TrimSpace(record)
		if len(record) == 0 {
			continue
		}

		fields := bytes.Split(record, []byte{0x1f})
		if len(fields) < 5 {
			continue
		}

		commit := CommitInfo{
			Hash:    strings.TrimSpace(string(fields[0])),
			Author:  strings.TrimSpace(string(fields[1])),
			Date:    strings.TrimSpace(string(fields[2])),
			Message: strings.TrimSpace(string(fields[3])),
			Body:    strings.TrimSpace(string(fields[4])),
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

// CreateAnnotatedTag creates an annotated tag with the provided message.
func (c *Client) CreateAnnotatedTag(tag string, message string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}

	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("tag name cannot be empty")
	}

	if message == "" {
		message = "Release " + tag
	}

	if c.verbose {
		fmt.Fprintf(os.Stderr, "Running: git tag -a %s -m %q\n", tag, message)
	}

	result, err := c.runner.Run("tag", "-a", tag, "-m", message)
	if err != nil {
		return gitutil.WrapGitError(fmt.Sprintf("failed to create tag '%s'", tag), result, err)
	}

	return nil
}

func (c *Client) tagExists(tag string) (bool, error) {
	if tag == "" {
		return false, nil
	}

	ref := "refs/tags/" + tag
	_, err := c.runner.Run("rev-parse", "--verify", ref)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify tag %s: %w", tag, err)
	}

	return true, nil
}

// GetCommitHistory retrieves commit history with different modes
func (c *Client) GetCommitHistory(limit int, teamMode bool) ([]CommitInfo, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, err
	}

	var args []string
	if teamMode {
		// Team mode: get commits from all authors
		args = []string{"log", "--pretty=format:%h|%an|%ad|%s", "--date=short", fmt.Sprintf("-n%d", limit)}
	} else {
		// Personal mode: get commits from current user only
		currentUser, err := c.getCurrentGitUser()
		if err != nil {
			return nil, fmt.Errorf("failed to get current git user: %w", err)
		}
		args = []string{"log", "--pretty=format:%h|%an|%ad|%s", "--date=short",
			"--author=" + currentUser, fmt.Sprintf("-n%d", limit)}
	}

	result, err := c.runner.RunLogged(args...)
	if err != nil {
		c.logVerboseOutput("Git stderr:", result.Stderr)
		return nil, fmt.Errorf("failed to run git log: %w", err)
	}

	output := result.StdoutString(true)
	if output == "" {
		return []CommitInfo{}, nil
	}

	return parseCommitOutput(output)
}

// getCurrentGitUser gets the current git user name
func (c *Client) getCurrentGitUser() (string, error) {
	result, err := c.runner.Run("config", "user.name")
	if err != nil {
		return "", fmt.Errorf("failed to get git user name: %w", err)
	}

	return result.StdoutString(true), nil
}

// parseCommitOutput parses the git log output into CommitInfo structs
func parseCommitOutput(output string) ([]CommitInfo, error) {
	lines := strings.Split(output, "\n")
	commits := make([]CommitInfo, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue // Skip malformed lines
		}

		commit := CommitInfo{
			Hash:    strings.TrimSpace(parts[0]),
			Author:  strings.TrimSpace(parts[1]),
			Date:    strings.TrimSpace(parts[2]),
			Message: strings.TrimSpace(parts[3]),
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

// ResolveFiles expands directories to individual files and validates file paths
func (c *Client) ResolveFiles(paths []string) ([]string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, err
	}

	var resolvedFiles []string
	for _, path := range paths {
		// Clean path to prevent directory traversal
		cleanPath := filepath.Clean(path)

		info, err := os.Stat(cleanPath)
		if err != nil {
			if os.IsNotExist(err) {
				inIndex, indexErr := c.isPathInStagedDiff(cleanPath)
				if indexErr != nil {
					return nil, fmt.Errorf("failed to resolve path %s: %w", path, indexErr)
				}
				if inIndex {
					resolvedFiles = append(resolvedFiles, cleanPath)
					continue
				}
				return nil, fmt.Errorf("file or directory does not exist: %s", path)
			}
			return nil, fmt.Errorf("failed to check path: %s: %w", path, err)
		}

		if info.IsDir() {
			// Expand directory to git-tracked files
			dirFiles, err := c.getGitTrackedFilesInDir(cleanPath)
			if err != nil {
				return nil, fmt.Errorf("failed to get files in directory %s: %w", path, err)
			}
			resolvedFiles = append(resolvedFiles, dirFiles...)
		} else {
			resolvedFiles = append(resolvedFiles, cleanPath)
		}
	}

	// Remove duplicates while preserving order
	return stringsutil.UniqueStrings(resolvedFiles), nil
}

func (c *Client) isPathInStagedDiff(path string) (bool, error) {
	gitPath := filepath.ToSlash(path)

	result, err := c.runner.RunLogged("diff", "--cached", "--name-only", "--", gitPath)
	if err != nil {
		c.logVerboseOutput("Git stderr:", result.Stderr)
		return false, fmt.Errorf("failed to inspect staged diff: %w", err)
	}

	output := result.StdoutString(true)
	if output == "" {
		return false, nil
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == gitPath {
			return true, nil
		}
	}

	return false, nil
}

// getGitTrackedFilesInDir gets all git-tracked files in a directory, including untracked files that are not ignored.
func (c *Client) getGitTrackedFilesInDir(dir string) ([]string, error) {
	runResult, err := c.runner.RunLogged("ls-files", "--cached", "--others", "--exclude-standard", dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list git files in directory: %w", err)
	}

	output := runResult.StdoutString(true)
	if output == "" {
		return []string{}, nil
	}

	files := strings.Split(output, "\n")
	var tracked []string
	for _, file := range files {
		if file != "" {
			tracked = append(tracked, file)
		}
	}

	return tracked, nil
}

// CheckFileStatus checks the git status of specified files
// Returns: staged, modified, untracked files
func (c *Client) CheckFileStatus(files []string) ([]string, []string, []string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, nil, nil, err
	}

	var staged, modified, untracked []string

	for _, file := range files {
		// Check if file is staged
		isStaged, err := c.isFileStaged(file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to check staged status for %s: %w", file, err)
		}

		// Check if file is modified
		isModified, err := c.isFileModified(file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to check modified status for %s: %w", file, err)
		}

		// Check if file is tracked
		isTracked, err := c.isFileTracked(file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to check tracked status for %s: %w", file, err)
		}

		switch {
		case isStaged:
			staged = append(staged, file)
		case isModified:
			modified = append(modified, file)
		case !isTracked:
			untracked = append(untracked, file)
		}
	}

	return staged, modified, untracked, nil
}

// isFileStaged checks if a file is staged
func (c *Client) isFileStaged(file string) (bool, error) {
	result, err := c.runner.Run("diff", "--cached", "--name-only", file)
	if err != nil {
		return false, err
	}

	return result.StdoutString(true) != "", nil
}

// isFileModified checks if a file is modified (unstaged changes)
func (c *Client) isFileModified(file string) (bool, error) {
	result, err := c.runner.Run("diff", "--name-only", file)
	if err != nil {
		return false, err
	}

	return result.StdoutString(true) != "", nil
}

// isFileTracked checks if a file is tracked by git
func (c *Client) isFileTracked(file string) (bool, error) {
	result, err := c.runner.Run("ls-files", file)
	if err != nil {
		return false, err
	}

	return result.StdoutString(true) != "", nil
}

// StageFiles stages specific files
func (c *Client) StageFiles(files []string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}

	for _, file := range files {
		result, err := c.runner.RunLogged("add", file)
		if err != nil {
			return gitutil.WrapGitError("failed to stage file "+file, result, err)
		}
	}

	return nil
}

// GetFilesDiff gets diff for specific files
func (c *Client) GetFilesDiff(files []string) (string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", nil
	}

	args := []string{"diff", "--cached"}
	args = append(args, "--")
	args = append(args, files...)

	result, err := c.runner.RunLogged(args...)
	if err != nil {
		c.logVerboseOutput("Git stderr:", result.Stderr)
		return "", fmt.Errorf("failed to get diff for files: %w", err)
	}

	return string(result.Stdout), nil
}

// CommitFiles commits specific files only
func (c *Client) CommitFiles(message string, files []string, args ...string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}

	commitArgs := []string{"commit", "-m", message}
	commitArgs = append(commitArgs, args...)
	commitArgs = append(commitArgs, "--")
	commitArgs = append(commitArgs, files...)

	result, err := c.runner.RunLogged(commitArgs...)

	// Always show output in verbose mode
	if c.verbose {
		c.logVerboseOutput("Git output:", result.Stdout)
		c.logVerboseOutput("Git stderr:", result.Stderr)
	}

	if err != nil {
		// Include git error output in the error message
		return gitutil.WrapGitError("Failed to commit files", result, err)
	}

	return nil
}
