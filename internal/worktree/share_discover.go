package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samzong/gmc/internal/gitcmd"
)

var copyCandidates = []string{
	".env", ".env.local", ".env.development", ".env.production",
	".claude/settings.json", ".claude/CLAUDE.md", ".serena/project.yml",
}

var managedDirectories = []string{"node_modules", ".venv", "venv", "vendor", "__pycache__"}

type DiscoverOptions struct {
	MainWorktreePath  string
	IncludeConfigured bool
}

type DiscoverResult struct {
	Path           string           `json:"path"`
	Strategy       ResourceStrategy `json:"strategy,omitempty"`
	Reason         string           `json:"reason"`
	Source         string           `json:"source,omitempty"`
	Status         string           `json:"status"`
	SizeBytes      int64            `json:"size_bytes"`
	SizeLimited    bool             `json:"size_limited,omitempty"`
	Project        string           `json:"project,omitempty"`
	Ecosystem      string           `json:"ecosystem,omitempty"`
	PackageManager string           `json:"package_manager,omitempty"`
	ConfiguredBy   string           `json:"configured_by,omitempty"`
	TargetState    string           `json:"target_state,omitempty"`
	ProjectMarker  bool             `json:"-"`
}

func (c *Client) Discover(opts DiscoverOptions) ([]DiscoverResult, error) {
	cfg, err := c.LoadEffectiveSharedConfig()
	if err != nil {
		return nil, err
	}
	roots := []string{opts.MainWorktreePath}
	if opts.MainWorktreePath == "" {
		roots, err = c.sharedSourceRoots()
		if err != nil {
			return nil, err
		}
	}
	primary := ""
	if opts.MainWorktreePath != "" {
		primary = opts.MainWorktreePath
	} else if primaryRoots := c.primarySharedRoots(roots); len(primaryRoots) > 0 {
		primary = primaryRoots[0]
		for _, root := range roots {
			if root != primary {
				primaryRoots = append(primaryRoots, root)
			}
		}
		roots = primaryRoots
	}
	current := c.currentTopLevel()
	seen := make(map[string]bool)
	var results []DiscoverResult
	for _, root := range roots {
		tracked, err := discoverTrackedPaths(root)
		if err != nil {
			return nil, err
		}
		entries, projects, err := scanShareProjects(root, cfg.Resources)
		if err != nil {
			return nil, err
		}
		paths := make([]string, 0, len(entries))
		for _, entry := range entries {
			paths = append(paths, entry.Path)
		}
		for _, entry := range entries {
			if seen[entry.Path] {
				continue
			}
			seen[entry.Path] = true
			entry.Source = filepath.Join(root, filepath.FromSlash(entry.Path))
			if entry.Status == "" {
				entry.Status = "candidate"
			}
			if project, ok := nearestShareProject(entry.Path, projects); ok {
				entry.Project, entry.Ecosystem, entry.PackageManager = project.Project, project.Ecosystem, project.PackageManager
			}
			if rule, ok := discoverCoveringRule(cfg.Resources, entry.Path); ok {
				entry.ConfiguredBy = rule.Path
				entry.Strategy = rule.Strategy
				entry.Status = "configured"
				entry.Reason = "covered by " + rule.Origin + " rule " + rule.Path
				if !rule.Disabled && sharedRuleUsesPrimary(rule) && root != primary {
					entry.Reason += "; source exists outside the primary worktree; not applied automatically"
				}
				if rule.Disabled {
					entry.Status, entry.Reason = "excluded", "excluded by rule "+rule.Path
				} else {
					parent := rule
					parent.Path = entry.Path
					if excluded := sharedExcludedChild(cfg.Resources, parent, paths); excluded != "" {
						entry.Reason += "; conflicts with excluded child " + excluded +
							"; disable the parent and share selected children"
					}
				}
			}
			for _, path := range tracked {
				if path == entry.Path || strings.HasPrefix(path, entry.Path+"/") {
					entry.Status, entry.Reason = "tracked", "contains Git-tracked content; keep it in each worktree"
					break
				}
			}
			if !opts.IncludeConfigured && entry.Status != "candidate" {
				continue
			}
			entry.SizeBytes, entry.SizeLimited = discoverLogicalSize(entry.Source)
			entry.TargetState = discoverTargetState(current, entry.Path, entry.Source)
			results = append(results, entry)
		}
		if opts.IncludeConfigured {
			for _, project := range projects {
				key := project.Path + ":" + project.Ecosystem
				if seen[key] {
					continue
				}
				seen[key] = true
				project.Source = filepath.Join(root, filepath.FromSlash(project.Path))
				project.ProjectMarker = true
				project.SizeBytes, project.SizeLimited = discoverLogicalSize(project.Source)
				project.TargetState = discoverTargetState(current, project.Path, project.Source)
				results = append(results, project)
			}
		}
	}
	if opts.IncludeConfigured {
		for _, rule := range cfg.Resources {
			if seen[rule.Path] || strings.ContainsAny(rule.Path, "*?[") {
				continue
			}
			status := "configured"
			if rule.Disabled {
				status = "excluded"
			}
			results = append(results, DiscoverResult{
				Path: rule.Path, Strategy: rule.Strategy, Status: status, ConfiguredBy: rule.Path,
				Reason: "no source found in scanned worktrees", TargetState: discoverTargetState(current, rule.Path, ""),
			})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Path == results[j].Path {
			return results[i].Status < results[j].Status
		}
		return results[i].Path < results[j].Path
	})
	return results, nil
}

func discoverCoveringRule(resources []SharedResource, path string) (SharedResource, bool) {
	return sharedRuleForPath(resources, path, true)
}

func discoverTrackedPaths(root string) ([]string, error) {
	if _, err := os.Lstat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		return nil, nil
	}
	result, err := (gitcmd.Runner{Dir: root}).Run("ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("inspect tracked files in %s: %w", root, err)
	}
	return strings.Split(string(result.Stdout), "\x00"), nil
}

func discoverTargetState(root, path, source string) string {
	if root == "" {
		return ""
	}
	target := filepath.Join(root, filepath.FromSlash(path))
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return "missing"
	}
	if err != nil {
		return "unreadable"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			return "broken-link"
		}
		if target == source {
			return "source"
		}
		resolvedSource, _ := filepath.EvalSymlinks(source)
		if resolved == resolvedSource {
			return "shared-link"
		}
		return "other-link"
	}
	if target == source {
		return "source"
	}
	return "independent"
}

func (c *Client) AddDiscoveredResources(results []DiscoverResult) (Report, error) {
	var report Report
	if len(results) == 0 {
		return report, nil
	}
	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}
	effective, err := c.LoadEffectiveSharedConfig()
	if err != nil {
		return report, err
	}
	for _, r := range results {
		if r.Status != "" && r.Status != "candidate" {
			continue
		}
		if r.Strategy != StrategyCopy && r.Strategy != StrategySymlink {
			continue
		}
		if _, covered := discoverCoveringRule(effective.Resources, r.Path); covered {
			continue
		}
		resource := SharedResource{Path: r.Path, Strategy: r.Strategy}
		cfg.Resources = append(cfg.Resources, resource)
		effective.Resources = append(effective.Resources, resource)
		report.Info(fmt.Sprintf("Added shared resource: %s (%s)", r.Path, r.Strategy))
	}
	if len(report.Events) == 0 {
		return report, nil
	}
	if err := c.SaveSharedConfig(cfg, configPath); err != nil {
		return report, err
	}
	return report, nil
}

func (c *Client) findMainWorktreePath() (string, error) {
	worktrees, err := c.ListCached()
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees: %w", err)
	}
	mainBranch, err := c.resolvedMainBranch()
	if err == nil && mainBranch != "" {
		for _, wt := range worktrees {
			if wt.Branch == mainBranch {
				return wt.Path, nil
			}
		}
	}
	for _, wt := range worktrees {
		if !wt.IsBare && filepath.Base(wt.Path) != ".bare" {
			return wt.Path, nil
		}
	}
	return "", errors.New("no worktree found to scan")
}
