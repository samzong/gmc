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
)

var Verbose bool

func gitRunner() gitcmd.Runner {
	return gitcmd.Runner{Verbose: Verbose}
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
func IsGitRepository() bool {
	_, err := gitRunner().Run("rev-parse", "--is-inside-work-tree")
	return err == nil
}

func CheckGitRepository() error {
	if !IsGitRepository() {
		return errors.New("not in a git repository. Please run this command in a git repository directory")
	}
	return nil
}

func GetDiff() (string, error) {
	if err := CheckGitRepository(); err != nil {
		return "", err
	}

	runner := gitRunner()
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

func GetStagedDiff() (string, error) {
	if err := CheckGitRepository(); err != nil {
		return "", err
	}

	result, err := gitRunner().RunLogged("diff", "--cached")
	if err != nil {
		if Verbose && len(result.Stderr) > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", string(result.Stderr))
		}
		return "", fmt.Errorf("failed to run git diff --cached: %w", err)
	}

	return string(result.Stdout), nil
}

func ParseChangedFiles() ([]string, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, err
	}

	runner := gitRunner()
	result, err := runner.Run("diff", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("failed to run git diff --name-only: %w", err)
	}

	unstaged := strings.Split(strings.TrimSpace(string(result.Stdout)), "\n")

	result, err = runner.Run("diff", "--cached", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("failed to run git diff --cached --name-only: %w", err)
	}

	staged := strings.Split(strings.TrimSpace(string(result.Stdout)), "\n")

	fileMap := make(map[string]bool)
	for _, file := range unstaged {
		if file != "" {
			fileMap[file] = true
		}
	}

	for _, file := range staged {
		if file != "" {
			fileMap[file] = true
		}
	}

	changedFiles := make([]string, 0, len(fileMap))
	for file := range fileMap {
		changedFiles = append(changedFiles, file)
	}

	return changedFiles, nil
}

func ParseStagedFiles() ([]string, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, err
	}

	runResult, err := gitRunner().RunLogged("diff", "--cached", "--name-only")
	if err != nil {
		if Verbose && len(runResult.Stderr) > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", string(runResult.Stderr))
		}
		return nil, fmt.Errorf("failed to run git diff --cached --name-only: %w", err)
	}

	stagedFiles := strings.Split(strings.TrimSpace(string(runResult.Stdout)), "\n")

	var files []string
	for _, file := range stagedFiles {
		if file != "" {
			files = append(files, file)
		}
	}

	return files, nil
}

func AddAll() error {
	if err := CheckGitRepository(); err != nil {
		return err
	}

	result, err := gitRunner().RunLogged("add", ".")
	if err != nil {
		if Verbose && len(result.Stderr) > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", string(result.Stderr))
		}
		return fmt.Errorf("failed to run git add .: %w", err)
	}

	if Verbose && len(result.Stderr) > 0 {
		fmt.Fprintln(os.Stderr, "Git output:", string(result.Stderr))
	}

	return nil
}

func Commit(message string, args ...string) error {
	if err := CheckGitRepository(); err != nil {
		return err
	}

	// Safety check: In test environment, prevent commits unless in temp directory
	if os.Getenv("GO_TEST_ENV") == "1" {
		cwd, _ := os.Getwd()
		if !strings.Contains(cwd, "/tmp/") && !strings.Contains(cwd, "\\Temp\\") &&
			!strings.Contains(cwd, "gmc_git_test") && !strings.Contains(cwd, "gmc_non_git_test") {
			return errors.New("SAFETY: refusing to commit in non-temporary directory during tests")
		}
	}

	commitArgs := append([]string{"commit", "-m", message}, args...)
	result, err := gitRunner().RunLogged(commitArgs...)

	// Always show output in verbose mode
	if Verbose {
		if len(result.Stdout) > 0 {
			fmt.Fprintln(os.Stderr, "Git output:", string(result.Stdout))
		}
		if len(result.Stderr) > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", string(result.Stderr))
		}
	}

	if err != nil {
		// Include git error output in the error message
		errMsg := "Failed to run git commit"
		if len(result.Stderr) > 0 {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(string(result.Stderr)))
		}
		return fmt.Errorf("%s: %w", errMsg, err)
	}

	return nil
}

func CreateAndSwitchBranch(branchName string) error {
	if err := CheckGitRepository(); err != nil {
		return err
	}

	if err := validateBranchName(branchName); err != nil {
		return err
	}

	if exists, err := branchExists(branchName); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("branch '%s' already exists", branchName)
	}

	return createAndSwitchBranch(branchName)
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

func branchExists(branchName string) (bool, error) {
	if Verbose {
		fmt.Fprintf(os.Stderr, "Checking if branch exists: git rev-parse --verify %s\n", branchName)
	}

	_, err := gitRunner().Run("rev-parse", "--verify", branchName)
	return err == nil, nil
}

func createAndSwitchBranch(branchName string) error {
	if Verbose {
		fmt.Fprintf(os.Stderr, "Creating and switching to branch: git checkout -b %s\n", branchName)
	}

	result, err := gitRunner().Run("checkout", "-b", branchName)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create and switch to branch '%s'", branchName)
		if len(result.Stderr) > 0 {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(string(result.Stderr)))
		}
		return fmt.Errorf("%s: %w", errMsg, err)
	}

	if Verbose && len(result.Stdout) > 0 {
		fmt.Fprintln(os.Stderr, "Git output:", string(result.Stdout))
	}

	return nil
}

// GetLatestTag returns the most recently created tag in the repository.
func GetLatestTag() (string, error) {
	if err := CheckGitRepository(); err != nil {
		return "", err
	}

	result, err := gitRunner().RunLogged("tag", "--sort=-creatordate")
	if err != nil {
		return "", fmt.Errorf("failed to list tags: %w", err)
	}

	output := strings.TrimSpace(string(result.Stdout))
	if output == "" {
		return "", nil
	}

	tags := strings.Split(output, "\n")
	return strings.TrimSpace(tags[0]), nil
}

// GetCommitsSinceTag returns the commits between the given tag (exclusive) and HEAD.
// If the tag is empty or not found, all commits up to HEAD are returned.
func GetCommitsSinceTag(tag string) ([]CommitInfo, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, err
	}

	format := "%H%x1f%an%x1f%ad%x1f%s%x1f%b%x1e"
	args := []string{"log", "--pretty=format:" + format, "--date=short"}

	if tag != "" {
		exists, err := tagExists(tag)
		if err != nil {
			return nil, err
		}
		if exists {
			args = append(args, tag+"..HEAD")
		}
	}

	result, err := gitRunner().RunLogged(args...)
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
func CreateAnnotatedTag(tag string, message string) error {
	if err := CheckGitRepository(); err != nil {
		return err
	}

	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("tag name cannot be empty")
	}

	if message == "" {
		message = "Release " + tag
	}

	if Verbose {
		fmt.Fprintf(os.Stderr, "Running: git tag -a %s -m %q\n", tag, message)
	}

	result, err := gitRunner().Run("tag", "-a", tag, "-m", message)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create tag '%s'", tag)
		if len(result.Stderr) > 0 {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(string(result.Stderr)))
		}
		return fmt.Errorf("%s: %w", errMsg, err)
	}

	return nil
}

func tagExists(tag string) (bool, error) {
	if tag == "" {
		return false, nil
	}

	ref := "refs/tags/" + tag
	_, err := gitRunner().Run("rev-parse", "--verify", ref)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify tag %s: %w", tag, err)
	}

	return true, nil
}

// GetCommitHistory retrieves commit history with different modes
func GetCommitHistory(limit int, teamMode bool) ([]CommitInfo, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, err
	}

	var args []string
	if teamMode {
		// Team mode: get commits from all authors
		args = []string{"log", "--pretty=format:%h|%an|%ad|%s", "--date=short", fmt.Sprintf("-n%d", limit)}
	} else {
		// Personal mode: get commits from current user only
		currentUser, err := getCurrentGitUser()
		if err != nil {
			return nil, fmt.Errorf("failed to get current git user: %w", err)
		}
		args = []string{"log", "--pretty=format:%h|%an|%ad|%s", "--date=short",
			"--author=" + currentUser, fmt.Sprintf("-n%d", limit)}
	}

	result, err := gitRunner().RunLogged(args...)
	if err != nil {
		if Verbose && len(result.Stderr) > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", string(result.Stderr))
		}
		return nil, fmt.Errorf("failed to run git log: %w", err)
	}

	output := strings.TrimSpace(string(result.Stdout))
	if output == "" {
		return []CommitInfo{}, nil
	}

	return parseCommitOutput(output)
}

// getCurrentGitUser gets the current git user name
func getCurrentGitUser() (string, error) {
	result, err := gitRunner().Run("config", "user.name")
	if err != nil {
		return "", fmt.Errorf("failed to get git user name: %w", err)
	}

	return strings.TrimSpace(string(result.Stdout)), nil
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
func ResolveFiles(paths []string) ([]string, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, err
	}

	var resolvedFiles []string
	for _, path := range paths {
		// Clean path to prevent directory traversal
		cleanPath := filepath.Clean(path)

		info, err := os.Stat(cleanPath)
		if err != nil {
			if os.IsNotExist(err) {
				inIndex, indexErr := isPathInStagedDiff(cleanPath)
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
			dirFiles, err := getGitTrackedFilesInDir(cleanPath)
			if err != nil {
				return nil, fmt.Errorf("failed to get files in directory %s: %w", path, err)
			}
			resolvedFiles = append(resolvedFiles, dirFiles...)
		} else {
			resolvedFiles = append(resolvedFiles, cleanPath)
		}
	}

	// Remove duplicates
	fileMap := make(map[string]bool)
	var uniqueFiles []string
	for _, file := range resolvedFiles {
		if !fileMap[file] {
			fileMap[file] = true
			uniqueFiles = append(uniqueFiles, file)
		}
	}

	return uniqueFiles, nil
}

func isPathInStagedDiff(path string) (bool, error) {
	gitPath := filepath.ToSlash(path)

	result, err := gitRunner().RunLogged("diff", "--cached", "--name-only", "--", gitPath)
	if err != nil {
		if Verbose && len(result.Stderr) > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", string(result.Stderr))
		}
		return false, fmt.Errorf("failed to inspect staged diff: %w", err)
	}

	output := strings.TrimSpace(string(result.Stdout))
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
func getGitTrackedFilesInDir(dir string) ([]string, error) {
	runResult, err := gitRunner().RunLogged("ls-files", "--cached", "--others", "--exclude-standard", dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list git files in directory: %w", err)
	}

	output := strings.TrimSpace(string(runResult.Stdout))
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
func CheckFileStatus(files []string) ([]string, []string, []string, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, nil, nil, err
	}

	var staged, modified, untracked []string

	for _, file := range files {
		// Check if file is staged
		isStaged, err := isFileStaged(file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to check staged status for %s: %w", file, err)
		}

		// Check if file is modified
		isModified, err := isFileModified(file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to check modified status for %s: %w", file, err)
		}

		// Check if file is tracked
		isTracked, err := isFileTracked(file)
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
func isFileStaged(file string) (bool, error) {
	result, err := gitRunner().Run("diff", "--cached", "--name-only", file)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(result.Stdout)) != "", nil
}

// isFileModified checks if a file is modified (unstaged changes)
func isFileModified(file string) (bool, error) {
	result, err := gitRunner().Run("diff", "--name-only", file)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(result.Stdout)) != "", nil
}

// isFileTracked checks if a file is tracked by git
func isFileTracked(file string) (bool, error) {
	result, err := gitRunner().Run("ls-files", file)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(result.Stdout)) != "", nil
}

// StageFiles stages specific files
func StageFiles(files []string) error {
	if err := CheckGitRepository(); err != nil {
		return err
	}

	for _, file := range files {
		result, err := gitRunner().RunLogged("add", file)
		if err != nil {
			errMsg := "failed to stage file " + file
			if len(result.Stderr) > 0 {
				errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(string(result.Stderr)))
			}
			return fmt.Errorf("%s: %w", errMsg, err)
		}
	}

	return nil
}

// GetFilesDiff gets diff for specific files
func GetFilesDiff(files []string) (string, error) {
	if err := CheckGitRepository(); err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", nil
	}

	args := []string{"diff", "--cached"}
	args = append(args, "--")
	args = append(args, files...)

	result, err := gitRunner().RunLogged(args...)
	if err != nil {
		if Verbose && len(result.Stderr) > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", string(result.Stderr))
		}
		return "", fmt.Errorf("failed to get diff for files: %w", err)
	}

	return string(result.Stdout), nil
}

// CommitFiles commits specific files only
func CommitFiles(message string, files []string, args ...string) error {
	if err := CheckGitRepository(); err != nil {
		return err
	}

	// Safety check: In test environment, prevent commits unless in temp directory
	if os.Getenv("GO_TEST_ENV") == "1" {
		cwd, _ := os.Getwd()
		if !strings.Contains(cwd, "/tmp/") && !strings.Contains(cwd, "\\Temp\\") &&
			!strings.Contains(cwd, "gmc_git_test") && !strings.Contains(cwd, "gmc_non_git_test") {
			return errors.New("SAFETY: refusing to commit in non-temporary directory during tests")
		}
	}

	commitArgs := []string{"commit", "-m", message}
	commitArgs = append(commitArgs, args...)
	commitArgs = append(commitArgs, "--")
	commitArgs = append(commitArgs, files...)

	result, err := gitRunner().RunLogged(commitArgs...)

	// Always show output in verbose mode
	if Verbose {
		if len(result.Stdout) > 0 {
			fmt.Fprintln(os.Stderr, "Git output:", string(result.Stdout))
		}
		if len(result.Stderr) > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", string(result.Stderr))
		}
	}

	if err != nil {
		// Include git error output in the error message
		errMsg := "Failed to commit files"
		if len(result.Stderr) > 0 {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(string(result.Stderr)))
		}
		return fmt.Errorf("%s: %w", errMsg, err)
	}

	return nil
}
