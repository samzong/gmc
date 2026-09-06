package worktree

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samzong/gmc/internal/gitcmd"
)

func validateSharedPattern(pattern string) error {
	if _, err := sanitizeTargetRelativePath(pattern); err != nil {
		return err
	}
	for _, part := range strings.Split(filepath.ToSlash(pattern), "/") {
		if part == ".git" || part == ".bare" || part == ".." {
			return fmt.Errorf("shared path cannot address Git metadata or a parent directory: %s", pattern)
		}
		if part != "**" {
			if _, err := path.Match(part, ""); err != nil {
				return fmt.Errorf("invalid shared path pattern %q: %w", pattern, err)
			}
		}
	}
	return nil
}

func MatchSharedPath(pattern, candidate string) bool {
	pattern = filepath.ToSlash(filepath.Clean(pattern))
	candidate = filepath.ToSlash(filepath.Clean(candidate))
	return matchSharedSegments(strings.Split(pattern, "/"), strings.Split(candidate, "/"))
}

func matchSharedSegments(pattern, candidate []string) bool {
	if len(pattern) == 0 {
		return len(candidate) == 0
	}
	if pattern[0] == "**" {
		if matchSharedSegments(pattern[1:], candidate) {
			return true
		}
		return len(candidate) > 0 && matchSharedSegments(pattern, candidate[1:])
	}
	if len(candidate) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], candidate[0])
	return err == nil && matched && matchSharedSegments(pattern[1:], candidate[1:])
}

func sharedRuleForPath(rules []SharedResource, candidate string, includeParents bool) (SharedResource, bool) {
	var chosen SharedResource
	found := false
	for _, rule := range rules {
		matches := MatchSharedPath(rule.Path, candidate)
		if includeParents {
			matches = matches || MatchSharedPath(strings.TrimSuffix(rule.Path, "/")+"/**", candidate)
		}
		if !matches {
			continue
		}
		if !found || sharedRulePreferred(rule, chosen, candidate) {
			chosen, found = rule, true
		}
	}
	return chosen, found
}

func sharedRulePreferred(rule, previous SharedResource, candidate string) bool {
	if (rule.Origin == "repository") != (previous.Origin == "repository") {
		return rule.Origin == "repository"
	}
	if (rule.Path == candidate) != (previous.Path == candidate) {
		return rule.Path == candidate
	}
	return len(rule.Path) >= len(previous.Path)
}

func sharedExcludedChild(rules []SharedResource, parent SharedResource, paths []string) string {
	var candidates []string
	for _, rule := range rules {
		if rule.Disabled && !strings.ContainsAny(rule.Path, "*?[") {
			candidates = append(candidates, rule.Path)
		}
	}
	candidates = append(candidates, paths...)
	for _, candidate := range candidates {
		if !strings.HasPrefix(candidate, parent.Path+"/") {
			continue
		}
		if rule, found := sharedRuleForPath(rules, candidate, true); found && rule.Disabled {
			return rule.Path
		}
	}
	return ""
}

func (c *Client) sharedSourceRoots() ([]string, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}
	var roots []string
	seen := make(map[string]bool)
	add := func(root string) {
		if root == "" {
			return
		}
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() || seen[root] {
			return
		}
		seen[root] = true
		roots = append(roots, root)
	}
	add(c.worktreeRoot)
	if mainRoot, err := c.findMainWorktreePath(); err == nil {
		add(mainRoot)
	}
	worktrees, err := c.ListCached()
	if err != nil && len(roots) == 0 {
		return nil, err
	}
	for _, wt := range worktrees {
		if !wt.IsBare && filepath.Base(wt.Path) != ".bare" {
			add(wt.Path)
		}
	}
	return roots, nil
}

func (c *Client) primarySharedRoots(roots []string) []string {
	if len(roots) > 0 {
		if _, err := os.Lstat(filepath.Join(roots[0], ".git")); err == nil {
			return roots[:1]
		}
	}
	policy, err := c.NewProtectionPolicy()
	if err != nil {
		return nil
	}
	worktrees, err := c.ListCached()
	if err != nil {
		return nil
	}
	for _, wt := range worktrees {
		if !wt.IsBare && policy.IsProtected(wt) {
			if root, err := filepath.EvalSymlinks(wt.Path); err == nil {
				return []string{root}
			}
		}
	}
	return nil
}

func sharedRuleUsesPrimary(rule SharedResource) bool {
	return rule.Origin == "global" || strings.ContainsAny(rule.Path, "*?[")
}

func (c *Client) expandSharedResources(rules []SharedResource) ([]SharedResource, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	roots, err := c.sharedSourceRoots()
	if err != nil {
		return nil, err
	}
	paths := make(map[string]bool)
	patterns := false
	for _, rule := range rules {
		if strings.ContainsAny(rule.Path, "*?[") {
			patterns = true
		} else {
			paths[rule.Path] = true
		}
	}
	if patterns {
		for _, root := range c.primarySharedRoots(roots) {
			entries, _, err := scanShareProjects(root, rules)
			if err != nil {
				return nil, err
			}
			for _, entry := range entries {
				paths[entry.Path] = true
			}
		}
	}
	ordered := make([]string, 0, len(paths))
	for resourcePath := range paths {
		ordered = append(ordered, resourcePath)
	}
	sort.Strings(ordered)
	var resources []SharedResource
	for _, resourcePath := range ordered {
		rule, found := sharedRuleForPath(rules, resourcePath, true)
		if !found || rule.Disabled || !MatchSharedPath(rule.Path, resourcePath) {
			continue
		}
		covered := false
		for _, parent := range resources {
			if strings.HasPrefix(resourcePath, parent.Path+"/") {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		rule.worktreeRelative = sharedRuleUsesPrimary(rule)
		rule.Path = resourcePath
		if excluded := sharedExcludedChild(rules, rule, ordered); excluded != "" {
			return nil, fmt.Errorf("cannot exclude %s inside shared directory %s; "+
				"disable the parent and share selected children", excluded, resourcePath)
		}
		resources = append(resources, rule)
	}
	return resources, nil
}

func ensureUntrackedResource(root, resourcePath string) error {
	runner := gitcmd.Runner{Dir: root}
	if _, err := runner.Run("rev-parse", "--show-toplevel"); err != nil {
		if _, statErr := os.Lstat(filepath.Join(root, ".git")); os.IsNotExist(statErr) {
			return nil
		}
		return fmt.Errorf("cannot inspect resource repository %s: %w", root, err)
	}
	result, err := runner.Run("--literal-pathspecs", "ls-files", "-z", "--", resourcePath)
	if err != nil {
		return fmt.Errorf("cannot inspect tracked resource %s: %w", resourcePath, err)
	}
	if len(result.Stdout) > 0 {
		return fmt.Errorf("refusing to share Git-tracked content: %s", filepath.Join(root, resourcePath))
	}
	return nil
}

func ensureSharedDestination(root, relative string) error {
	parent := filepath.Dir(relative)
	for parent != "." {
		info, err := os.Lstat(filepath.Join(root, parent))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("shared destination has a symlink parent: %s", filepath.Join(root, parent))
		}
		parent = filepath.Dir(parent)
	}
	return nil
}
