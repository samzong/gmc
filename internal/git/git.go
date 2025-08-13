package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var Verbose bool

// CommitInfo represents information about a single commit
type CommitInfo struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

// IsGitRepository checks if the current directory is a git repository
func IsGitRepository() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	err := cmd.Run()
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

	cmd := exec.Command("git", "diff")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run git diff: %w", err)
	}

	unstaged := out.String()
	out.Reset()

	cmd = exec.Command("git", "diff", "--cached")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run git diff --cached: %w", err)
	}

	staged := out.String()

	diff := unstaged + staged
	return diff, nil
}

func GetStagedDiff() (string, error) {
	if err := CheckGitRepository(); err != nil {
		return "", err
	}

	cmd := exec.Command("git", "diff", "--cached")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if Verbose {
		fmt.Fprintln(os.Stderr, "Running: git diff --cached")
	}

	if err := cmd.Run(); err != nil {
		if Verbose && errBuf.Len() > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", errBuf.String())
		}
		return "", fmt.Errorf("failed to run git diff --cached: %w", err)
	}

	return out.String(), nil
}

func ParseChangedFiles() ([]string, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "diff", "--name-only")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run git diff --name-only: %w", err)
	}

	unstaged := strings.Split(strings.TrimSpace(out.String()), "\n")
	out.Reset()

	cmd = exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run git diff --cached --name-only: %w", err)
	}

	staged := strings.Split(strings.TrimSpace(out.String()), "\n")

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

	var changedFiles []string
	for file := range fileMap {
		changedFiles = append(changedFiles, file)
	}

	return changedFiles, nil
}

func ParseStagedFiles() ([]string, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if Verbose {
		fmt.Fprintln(os.Stderr, "Running: git diff --cached --name-only")
	}

	if err := cmd.Run(); err != nil {
		if Verbose && errBuf.Len() > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", errBuf.String())
		}
		return nil, fmt.Errorf("failed to run git diff --cached --name-only: %w", err)
	}

	stagedFiles := strings.Split(strings.TrimSpace(out.String()), "\n")

	var result []string
	for _, file := range stagedFiles {
		if file != "" {
			result = append(result, file)
		}
	}

	return result, nil
}

func AddAll() error {
	if err := CheckGitRepository(); err != nil {
		return err
	}

	cmd := exec.Command("git", "add", ".")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if Verbose {
		fmt.Fprintln(os.Stderr, "Running: git add .")
	}

	if err := cmd.Run(); err != nil {
		if Verbose && errBuf.Len() > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", errBuf.String())
		}
		return fmt.Errorf("failed to run git add .: %w", err)
	}

	if Verbose && errBuf.Len() > 0 {
		fmt.Fprintln(os.Stderr, "Git output:", errBuf.String())
	}

	return nil
}

func Commit(message string, args ...string) error {
	if err := CheckGitRepository(); err != nil {
		return err
	}

	commitArgs := append([]string{"commit", "-m", message}, args...)
	cmd := exec.Command("git", commitArgs...)

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if Verbose {
		fmt.Fprintf(os.Stderr, "Running: git %s\n", strings.Join(commitArgs, " "))
	}

	err := cmd.Run()

	// Always show output in verbose mode
	if Verbose {
		if outBuf.Len() > 0 {
			fmt.Fprintln(os.Stderr, "Git output:", outBuf.String())
		}
		if errBuf.Len() > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", errBuf.String())
		}
	}

	if err != nil {
		// Include git error output in the error message
		errMsg := "Failed to run git commit"
		if errBuf.Len() > 0 {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(errBuf.String()))
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
	cmd := exec.Command("git", "rev-parse", "--verify", branchName)
	cmd.Stderr = nil // Suppress error output

	if Verbose {
		fmt.Fprintf(os.Stderr, "Checking if branch exists: git rev-parse --verify %s\n", branchName)
	}

	return cmd.Run() == nil, nil
}

func createAndSwitchBranch(branchName string) error {
	cmd := exec.Command("git", "checkout", "-b", branchName)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if Verbose {
		fmt.Fprintf(os.Stderr, "Creating and switching to branch: git checkout -b %s\n", branchName)
	}

	if err := cmd.Run(); err != nil {
		errMsg := fmt.Sprintf("failed to create and switch to branch '%s'", branchName)
		if errBuf.Len() > 0 {
			errMsg = fmt.Sprintf("%s: %s", errMsg, strings.TrimSpace(errBuf.String()))
		}
		return fmt.Errorf("%s: %w", errMsg, err)
	}

	if Verbose && outBuf.Len() > 0 {
		fmt.Fprintln(os.Stderr, "Git output:", outBuf.String())
	}

	return nil
}

// GetCommitHistory retrieves commit history with different modes
func GetCommitHistory(limit int, teamMode bool) ([]CommitInfo, error) {
	if err := CheckGitRepository(); err != nil {
		return nil, err
	}

	var cmd *exec.Cmd
	if teamMode {
		// Team mode: get commits from all authors
		cmd = exec.Command("git", "log", "--pretty=format:%h|%an|%ad|%s", "--date=short", fmt.Sprintf("-n%d", limit))
	} else {
		// Personal mode: get commits from current user only
		currentUser, err := getCurrentGitUser()
		if err != nil {
			return nil, fmt.Errorf("failed to get current git user: %w", err)
		}
		cmd = exec.Command("git", "log", "--pretty=format:%h|%an|%ad|%s", "--date=short",
			fmt.Sprintf("--author=%s", currentUser), fmt.Sprintf("-n%d", limit))
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if Verbose {
		fmt.Fprintf(os.Stderr, "Running: %s\n", strings.Join(cmd.Args, " "))
	}

	if err := cmd.Run(); err != nil {
		if Verbose && errBuf.Len() > 0 {
			fmt.Fprintln(os.Stderr, "Git stderr:", errBuf.String())
		}
		return nil, fmt.Errorf("failed to run git log: %w", err)
	}

	output := strings.TrimSpace(out.String())
	if output == "" {
		return []CommitInfo{}, nil
	}

	return parseCommitOutput(output)
}

// getCurrentGitUser gets the current git user name
func getCurrentGitUser() (string, error) {
	cmd := exec.Command("git", "config", "user.name")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get git user name: %w", err)
	}

	return strings.TrimSpace(out.String()), nil
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
