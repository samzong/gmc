package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/gitcmd"
	"github.com/samzong/gmc/internal/gitutil"
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

// RepoType represents the type of git repository
type RepoType int

const (
	RepoTypeNormal   RepoType = iota // Normal git repository
	RepoTypeBare                     // Bare repository
	RepoTypeWorktree                 // Worktree directory
	RepoTypeUnknown                  // Not a git repository
)

// String returns the string representation of RepoType
func (r RepoType) String() string {
	switch r {
	case RepoTypeNormal:
		return "normal"
	case RepoTypeBare:
		return "bare"
	case RepoTypeWorktree:
		return "worktree"
	default:
		return "unknown"
	}
}

// WorktreeInfo represents information about a worktree
type WorktreeInfo struct {
	Path       string // Absolute path to the worktree
	Branch     string // Branch name
	Commit     string // Current commit hash
	IsPrunable bool   // Can be pruned
	IsLocked   bool   // Is locked
	IsBare     bool   // Is the main bare worktree
}

// AddOptions options for adding a worktree
type AddOptions struct {
	BaseBranch string // Base branch to create from
	Fetch      bool   // Whether to fetch before creating
}

// RemoveOptions options for removing a worktree
type RemoveOptions struct {
	Force        bool // Force removal even if dirty
	DeleteBranch bool // Also delete the branch
	DryRun       bool // Preview what would be done without making changes
}

// DetectRepositoryType detects the type of git repository in the current or specified directory
func (c *Client) DetectRepositoryType(dir string) (RepoType, error) {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return RepoTypeUnknown, fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Check if it's inside a git repository
	if c.isInsideWorkTree(dir) {
		// It's a work tree, check if it's a worktree or normal repo
		commonDir := c.getGitOutput(dir, "rev-parse", "--git-common-dir")
		gitDir := c.getGitOutput(dir, "rev-parse", "--git-dir")

		// If git-dir != git-common-dir, it's a worktree
		if commonDir != "" && gitDir != "" && gitDir != commonDir && gitDir != "." {
			return RepoTypeWorktree, nil
		}
		return RepoTypeNormal, nil
	}

	// Not inside work tree, check if it's a bare repository
	if c.isBareRepository(dir) {
		return RepoTypeBare, nil
	}

	return RepoTypeUnknown, nil
}

// isInsideWorkTree checks if the directory is inside a git work tree
func (c *Client) isInsideWorkTree(dir string) bool {
	result, err := c.runner.Run("-C", dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && result.StdoutString(true) == "true"
}

// isBareRepository checks if the directory is a bare git repository
func (c *Client) isBareRepository(dir string) bool {
	result, err := c.runner.Run("-C", dir, "rev-parse", "--is-bare-repository")
	return err == nil && result.StdoutString(true) == "true"
}

// getGitOutput runs a git command and returns the trimmed output, or empty string on error
func (c *Client) getGitOutput(dir string, args ...string) string {
	fullArgs := append([]string{"-C", dir}, args...)
	result, err := c.runner.Run(fullArgs...)
	if err != nil {
		return ""
	}
	return result.StdoutString(true)
}

// FindBareRoot finds the root directory containing .bare
func FindBareRoot(startDir string) (string, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	dir := startDir
	for {
		bareDir := filepath.Join(dir, ".bare")
		if info, err := os.Stat(bareDir); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("no .bare directory found in parent directories")
}

// GetWorktreeRoot returns the root directory for worktrees (parent of .bare)
func (c *Client) GetWorktreeRoot() (string, error) {
	// First try to find .bare directory
	root, err := FindBareRoot("")
	if err == nil {
		return root, nil
	}

	// Fall back to git-common-dir
	result, err := c.runner.Run("rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}

	commonDir := result.StdoutString(true)
	if commonDir == "" {
		return "", errors.New("failed to determine git common directory")
	}

	// Get the absolute path
	absCommonDir, err := filepath.Abs(commonDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// If it ends with .bare, return parent
	if filepath.Base(absCommonDir) == ".bare" {
		return filepath.Dir(absCommonDir), nil
	}

	// Otherwise return parent of .git
	return filepath.Dir(absCommonDir), nil
}

// IsBareWorktree checks if the current repository uses the .bare worktree pattern
func (c *Client) IsBareWorktree() bool {
	_, err := FindBareRoot("")
	return err == nil
}

// List returns all worktrees for the current repository
func (c *Client) List() ([]WorktreeInfo, error) {
	// Find the bare root to support running from any directory
	root, err := FindBareRoot("")
	if err != nil {
		// Fallback to current directory if not in a bare repo structure
		result, err := c.runner.RunLogged("worktree", "list", "--porcelain")
		if err != nil {
			return nil, fmt.Errorf("failed to list worktrees: %w", err)
		}
		return parseWorktreeList(string(result.Stdout))
	}

	// Run git command from the .bare directory
	bareDir := filepath.Join(root, ".bare")
	result, err := c.runner.RunLogged("-C", bareDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return parseWorktreeList(string(result.Stdout))
}

// parseWorktreeList parses the porcelain output of git worktree list
func parseWorktreeList(output string) ([]WorktreeInfo, error) {
	var worktrees []WorktreeInfo
	var current *WorktreeInfo

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current != nil {
				worktrees = append(worktrees, *current)
				current = nil
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			if current != nil {
				worktrees = append(worktrees, *current)
			}
			current = &WorktreeInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if current != nil {
			switch {
			case strings.HasPrefix(line, "HEAD "):
				current.Commit = strings.TrimPrefix(line, "HEAD ")
			case strings.HasPrefix(line, "branch "):
				branch := strings.TrimPrefix(line, "branch ")
				// Remove refs/heads/ prefix
				current.Branch = strings.TrimPrefix(branch, "refs/heads/")
			case line == "bare":
				current.IsBare = true
			case line == "prunable":
				current.IsPrunable = true
			case line == "locked":
				current.IsLocked = true
			case strings.HasPrefix(line, "detached"):
				current.Branch = "(detached)"
			}
		}
	}

	if current != nil {
		worktrees = append(worktrees, *current)
	}

	return worktrees, nil
}

// Add creates a new worktree with a new branch
func (c *Client) Add(name string, opts AddOptions) error {
	if name == "" {
		return errors.New("worktree name cannot be empty")
	}

	// Validate branch name
	if err := validateBranchName(name); err != nil {
		return err
	}

	// Find the worktree root
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return fmt.Errorf("failed to find worktree root: %w", err)
	}

	// Target path for the new worktree
	targetPath := filepath.Join(root, name)

	// Check if directory already exists
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("directory already exists: %s", targetPath)
	}

	// Determine base branch
	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		// Default to HEAD
		baseBranch = "HEAD"
	}

	// Optionally fetch first
	if opts.Fetch {
		if c.verbose {
			fmt.Fprintln(os.Stderr, "Fetching latest changes...")
		}
		_ = c.runner.RunWithWriters(false, nil, os.Stderr, "fetch", "--all") // Ignore fetch errors
	}

	// Check if branch already exists
	var args []string
	branchExistsFlag, _ := c.branchExists(name)
	if branchExistsFlag {
		// Branch exists: create worktree from existing branch
		args = []string{"worktree", "add", targetPath, name}
	} else {
		// Branch does not exist: create new branch
		args = []string{"worktree", "add", "-b", name, targetPath, baseBranch}
	}

	result, err := c.runner.RunLogged(args...)
	if err != nil {
		return gitutil.WrapGitError("failed to create worktree", result, err)
	}

	// Sync shared resources
	if err := c.SyncSharedResources(name); err != nil {
		fmt.Printf("Warning: failed to sync shared resources: %v\n", err)
	}

	if branchExistsFlag {
		fmt.Printf("Created worktree '%s' at %s\n", name, targetPath)
		fmt.Printf("Branch: %s (existing)\n", name)
	} else {
		fmt.Printf("Created worktree '%s' at %s\n", name, targetPath)
		fmt.Printf("Branch: %s (based on %s)\n", name, baseBranch)
	}
	fmt.Printf("Next step: cd %s\n", targetPath)

	return nil
}

// Remove removes a worktree
func (c *Client) Remove(name string, opts RemoveOptions) error {
	if name == "" {
		return errors.New("worktree name cannot be empty")
	}

	// Find the worktree root
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return fmt.Errorf("failed to find worktree root: %w", err)
	}

	// Target path
	targetPath := filepath.Join(root, name)

	// Check if worktree exists
	worktrees, err := c.List()
	if err != nil {
		return err
	}

	var found bool
	var wtInfo WorktreeInfo
	for _, wt := range worktrees {
		if wt.Path == targetPath || filepath.Base(wt.Path) == name {
			found = true
			wtInfo = wt
			targetPath = wt.Path
			break
		}
	}

	if !found {
		return fmt.Errorf("worktree not found: %s", name)
	}

	if wtInfo.IsBare {
		return errors.New("cannot remove the main bare worktree")
	}

	// Dry run: preview what would be done
	if opts.DryRun {
		status := c.GetWorktreeStatus(targetPath)
		fmt.Fprintf(os.Stderr, "Would remove worktree: %s\n", targetPath)
		fmt.Fprintf(os.Stderr, "  Branch: %s\n", wtInfo.Branch)
		fmt.Fprintf(os.Stderr, "  Status: %s\n", status)
		if opts.DeleteBranch && wtInfo.Branch != "" && wtInfo.Branch != "(detached)" {
			fmt.Fprintf(os.Stderr, "Would delete branch: %s\n", wtInfo.Branch)
		}
		if status == "modified" && !opts.Force {
			fmt.Fprintln(os.Stderr, "Note: Worktree has uncommitted changes. Use -f to force removal.")
		}
		return nil
	}

	// Remove worktree
	args := []string{"worktree", "remove"}
	if opts.Force {
		args = append(args, "--force")
	}
	args = append(args, targetPath)

	result, err := c.runner.RunLogged(args...)
	if err != nil {
		return gitutil.WrapGitError("failed to remove worktree", result, err)
	}

	fmt.Fprintf(os.Stderr, "Removed worktree '%s'\n", name)

	// Optionally delete branch
	if opts.DeleteBranch && wtInfo.Branch != "" && wtInfo.Branch != "(detached)" {
		args := []string{"branch", "-D", wtInfo.Branch}
		result, err := c.runner.RunLogged(args...)
		if err != nil {
			return gitutil.WrapGitError("failed to delete branch", result, err)
		}

		fmt.Fprintf(os.Stderr, "Deleted branch '%s'\n", wtInfo.Branch)
	}

	return nil
}

// GetWorktreeStatus returns the git status of a worktree (clean/modified)
func (c *Client) GetWorktreeStatus(path string) string {
	result, err := c.runner.Run("-C", path, "status", "--porcelain")
	if err != nil {
		return "unknown"
	}

	if result.StdoutString(true) == "" {
		return "clean"
	}
	return "modified"
}

// validateBranchName validates a git branch name
func validateBranchName(name string) error {
	if name == "" {
		return errors.New("branch name cannot be empty")
	}

	if strings.Contains(name, "..") || strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid branch name: '%s'", name)
	}

	// Check for invalid characters
	invalidChars := []string{" ", "~", "^", ":", "?", "*", "[", "\\"}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return fmt.Errorf("invalid character '%s' in branch name", char)
		}
	}

	return nil
}

// branchExists checks if a branch exists in the repository
func (c *Client) branchExists(name string) (bool, error) {
	// Try to find the bare repo root for proper -C path
	root, _ := c.GetWorktreeRoot()

	var args []string
	if root == "" {
		// Fallback to current directory
		args = []string{"rev-parse", "--verify", "refs/heads/" + name}
	} else {
		// Check if .bare directory exists
		bareDir := filepath.Join(root, ".bare")
		if _, statErr := os.Stat(bareDir); statErr == nil {
			args = []string{"-C", bareDir, "rev-parse", "--verify", "refs/heads/" + name}
		} else {
			// Standard repo
			args = []string{"-C", root, "rev-parse", "--verify", "refs/heads/" + name}
		}
	}

	_, err := c.runner.Run(args...)
	return err == nil, nil
}

// DupOptions options for duplicating worktrees
type DupOptions struct {
	BaseBranch string // Base branch to create from
	Count      int    // Number of worktrees to create
}

// DupResult result of a dup operation
type DupResult struct {
	Worktrees []string // Created worktree directories
	Branches  []string // Created branch names
}

// Dup creates multiple worktrees with temporary branches for parallel development
func (c *Client) Dup(opts DupOptions) (*DupResult, error) {
	if opts.BaseBranch == "" {
		opts.BaseBranch = "HEAD"
	}
	if opts.Count < 1 {
		opts.Count = 2
	}

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find worktree root: %w", err)
	}

	timestamp := strconv.FormatInt(getCurrentTimestamp(), 10)
	dupResult := &DupResult{
		Worktrees: make([]string, 0, opts.Count),
		Branches:  make([]string, 0, opts.Count),
	}

	for i := 1; i <= opts.Count; i++ {
		dirName := fmt.Sprintf(".dup-%d", i)
		branchName := fmt.Sprintf("_dup/%s/%s-%d", opts.BaseBranch, timestamp, i)
		targetPath := filepath.Join(root, dirName)

		// Check if directory already exists
		if _, err := os.Stat(targetPath); err == nil {
			return nil, fmt.Errorf("directory already exists: %s", targetPath)
		}

		// Create worktree with new branch
		args := []string{"worktree", "add", "-b", branchName, targetPath, opts.BaseBranch}
		runResult, err := c.runner.RunLogged(args...)
		if err != nil {
			return nil, gitutil.WrapGitError(fmt.Sprintf("failed to create worktree %s", dirName), runResult, err)
		}

		// Sync shared resources
		if err := c.SyncSharedResources(dirName); err != nil {
			fmt.Printf("Warning: failed to sync shared resources for %s: %v\n", dirName, err)
		}

		dupResult.Worktrees = append(dupResult.Worktrees, dirName)
		dupResult.Branches = append(dupResult.Branches, branchName)
	}

	return dupResult, nil
}

// Promote renames the branch of a worktree to a permanent name
func (c *Client) Promote(worktreeName, newBranchName string) error {
	if worktreeName == "" {
		return errors.New("worktree name cannot be empty")
	}
	if newBranchName == "" {
		return errors.New("branch name cannot be empty")
	}

	if err := validateBranchName(newBranchName); err != nil {
		return err
	}

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return fmt.Errorf("failed to find worktree root: %w", err)
	}

	targetPath := filepath.Join(root, worktreeName)

	// Verify worktree exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return fmt.Errorf("worktree not found: %s", worktreeName)
	}

	// Get current branch name
	result, err := c.runner.Run("-C", targetPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	oldBranch := result.StdoutString(true)
	if oldBranch == "HEAD" {
		return errors.New("worktree is in detached HEAD state, cannot promote")
	}

	// Rename branch
	args := []string{"-C", targetPath, "branch", "-m", newBranchName}
	result, err = c.runner.RunLogged(args...)
	if err != nil {
		return gitutil.WrapGitError("failed to rename branch", result, err)
	}

	fmt.Printf("Promoted '%s' -> '%s'\n", oldBranch, newBranchName)
	return nil
}

// getCurrentTimestamp returns current unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}
