package worktree

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/samzong/gmc/internal/gitcmd"
)

type Options struct {
	Verbose          bool
	GlobalConfigPath string
}

type Client struct {
	runner           gitcmd.Runner
	verbose          bool
	globalConfigPath string

	once         sync.Once
	bareRoot     string
	worktreeRoot string
	searchRoot   string
	repoDir      string
	initErr      error

	listMu    sync.Mutex
	listCache []Info
	listValid bool
}

func NewClient(opts Options) *Client {
	return &Client{
		runner:           gitcmd.Runner{Verbose: opts.Verbose},
		verbose:          opts.Verbose,
		globalConfigPath: opts.GlobalConfigPath,
	}
}

func (c *Client) init() {
	bareRoot, err := FindBareRoot("")
	if err == nil {
		c.bareRoot = bareRoot
		c.worktreeRoot = bareRoot
	} else {
		commonDir, cdErr := c.GetGitCommonDir()
		if cdErr != nil {
			c.initErr = cdErr
			return
		}
		c.worktreeRoot = filepath.Dir(commonDir)
	}

	c.repoDir = repoDirForGit(c.worktreeRoot)

	if c.repoDir != c.worktreeRoot {
		c.searchRoot = c.worktreeRoot
	} else {
		c.searchRoot = filepath.Dir(c.worktreeRoot)
	}
}

func (c *Client) ensureInit() error {
	c.once.Do(c.init)
	return c.initErr
}

func (c *Client) ListCached() ([]Info, error) {
	c.listMu.Lock()
	defer c.listMu.Unlock()

	if !c.listValid {
		list, err := c.List()
		if err != nil {
			return nil, err
		}
		c.listCache, c.listValid = list, true
	}
	return append([]Info{}, c.listCache...), nil
}

func (c *Client) InvalidateList() {
	c.listMu.Lock()
	c.listCache = nil
	c.listValid = false
	c.listMu.Unlock()
}

type Info struct {
	Path       string
	Branch     string
	Commit     string
	IsPrunable bool
	IsLocked   bool
	IsBare     bool
}

func (c *Client) List() ([]Info, error) {
	c.once.Do(c.init)

	args := []string{"worktree", "list", "--porcelain"}
	if c.bareRoot != "" {
		args = append([]string{"-C", filepath.Join(c.bareRoot, ".bare")}, args...)
	}
	result, err := c.runner.RunLogged(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}
	return parseWorktreeList(string(result.Stdout))
}

func parseWorktreeList(output string) ([]Info, error) {
	var worktrees []Info
	var current *Info
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			current = nil
		case strings.HasPrefix(line, "worktree "):
			worktrees = append(worktrees, Info{Path: strings.TrimPrefix(line, "worktree ")})
			current = &worktrees[len(worktrees)-1]
		case current == nil:
			continue
		case strings.HasPrefix(line, "HEAD "):
			current.Commit = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
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
	return worktrees, nil
}

func (c *Client) GetWorktreeStatus(path string) string {
	result, err := c.runner.Run("-C", path, "status", "--porcelain")
	if err != nil {
		return "unknown"
	}

	output := result.StdoutString(true)
	if output == "" {
		return "clean"
	}

	lines := strings.Split(output, "\n")
	var modified, untracked int

	for _, line := range lines {
		if len(line) < 2 {
			continue
		}

		if line[:2] == "??" {
			untracked++
		} else {
			modified++
		}
	}

	var parts []string
	if modified > 0 {
		if modified == 1 {
			parts = append(parts, "1 file changed")
		} else {
			parts = append(parts, fmt.Sprintf("%d files changed", modified))
		}
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}

	if len(parts) == 0 {
		return "modified"
	}
	return strings.Join(parts, ", ")
}

func (c *Client) listGitRefs(errLabel string, gitArgs ...string) ([]string, error) {
	c.once.Do(c.init)

	var args []string
	if c.repoDir != "" {
		args = append([]string{"-C", c.repoDir}, gitArgs...)
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

func (c *Client) ListBranches() ([]string, error) {
	return c.listGitRefs("list branches", "branch", "--format=%(refname:short)")
}

func (c *Client) ListRemotes() ([]string, error) {
	return c.listGitRefs("list remotes", "remote")
}
