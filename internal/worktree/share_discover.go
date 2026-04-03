package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

var copyCandidates = []string{
	".env",
	".env.local",
	".env.development",
	".env.production",
	".claude/settings.json",
	".claude/CLAUDE.md",
	".serena/project.yml",
}

var linkCandidates = []string{
	"node_modules",
	".venv",
	"vendor",
	"__pycache__",
}

type DiscoverOptions struct {
	MainWorktreePath string
}

type DiscoverResult struct {
	Path     string           `json:"path"`
	Strategy ResourceStrategy `json:"strategy"`
	Reason   string           `json:"reason"`
}

func (c *Client) Discover(opts DiscoverOptions) ([]DiscoverResult, error) {
	cfg, _, err := c.LoadSharedConfig()
	if err != nil {
		return nil, err
	}

	existing := make(map[string]bool, len(cfg.Resources))
	for _, res := range cfg.Resources {
		existing[res.Path] = true
	}

	mainPath := opts.MainWorktreePath
	if mainPath == "" {
		mainPath, err = c.findMainWorktreePath()
		if err != nil {
			return nil, err
		}
	}

	var results []DiscoverResult

	for _, candidate := range copyCandidates {
		if existing[candidate] {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(mainPath, candidate)); statErr == nil {
			results = append(results, DiscoverResult{
				Path:     candidate,
				Strategy: StrategyCopy,
				Reason:   "config/env file — isolated copy per worktree",
			})
		}
	}

	for _, candidate := range linkCandidates {
		if existing[candidate] {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(mainPath, candidate)); statErr == nil {
			results = append(results, DiscoverResult{
				Path:     candidate,
				Strategy: StrategySymlink,
				Reason:   "dependency/cache directory — shared symlink saves disk",
			})
		}
	}

	return results, nil
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

	existing := make(map[string]bool, len(cfg.Resources))
	for _, res := range cfg.Resources {
		existing[res.Path] = true
	}

	for _, r := range results {
		if existing[r.Path] {
			continue
		}
		cfg.Resources = append(cfg.Resources, SharedResource{
			Path:     r.Path,
			Strategy: r.Strategy,
		})
		report.Info(fmt.Sprintf("Added shared resource: %s (%s)", r.Path, r.Strategy))
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
			fmt.Fprintf(os.Stderr, "Warning: no main/master branch found, using worktree %q\n", filepath.Base(wt.Path))
			return wt.Path, nil
		}
	}

	return "", fmt.Errorf("no worktree found to scan")
}
