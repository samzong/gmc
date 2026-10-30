package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var errWorktreeNotFound = errors.New("worktree not found")

type NameUse int

const (
	DisplayName NameUse = iota
	RemoveName
	PromoteName
	SharedPrefix
)

func (c *Client) WorktreeName(wt Info, worktrees []Info) string {
	_ = c.ensureInit()
	name := filepath.Base(wt.Path)
	if !c.IsGlobalWorktree(wt) && c.searchRoot != "" {
		rel, err := filepath.Rel(c.searchRoot, wt.Path)
		switch {
		case err != nil || !isLocalRel(rel) || IsAgentWorktree(wt.Path):
			name = wt.Path
			if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(name, home+string(filepath.Separator)) {
				name = "~" + name[len(home):]
			}
		case rel != "." && rel != "":
			name = rel
		}
	}
	for _, use := range []NameUse{DisplayName, RemoveName, PromoteName} {
		resolved, err := c.ResolveWorktree(name, worktrees, use)
		if err != nil || resolved.Path != wt.Path {
			return wt.Path
		}
	}
	return name
}

func (c *Client) ResolveWorktree(name string, worktrees []Info, use NameUse) (Info, error) {
	if name == "" {
		return Info{}, errors.New("worktree name cannot be empty")
	}
	if err := c.ensureInit(); err != nil {
		return Info{}, err
	}
	input := name
	if strings.HasPrefix(name, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Info{}, err
		}
		name = filepath.Join(home, strings.TrimPrefix(name, "~/"))
	}
	const (
		absolute = iota
		searchRelative
		repoRelative
		cwdRelative
		globalAlias
		basename
	)
	order := []int{absolute, searchRelative, globalAlias, repoRelative, cwdRelative, basename}
	switch use {
	case RemoveName:
		order = []int{absolute, searchRelative, globalAlias}
	case PromoteName:
		order = []int{absolute, cwdRelative, repoRelative, basename, searchRelative, globalAlias}
	case SharedPrefix:
		order = []int{absolute, basename}
	}
	cwd, _ := os.Getwd()
	for _, kind := range order {
		var matches []Info
		for _, wt := range worktrees {
			matched := false
			switch kind {
			case absolute:
				matched = filepath.IsAbs(name) && samePath(wt.Path, name)
			case searchRelative, repoRelative, cwdRelative:
				root := c.searchRoot
				if kind == repoRelative {
					root = c.worktreeRoot
				}
				if kind == cwdRelative {
					root = cwd
				}
				matched = !filepath.IsAbs(name) && root != "" && samePath(wt.Path, filepath.Join(root, name))
			case globalAlias:
				base := filepath.Base(wt.Path)
				matched = !filepath.IsAbs(name) && c.IsGlobalWorktree(wt) &&
					(base == name || strings.TrimPrefix(base, filepath.Base(c.worktreeRoot)+"--") == name)
			case basename:
				matched = filepath.Base(wt.Path) == name
			}
			if matched {
				matches = append(matches, wt)
			}
		}
		if len(matches) > 1 {
			paths := make([]string, len(matches))
			for i, wt := range matches {
				paths[i] = wt.Path
			}
			hint := ""
			if use != SharedPrefix {
				hint = "; use an absolute path"
			}
			return Info{}, fmt.Errorf("ambiguous worktree %q: %s%s", input, strings.Join(paths, ", "), hint)
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
	}
	return Info{}, fmt.Errorf("%w: %s\nUse 'gmc wt ls' to see available worktrees", errWorktreeNotFound, input)
}

func IsAgentWorktree(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/.claude/worktrees/") || strings.Contains(normalized, "/.codex/worktrees/")
}
