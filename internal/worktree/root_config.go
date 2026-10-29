package worktree

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const globalRootLockReason = "gmc: preserve worktree on an unavailable global root"

func (c *Client) globalWorktreeDirectory() (string, string, error) {
	c.rootOnce.Do(func() {
		if err := c.ensureInit(); err != nil {
			c.globalRootErr = err
			return
		}
		c.globalRoot, c.globalRelative, c.globalRootErr = c.loadGlobalWorktreeDirectory()
	})
	return c.globalRoot, c.globalRelative, c.globalRootErr
}

func (c *Client) loadGlobalWorktreeDirectory() (string, string, error) {
	cfg, _, err := c.LoadGlobalSharedConfig()
	if err != nil {
		return "", "", err
	}
	if cfg.Root == "" {
		return "", "", nil
	}
	root := cfg.Root
	if root == "~" || strings.HasPrefix(root, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		if root == "~" {
			root = home
		} else {
			root = filepath.Join(home, strings.TrimPrefix(root, "~/"))
		}
	}
	if !filepath.IsAbs(root) {
		return "", "", fmt.Errorf("worktree.root must be an absolute path: %s", root)
	}
	owner := remoteOwner(c.getGitOutput(c.repoDir, "remote", "get-url", "origin"))
	if owner == "" {
		owner = filepath.Base(filepath.Dir(c.worktreeRoot))
	}
	if !safeDirectoryComponent(owner) {
		return "", "", fmt.Errorf("invalid worktree owner directory: %q", owner)
	}
	return filepath.Clean(root), filepath.Join(owner, filepath.Base(c.worktreeRoot)), nil
}

func remoteOwner(remote string) string {
	var path string
	if strings.Contains(remote, "://") {
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Hostname() == "" || parsed.Scheme == "file" {
			return ""
		}
		path = parsed.Path
	} else {
		host, rest, ok := strings.Cut(remote, ":")
		if !ok || host == "" || strings.ContainsAny(host, "/\\") {
			return ""
		}
		path = rest
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || !safeDirectoryComponent(parts[0]) ||
		!safeDirectoryComponent(strings.TrimSuffix(parts[1], ".git")) {
		return ""
	}
	return parts[0]
}

func safeDirectoryComponent(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\\x00\r\n")
}

func (c *Client) newWorktreePath(dirName, fallback string) (string, bool, string, error) {
	root, relative, err := c.globalWorktreeDirectory()
	if err != nil || root == "" {
		return fallback, false, "", err
	}
	directory, err := os.OpenRoot(root)
	if os.IsNotExist(err) {
		warning := fmt.Sprintf("Warning: worktree root %s is unavailable; using the repository's default location", root)
		return fallback, false, warning, nil
	}
	if err != nil {
		return "", false, "", fmt.Errorf("open worktree root: %w", err)
	}
	defer directory.Close()
	if err := directory.MkdirAll(relative, 0o755); err != nil {
		return "", false, "", fmt.Errorf("create worktree parent directories: %w", err)
	}
	return filepath.Join(root, relative, filepath.Base(c.worktreeRoot)+"--"+dirName), true, "", nil
}

func (c *Client) isManagedWorktree(wt Info) bool {
	return pathWithin(c.searchRoot, wt.Path) || c.IsGlobalWorktree(wt)
}

func (c *Client) IsGlobalWorktree(wt Info) bool {
	if wt.IsLocked && wt.LockReason == globalRootLockReason {
		return true
	}
	if err := c.ensureInit(); err != nil {
		return false
	}
	root, relative, err := c.globalWorktreeDirectory()
	if err != nil || root == "" {
		return false
	}
	parent := filepath.Join(root, relative)
	return samePath(filepath.Dir(wt.Path), parent) &&
		strings.HasPrefix(filepath.Base(wt.Path), filepath.Base(c.worktreeRoot)+"--")
}

func unavailableGlobalWorktreeMessage(path string) string {
	root := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	return fmt.Sprintf("worktree directory is unavailable: %s; keeping its Git metadata; reconnect %s and retry",
		path, root)
}
