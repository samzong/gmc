package worktree

import (
	"os"
	"path/filepath"
)

func (c *Client) CurrentWorktree(worktrees []Info) (Info, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return Info{}, false
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	var current Info
	deepest := ""
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" {
			continue
		}
		path := filepath.Clean(wt.Path)
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		if pathWithin(path, cwd) && len(path) > len(deepest) {
			current, deepest = wt, path
		}
	}
	return current, deepest != ""
}
