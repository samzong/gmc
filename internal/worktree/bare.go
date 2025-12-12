package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CloneOptions options for cloning a repository
type CloneOptions struct {
	Name     string // Custom project name
	Upstream string // Upstream URL for fork workflow
}

// Clone clones a repository as a bare + worktree structure
func Clone(repoURL string, opts CloneOptions) error {
	if repoURL == "" {
		return errors.New("repository URL cannot be empty")
	}

	// Determine project name
	projectName := opts.Name
	if projectName == "" {
		var err error
		projectName, err = extractProjectName(repoURL)
		if err != nil {
			return err
		}
	}

	// Check if directory already exists
	if _, err := os.Stat(projectName); err == nil {
		return fmt.Errorf("directory already exists: %s", projectName)
	}

	fmt.Printf("Cloning %s as bare + worktree structure...\n", repoURL)
	fmt.Println()

	// Create project directory
	if err := os.MkdirAll(projectName, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	bareDir := filepath.Join(projectName, ".bare")

	// Clone as bare repository - pass output to user for progress
	args := []string{"clone", "--bare", "--progress", repoURL, bareDir}
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if Verbose {
		fmt.Fprintf(os.Stderr, "Running: git %s\n", strings.Join(args, " "))
	}

	if err := cmd.Run(); err != nil {
		// Clean up on failure
		os.RemoveAll(projectName)
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	fmt.Println()

	// Determine default branch BEFORE configuring (while still bare=true)
	defaultBranch, err := getDefaultBranch(bareDir)
	if err != nil {
		defaultBranch = "main" // Fallback
	}

	// Create the main worktree FIRST (while bare=true, so branch is not "in use")
	absProjectDir, err := filepath.Abs(projectName)
	if err != nil {
		os.RemoveAll(projectName)
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	mainWorktree := filepath.Join(absProjectDir, defaultBranch)
	args = []string{"-C", bareDir, "worktree", "add", mainWorktree, defaultBranch}
	cmd = exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if Verbose {
		fmt.Fprintf(os.Stderr, "Running: git %s\n", strings.Join(args, " "))
	}

	if err := cmd.Run(); err != nil {
		os.RemoveAll(projectName)
		return fmt.Errorf("failed to create main worktree: %w", err)
	}

	// NOW configure the bare repository (after worktree is created)
	if err := configureBareRepo(bareDir, opts.Upstream); err != nil {
		os.RemoveAll(projectName)
		return err
	}

	// Print success message
	fmt.Println()
	fmt.Printf("Successfully cloned to %s/\n", projectName)
	fmt.Println()
	fmt.Println("Directory Structure:")
	fmt.Printf("  %s/\n", projectName)
	fmt.Printf("  ├── .bare/          # Bare repository\n")
	fmt.Printf("  └── %s/           # Main worktree\n", defaultBranch)
	fmt.Println()

	if opts.Upstream != "" {
		fmt.Println("Remote Configuration:")
		fmt.Println("  origin   = " + repoURL + " (your fork)")
		fmt.Println("  upstream = " + opts.Upstream + " (upstream)")
		fmt.Println()
	}

	fmt.Println("Next steps:")
	fmt.Printf("  cd %s/%s\n", projectName, defaultBranch)
	fmt.Println("  gmc wt add feature-name")

	return nil
}

// configureBareRepo configures the bare repository for worktree usage
func configureBareRepo(bareDir string, upstreamURL string) error {
	// Note: We keep core.bare = true (default after git clone --bare)
	// Git worktree works correctly with bare = true

	// Configure fetch refspec for origin (bare clone doesn't set this by default)
	if err := gitConfig(bareDir, "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return fmt.Errorf("failed to configure remote.origin.fetch: %w", err)
	}

	// Fetch to populate refs/remotes/origin/* based on the new refspec
	// Ignore error as network might be flaky and the repo is already usable
	if Verbose {
		fmt.Fprintln(os.Stderr, "Fetching remote references...")
	}
	fetchCmd := exec.Command("git", "-C", bareDir, "fetch", "origin")
	_ = fetchCmd.Run()

	// Add upstream remote if specified
	if upstreamURL != "" {
		args := []string{"-C", bareDir, "remote", "add", "upstream", upstreamURL}
		cmd := exec.Command("git", args...)
		if Verbose {
			fmt.Fprintf(os.Stderr, "Running: git %s\n", strings.Join(args, " "))
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to add upstream remote: %w", err)
		}
	}

	return nil
}

// gitConfig sets a git config value
func gitConfig(repoDir string, key string, value string) error {
	args := []string{"-C", repoDir, "config", key, value}
	cmd := exec.Command("git", args...)
	if Verbose {
		fmt.Fprintf(os.Stderr, "Running: git %s\n", strings.Join(args, " "))
	}
	return cmd.Run()
}

// getDefaultBranch gets the default branch name from a bare repository
func getDefaultBranch(bareDir string) (string, error) {
	// Try to get from HEAD
	args := []string{"-C", bareDir, "symbolic-ref", "--short", "HEAD"}
	cmd := exec.Command("git", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err == nil {
		branch := strings.TrimSpace(out.String())
		if branch != "" {
			return branch, nil
		}
	}

	// Fallback: check common branch names
	for _, branch := range []string{"main", "master"} {
		args := []string{"-C", bareDir, "rev-parse", "--verify", "refs/heads/" + branch}
		cmd := exec.Command("git", args...)
		cmd.Stderr = nil
		if cmd.Run() == nil {
			return branch, nil
		}
	}

	return "", errors.New("could not determine default branch")
}

// extractProjectName extracts the project name from a git URL
func extractProjectName(repoURL string) (string, error) {
	// Handle SSH URLs (git@github.com:user/repo.git)
	if strings.Contains(repoURL, "@") && strings.Contains(repoURL, ":") {
		parts := strings.Split(repoURL, ":")
		if len(parts) == 2 {
			path := parts[1]
			return cleanProjectName(path), nil
		}
	}

	// Handle HTTPS URLs
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	path := parsed.Path
	return cleanProjectName(path), nil
}

// cleanProjectName cleans up a path to get the project name
func cleanProjectName(path string) string {
	// Remove .git suffix
	name := strings.TrimSuffix(path, ".git")
	// Get the last component
	name = filepath.Base(name)
	// Clean up
	name = strings.TrimPrefix(name, "/")
	return name
}
