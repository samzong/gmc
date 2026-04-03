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

// Info represents information about a worktree
type Info struct {
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

type addContext struct {
	name       string
	repoDir    string
	targetPath string
	baseBranch string
}

type removeContext struct {
	name       string
	repoDir    string
	targetPath string
	wtInfo     Info
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
		if filepath.Base(dir) == ".bare" {
			return filepath.Dir(dir), nil
		}
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

// GetGitCommonDir returns the absolute shared git directory for the current repository/worktree.
func (c *Client) GetGitCommonDir() (string, error) {
	result, err := c.runner.Run("rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("not in a git repository: %w", err)
	}

	commonDir := result.StdoutString(true)
	if commonDir == "" {
		return "", errors.New("failed to determine git common directory")
	}

	if filepath.IsAbs(commonDir) {
		return filepath.Clean(commonDir), nil
	}

	absCommonDir, err := filepath.Abs(commonDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}
	return absCommonDir, nil
}

// GetRepoRoot returns the main worktree/repository root for the current repository family.
func (c *Client) GetRepoRoot() (string, error) {
	// First try bare-root discovery for gmc's preferred layout.
	root, err := FindBareRoot("")
	if err == nil {
		return root, nil
	}

	commonDir, err := c.GetGitCommonDir()
	if err != nil {
		return "", err
	}

	if filepath.Base(commonDir) == ".bare" {
		return filepath.Dir(commonDir), nil
	}
	return filepath.Dir(commonDir), nil
}

// GetWorktreeRoot returns the root directory for worktrees (parent of .bare or main repo root).
func (c *Client) GetWorktreeRoot() (string, error) {
	return c.GetRepoRoot()
}

// IsBareWorktree checks if the current repository uses the .bare worktree pattern
func (c *Client) IsBareWorktree() bool {
	_, err := FindBareRoot("")
	return err == nil
}

// List returns all worktrees for the current repository
func (c *Client) List() ([]Info, error) {
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
func parseWorktreeList(output string) ([]Info, error) {
	var worktrees []Info
	var current *Info

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
			current = &Info{
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
func (c *Client) Add(name string, opts AddOptions) (Report, error) {
	var report Report

	ctx, err := c.prepareAdd(name, opts)
	if err != nil {
		return report, err
	}

	c.maybeFetchForAdd(ctx, opts, &report)
	args, branchExists := c.addArgs(ctx)
	result, err := c.runner.RunLogged(args...)
	if err != nil {
		return report, gitutil.WrapGitError("failed to create worktree", result, err)
	}

	// Pass the directory name (base of targetPath), not the branch name,
	// since SyncSharedResources resolves by worktree path.
	sharedReport, err := c.SyncSharedResources(filepath.Base(ctx.targetPath))
	report.Merge(sharedReport)
	if err != nil {
		report.Warn(fmt.Sprintf("Warning: failed to sync shared resources: %v", err))
	}

	c.appendAddSummary(&report, ctx, branchExists)
	return report, nil
}

// Remove removes a worktree
func (c *Client) Remove(name string, opts RemoveOptions) (Report, error) {
	var report Report

	ctx, err := c.prepareRemove(name)
	if err != nil {
		return report, err
	}

	if opts.DryRun {
		status := c.GetWorktreeStatus(ctx.targetPath)
		report.Warn("Would remove worktree: " + ctx.targetPath)
		report.Warn("  Branch: " + ctx.wtInfo.Branch)
		report.Warn("  Status: " + status)
		if opts.DeleteBranch && ctx.wtInfo.Branch != "" && ctx.wtInfo.Branch != "(detached)" {
			report.Warn("Would delete branch: " + ctx.wtInfo.Branch)
		}
		if status == "modified" && !opts.Force {
			report.Warn("Note: Worktree has uncommitted changes. Use -f to force removal.")
		}
		return report, nil
	}

	args := []string{"-C", ctx.repoDir, "worktree", "remove"}
	if opts.Force {
		args = append(args, "--force")
	}
	args = append(args, ctx.targetPath)

	result, err := c.runner.RunLogged(args...)
	if err != nil {
		return report, gitutil.WrapGitError("failed to remove worktree", result, err)
	}

	report.Warn(fmt.Sprintf("Removed worktree '%s'", ctx.name))

	if opts.DeleteBranch && ctx.wtInfo.Branch != "" && ctx.wtInfo.Branch != "(detached)" {
		args := []string{"-C", ctx.repoDir, "branch", "-D", ctx.wtInfo.Branch}
		result, err := c.runner.RunLogged(args...)
		if err != nil {
			return report, gitutil.WrapGitError("failed to delete branch", result, err)
		}

		report.Warn(fmt.Sprintf("Deleted branch '%s'", ctx.wtInfo.Branch))
	}

	return report, nil
}

func (c *Client) prepareAdd(name string, opts AddOptions) (addContext, error) {
	if name == "" {
		return addContext{}, errors.New("worktree name cannot be empty")
	}
	if err := gitutil.ValidateBranchName(name); err != nil {
		return addContext{}, err
	}

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return addContext{}, fmt.Errorf("failed to find worktree root: %w", err)
	}

	// Directory name: replace "/" with "--" so branch names like "feat/login"
	// become "feat--login" instead of a nested directory.
	dirName := strings.ReplaceAll(name, "/", "--")

	// Build target path:
	//   bare layout  → <root>/<dirName>           (sibling of .bare)
	//   non-bare     → <parent>/<repoName>--<dirName>  (sibling of the repo)
	var targetPath string
	repoDir := repoDirForGit(root)
	if repoDir != root { // bare layout: .bare exists
		targetPath = filepath.Join(root, dirName)
	} else {
		// Non-bare: create as a sibling of the repo, prefixed with the repo name.
		targetPath = filepath.Join(filepath.Dir(root), filepath.Base(root)+"--"+dirName)
	}

	if _, err := os.Stat(targetPath); err == nil {
		return addContext{}, fmt.Errorf("directory already exists: %s", targetPath)
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = "HEAD"
	}

	return addContext{
		name:       name,
		repoDir:    repoDir,
		targetPath: targetPath,
		baseBranch: baseBranch,
	}, nil
}

func (c *Client) maybeFetchForAdd(ctx addContext, opts AddOptions, report *Report) {
	if !opts.Fetch {
		return
	}
	report.Info("Fetching latest changes...")
	_ = c.runner.RunStreamingLogged("-C", ctx.repoDir, "fetch", "--all")
}

func (c *Client) addArgs(ctx addContext) ([]string, bool) {
	branchExists, _ := c.branchExists(ctx.name)
	if branchExists {
		return []string{"-C", ctx.repoDir, "worktree", "add", ctx.targetPath, ctx.name}, true
	}
	return []string{"-C", ctx.repoDir, "worktree", "add", "-b", ctx.name, ctx.targetPath, ctx.baseBranch}, false
}

func (c *Client) appendAddSummary(report *Report, ctx addContext, branchExists bool) {
	report.Info(fmt.Sprintf("Created worktree '%s' at %s", ctx.name, ctx.targetPath))
	if branchExists {
		report.Info(fmt.Sprintf("Branch: %s (existing)", ctx.name))
	} else {
		report.Info(fmt.Sprintf("Branch: %s (based on %s)", ctx.name, ctx.baseBranch))
	}
	report.Info("Next step: cd " + ctx.targetPath)
}

func (c *Client) prepareRemove(name string) (removeContext, error) {
	if name == "" {
		return removeContext{}, errors.New("worktree name cannot be empty")
	}

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return removeContext{}, fmt.Errorf("failed to find worktree root: %w", err)
	}

	// searchRoot is where linked worktrees live (parent of repo for non-bare).
	searchRoot, err := c.worktreeSearchRoot()
	if err != nil {
		return removeContext{}, fmt.Errorf("failed to determine worktree search root: %w", err)
	}

	targetPath := filepath.Join(searchRoot, name)
	worktrees, err := c.List()
	if err != nil {
		return removeContext{}, err
	}

	var found bool
	var wtInfo Info
	for _, wt := range worktrees {
		relPath := strings.TrimPrefix(wt.Path, searchRoot+string(filepath.Separator))
		if wt.Path == targetPath || relPath == name {
			wtInfo = wt
			targetPath = wt.Path
			found = true
			break
		}
	}
	if !found {
		return removeContext{}, fmt.Errorf("worktree not found: %s\nUse 'gmc wt ls' to see available worktrees", name)
	}
	pp, err := c.NewProtectionPolicy()
	if err != nil {
		return removeContext{}, err
	}
	if pp.IsProtected(wtInfo) {
		return removeContext{}, fmt.Errorf("cannot remove protected worktree '%s' (%s)", name, pp.Reason(wtInfo))
	}

	// Reject agent/external worktrees (outside searchRoot).
	rel, err := filepath.Rel(searchRoot, wtInfo.Path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return removeContext{}, fmt.Errorf("worktree '%s' is external (not managed by gmc wt)", name)
	}

	return removeContext{
		name:       name,
		repoDir:    repoDirForGit(root),
		targetPath: targetPath,
		wtInfo:     wtInfo,
	}, nil
}

// GetWorktreeStatus returns the git status of a worktree with detailed file counts
func (c *Client) GetWorktreeStatus(path string) string {
	result, err := c.runner.Run("-C", path, "status", "--porcelain")
	if err != nil {
		return "unknown"
	}

	output := result.StdoutString(true)
	if output == "" {
		return "clean"
	}

	// Parse porcelain output to count changes
	lines := strings.Split(output, "\n")
	var modified, untracked int

	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		// Porcelain format: XY filename
		// X = index status, Y = working tree status
		if line[:2] == "??" {
			untracked++
		} else {
			modified++
		}
	}

	// Build status message
	var parts []string
	if modified > 0 {
		if modified == 1 {
			parts = append(parts, "1 file changed")
		} else {
			parts = append(parts, fmt.Sprintf("%d files changed", modified))
		}
	}
	if untracked > 0 {
		if untracked == 1 {
			parts = append(parts, "1 untracked")
		} else {
			parts = append(parts, fmt.Sprintf("%d untracked", untracked))
		}
	}

	if len(parts) == 0 {
		return "modified"
	}
	return strings.Join(parts, ", ")
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
	Warnings  []string // Non-fatal warnings generated during creation
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
	repoDir := repoDirForGit(root)

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
		args := []string{"-C", repoDir, "worktree", "add", "-b", branchName, targetPath, opts.BaseBranch}
		runResult, err := c.runner.RunLogged(args...)
		if err != nil {
			return nil, gitutil.WrapGitError("failed to create worktree "+dirName, runResult, err)
		}

		// Sync shared resources
		sharedReport, err := c.SyncSharedResources(dirName)
		if err != nil {
			dupResult.Warnings = append(
				dupResult.Warnings,
				fmt.Sprintf("Warning: failed to sync shared resources for %s: %v", dirName, err),
			)
		}
		for _, event := range sharedReport.Events {
			if event.Level == EventWarn {
				dupResult.Warnings = append(dupResult.Warnings, event.Message)
			}
		}

		dupResult.Worktrees = append(dupResult.Worktrees, dirName)
		dupResult.Branches = append(dupResult.Branches, branchName)
	}

	return dupResult, nil
}

// Promote renames the branch of a worktree to a permanent name
func (c *Client) Promote(worktreeName, newBranchName string) (Report, error) {
	var report Report

	if worktreeName == "" {
		return report, errors.New("worktree name cannot be empty")
	}
	if newBranchName == "" {
		return report, errors.New("branch name cannot be empty")
	}

	if err := gitutil.ValidateBranchName(newBranchName); err != nil {
		return report, err
	}

	searchRoot, err := c.worktreeSearchRoot()
	if err != nil {
		return report, fmt.Errorf("failed to determine worktree search root: %w", err)
	}

	targetPath := filepath.Join(searchRoot, worktreeName)

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return report, fmt.Errorf("worktree not found: %s", worktreeName)
	}

	result, err := c.runner.Run("-C", targetPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return report, fmt.Errorf("failed to get current branch: %w", err)
	}

	oldBranch := result.StdoutString(true)
	if oldBranch == "HEAD" {
		return report, errors.New("worktree is in detached HEAD state, cannot promote")
	}

	pp, err := c.NewProtectionPolicy()
	if err != nil {
		return report, err
	}
	checkWt := Info{Path: targetPath, Branch: oldBranch}
	if pp.IsProtected(checkWt) {
		return report, fmt.Errorf("cannot promote protected worktree '%s' (%s)", worktreeName, pp.Reason(checkWt))
	}

	// Rename branch
	args := []string{"-C", targetPath, "branch", "-m", newBranchName}
	result, err = c.runner.RunLogged(args...)
	if err != nil {
		return report, gitutil.WrapGitError("failed to rename branch", result, err)
	}

	report.Info(fmt.Sprintf("Promoted '%s' -> '%s'", oldBranch, newBranchName))
	return report, nil
}

// getCurrentTimestamp returns current unix timestamp
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// worktreeSearchRoot returns the directory in which linked worktrees are expected to live.
// Bare layout: root (parent of .bare) — managed worktrees are direct children.
// Non-bare layout: parent of the repo dir — linked worktrees can be siblings of the repo.
func (c *Client) worktreeSearchRoot() (string, error) {
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return "", err
	}
	repoDir := repoDirForGit(root)
	if repoDir != root { // bare layout: .bare exists
		return root, nil
	}
	return filepath.Dir(root), nil
}

// isExternalPath reports whether wtPath is outside the managed root directory.
func isExternalPath(root, wtPath string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, wtPath)
	if err != nil {
		return true
	}
	return strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".."
}

// listGitRefs runs a git command in the repo dir and splits output by newline.
func (c *Client) listGitRefs(errLabel string, gitArgs ...string) ([]string, error) {
	root, _ := c.GetWorktreeRoot()
	repoDir := repoDirForGit(root)

	var args []string
	if repoDir != "" {
		args = append([]string{"-C", repoDir}, gitArgs...)
	} else {
		args = gitArgs
	}

	result, err := c.runner.Run(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to %s: %w", errLabel, err)
	}

	output := result.StdoutString(true)
	if output == "" {
		return nil, nil
	}

	return strings.Split(output, "\n"), nil
}

type ProtectionPolicy struct {
	MainBranch string
	RootPath   string
}

func (c *Client) NewProtectionPolicy() (ProtectionPolicy, error) {
	var p ProtectionPolicy
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return p, fmt.Errorf("failed to get worktree root: %w", err)
	}
	p.RootPath = root
	repoDir := repoDirForGit(root)
	isBareLayout := repoDir != root
	branch, err := c.resolveBaseBranchWithPolicy(repoDir, "", isBareLayout)
	if err != nil {
		return p, fmt.Errorf("failed to resolve main branch: %w", err)
	}
	p.MainBranch = localBranchName(branch)
	return p, nil
}

func (p ProtectionPolicy) IsProtected(wt Info) bool {
	if wt.IsBare {
		return true
	}
	if p.RootPath != "" && wt.Path == p.RootPath {
		return true
	}
	if p.MainBranch != "" && wt.Branch == p.MainBranch {
		return true
	}
	return false
}

func (p ProtectionPolicy) Reason(wt Info) string {
	if wt.IsBare {
		return "bare repository"
	}
	if p.RootPath != "" && wt.Path == p.RootPath {
		return "main worktree"
	}
	return "main branch"
}

func (c *Client) IsProtectedWorktree(wt Info) (bool, error) {
	pp, err := c.NewProtectionPolicy()
	if err != nil {
		return false, err
	}
	return pp.IsProtected(wt), nil
}

func (c *Client) resolvedMainBranch() (string, error) {
	pp, err := c.NewProtectionPolicy()
	if err != nil {
		return "", err
	}
	return pp.MainBranch, nil
}

// ListBranches returns all local branch names
func (c *Client) ListBranches() ([]string, error) {
	return c.listGitRefs("list branches", "branch", "--format=%(refname:short)")
}

// ListRemotes returns all remote names
func (c *Client) ListRemotes() ([]string, error) {
	return c.listGitRefs("list remotes", "remote")
}
