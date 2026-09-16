package git

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/samzong/gmc/internal/gitcmd"
	"github.com/samzong/gmc/internal/gitutil"
	"github.com/samzong/gmc/internal/stringsutil"
)

type Options struct {
	Verbose bool
}

type Client struct {
	runner gitcmd.Runner
}

func NewClient(opts Options) *Client {
	return &Client{runner: gitcmd.Runner{Verbose: opts.Verbose}}
}

func (c *Client) logVerboseOutput(label string, data []byte) {
	if c != nil && c.runner.Verbose && len(data) > 0 {
		fmt.Fprintln(os.Stderr, label, string(data))
	}
}

func (c *Client) IsGitRepository() bool {
	_, err := c.runner.Run("rev-parse", "--is-inside-work-tree")
	return err == nil
}

var ErrNotGitRepo = errors.New("not a git repository")

func (c *Client) CheckGitRepository() error {
	if !c.IsGitRepository() {
		return fmt.Errorf("%w: please run this command inside a git working tree", ErrNotGitRepo)
	}
	return nil
}

func (c *Client) checkedOutput(action string, args ...string) (string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return "", err
	}
	result, err := c.runner.RunLogged(args...)
	if err != nil {
		c.logVerboseOutput("Git stderr:", result.Stderr)
		return "", fmt.Errorf("%s: %w", action, err)
	}
	return string(result.Stdout), nil
}

func (c *Client) GetStagedDiff() (string, error) {
	return c.checkedOutput("failed to run git diff --cached", "diff", "--cached", "-U1")
}

func (c *Client) GetStagedDiffStats() (string, error) {
	return c.checkedOutput("failed to run git diff --cached --numstat --summary",
		"diff", "--cached", "--numstat", "--summary")
}

func (c *Client) ParseStagedFiles() ([]string, error) {
	output, err := c.checkedOutput("failed to run git diff --cached --name-only", "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	return stringsutil.SplitNonEmpty(strings.TrimSpace(output), "\n"), nil
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
	return c.commit("Failed to run git commit", message, args)
}

func (c *Client) CommitFiles(message string, files []string, args ...string) error {
	return c.commit("Failed to commit files", message, slices.Concat(args, []string{"--"}, files))
}

func (c *Client) commit(action, message string, args []string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}
	result, err := c.runner.RunLogged(append([]string{"commit", "-m", message}, args...)...)
	c.logVerboseOutput("Git output:", result.Stdout)
	c.logVerboseOutput("Git stderr:", result.Stderr)
	if err != nil {
		return gitutil.WrapGitError(action, result, err)
	}
	return nil
}

func (c *Client) CreateAndSwitchBranch(branchName string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}
	if err := gitutil.ValidateBranchName(branchName); err != nil {
		return err
	}
	if c.runner.Verbose {
		fmt.Fprintf(os.Stderr, "Checking if branch exists: git rev-parse --verify %s\n", branchName)
	}
	if _, err := c.runner.Run("rev-parse", "--verify", branchName); err == nil {
		return fmt.Errorf("branch '%s' already exists", branchName)
	}
	if c.runner.Verbose {
		fmt.Fprintf(os.Stderr, "Creating and switching to branch: git checkout -b %s\n", branchName)
	}
	result, err := c.runner.Run("checkout", "-b", branchName)
	if err != nil {
		return gitutil.WrapGitError(fmt.Sprintf("failed to create and switch to branch '%s'", branchName), result, err)
	}
	c.logVerboseOutput("Git output:", result.Stdout)
	return nil
}
