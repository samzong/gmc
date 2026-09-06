package worktree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ResourceStrategy string

const (
	StrategyCopy    ResourceStrategy = "copy"
	StrategySymlink ResourceStrategy = "link"
)

type SharedResource struct {
	worktreeRelative bool
	Path             string           `yaml:"path" json:"path"`
	Strategy         ResourceStrategy `yaml:"strategy,omitempty" json:"strategy,omitempty"`
	Disabled         bool             `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	Origin           string           `yaml:"-" json:"origin,omitempty"`
}

type Hook struct {
	ID       string `yaml:"id,omitempty" json:"id,omitempty"`
	Cmd      string `yaml:"cmd,omitempty" json:"cmd,omitempty"`
	Desc     string `yaml:"desc,omitempty" json:"desc,omitempty"`
	Disabled bool   `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	Origin   string `yaml:"-" json:"origin,omitempty"`
}

const (
	sharedConfigName       = "gmc-share.yml"
	legacySharedConfigYML  = ".gmc-shared.yml"
	legacySharedConfigYAML = ".gmc-shared.yaml"
)

type SharedConfig struct {
	Resources []SharedResource `yaml:"shared"`
	Hooks     []Hook           `yaml:"hooks,omitempty"`
}

func (c *Client) SyncSharedResources(worktreeName string) (Report, error) {
	var report Report

	targetRoot, err := c.resolveWorktreePath(worktreeName)
	if err != nil {
		return report, err
	}

	return c.syncSharedResourcesToPath(targetRoot, true)
}

func (c *Client) syncSharedResourcesToPath(targetRoot string, runHooks bool) (Report, error) {
	var report Report

	cfg, err := c.LoadEffectiveSharedConfig()
	if err != nil {
		return report, err
	}

	if len(cfg.Resources) == 0 && (!runHooks || len(cfg.Hooks) == 0) {
		return report, nil
	}

	if err := c.ensureInit(); err != nil {
		return report, err
	}

	resources, err := c.expandSharedResources(cfg.Resources)
	if err != nil {
		return report, err
	}
	for _, res := range resources {
		resourceReport, err := c.syncOneResource(c.worktreeRoot, targetRoot, res)
		report.Merge(resourceReport)
		if err != nil {
			return report, err
		}
	}

	if runHooks {
		if err := c.runHooks(targetRoot, cfg.Hooks, &report); err != nil {
			return report, err
		}
	}

	return report, nil
}

func (c *Client) syncOneResource(repoRoot, targetRoot string, res SharedResource) (Report, error) {
	var report Report

	if res.Path == "" {
		return report, errors.New("shared resource missing 'path' field")
	}
	if res.Strategy == "" {
		return report, fmt.Errorf("shared resource '%s' missing 'strategy' field", res.Path)
	}

	srcPath, targetPath, skip, err := c.resolveSharedPaths(repoRoot, targetRoot, res)
	if err != nil {
		return report, err
	}
	if skip {
		return report, nil
	}

	dstPath := filepath.Join(targetRoot, targetPath)

	info, err := os.Stat(srcPath)
	if os.IsNotExist(err) {
		if c.verbose {
			report.Warn("Shared resource source not found: " + srcPath)
		}
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("cannot inspect shared source %s: %w", srcPath, err)
	}

	if filepath.Clean(srcPath) == filepath.Clean(dstPath) {
		return report, nil
	}
	if dstInfo, statErr := os.Lstat(dstPath); statErr == nil {
		if res.Strategy == StrategyCopy && dstInfo.Mode().Type() == info.Mode().Type() {
			return report, nil
		}
		if dstInfo.Mode()&os.ModeSymlink != 0 {
			destination, dstErr := filepath.EvalSymlinks(dstPath)
			source, srcErr := filepath.EvalSymlinks(srcPath)
			if dstErr == nil && srcErr == nil && destination == source && res.Strategy == StrategySymlink {
				return report, nil
			}
		}
		report.Warn(fmt.Sprintf("Kept existing resource: %s (not replaced; requested %s from %s)",
			dstPath, res.Strategy, srcPath))
		return report, nil
	} else if !os.IsNotExist(statErr) {
		return report, fmt.Errorf("cannot inspect shared destination %s: %w", dstPath, statErr)
	}
	if err := ensureUntrackedResource(targetRoot, targetPath); err != nil {
		return report, err
	}
	if err := ensureUntrackedResource(filepath.Dir(srcPath), filepath.Base(srcPath)); err != nil {
		return report, err
	}
	if err := ensureSharedDestination(targetRoot, targetPath); err != nil {
		return report, err
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return report, fmt.Errorf("failed to create parent directory for %s: %w", dstPath, err)
	}

	report.Info(fmt.Sprintf("Syncing shared resource: %s -> %s (%s)", res.Path, targetPath, res.Strategy))

	switch res.Strategy {
	case StrategySymlink:
		relSrc, err := filepath.Rel(filepath.Dir(dstPath), srcPath)
		if err != nil {
			return report, fmt.Errorf("failed to calculate relative path: %w", err)
		}
		if err := os.Symlink(relSrc, dstPath); err != nil {
			return report, fmt.Errorf("failed to symlink %s: %w", res.Path, err)
		}
	case StrategyCopy:
		if info.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return report, fmt.Errorf("failed to copy directory %s: %w", res.Path, err)
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return report, fmt.Errorf("failed to copy file %s: %w", res.Path, err)
			}
		}
	default:
		return report, fmt.Errorf("unknown strategy '%s' for resource '%s' (valid: copy, link)", res.Strategy, res.Path)
	}
	return report, nil
}

func (c *Client) LoadSharedConfig() (*SharedConfig, string, error) {
	c.once.Do(c.init)

	commonDir, err := c.GetGitCommonDir()
	if err != nil {
		if c.bareRoot != "" {
			commonDir = filepath.Join(c.bareRoot, ".bare")
		} else {
			return nil, "", err
		}
	}

	configPath := filepath.Join(commonDir, sharedConfigName)
	legacyCandidates := []string{
		filepath.Join(commonDir, legacySharedConfigYML),
		filepath.Join(commonDir, legacySharedConfigYAML),
	}
	if c.worktreeRoot != "" {
		legacyCandidates = append(legacyCandidates,
			filepath.Join(c.worktreeRoot, legacySharedConfigYML),
			filepath.Join(c.worktreeRoot, legacySharedConfigYAML),
		)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		for _, candidate := range legacyCandidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				configPath = candidate
				break
			}
		}
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &SharedConfig{Resources: []SharedResource{}}, filepath.Join(commonDir, sharedConfigName), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, configPath, fmt.Errorf("failed to read shared config: %w", err)
	}

	var cfg SharedConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, configPath, fmt.Errorf("failed to parse shared config: %w", err)
	}

	return &cfg, configPath, nil
}

func (c *Client) SaveSharedConfig(cfg *SharedConfig, path string) error {
	if err := validateSharedConfig(cfg); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal shared config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create shared config directory: %w", err)
	}

	if err := writeSharedConfig(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write shared config: %w", err)
	}
	return nil
}

func (c *Client) AddSharedResource(path string, strategy ResourceStrategy) (Report, error) {
	return c.addSharedResource(path, strategy, false)
}

func (c *Client) RemoveSharedResource(path string) (Report, error) {
	return c.removeSharedResource(path, false)
}

func (c *Client) AddHook(hook Hook) (Report, error) {
	return c.addHook(hook, false)
}

func (c *Client) RemoveHook(index int) (Report, error) {
	return c.removeHook(index, false)
}

func (c *Client) SyncAllSharedResources() (Report, error) {
	var report Report

	worktrees, err := c.ListCached()
	if err != nil {
		return report, err
	}

	if err := c.ensureInit(); err != nil {
		return report, err
	}
	isBare := c.repoDir != c.worktreeRoot
	var targets []Info
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" {
			continue
		}
		if isBare && isExternalPath(c.worktreeRoot, wt.Path) {
			continue
		}
		targets = append(targets, wt)
	}

	if len(targets) == 0 {
		report.Info("No worktrees to sync.")
		return report, nil
	}

	report.Info(fmt.Sprintf("Syncing resources to %d worktrees...", len(targets)))
	var failures []error
	for _, wt := range targets {
		resourceReport, err := c.syncSharedResourcesToPath(wt.Path, false)
		report.Merge(resourceReport)
		if err != nil {
			report.Warn(fmt.Sprintf("Warning: failed to sync %s: %v", filepath.Base(wt.Path), err))
			failures = append(failures, fmt.Errorf("sync %s: %w", filepath.Base(wt.Path), err))
		}
	}

	return report, errors.Join(failures...)
}

func (c *Client) resolveWorktreePath(worktreeName string) (string, error) {
	if worktreeName == "" {
		return "", errors.New("worktree name cannot be empty")
	}

	c.once.Do(c.init)
	repoRoot := c.worktreeRoot
	worktrees, err := c.ListCached()
	if err != nil {
		if repoRoot != "" {
			candidate := filepath.Join(repoRoot, worktreeName)
			if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
				return candidate, nil
			}
		}
		return "", err
	}

	var exactMatches []string
	var relMatches []string
	var baseMatches []string
	for _, wt := range worktrees {
		if wt.Path == worktreeName {
			exactMatches = append(exactMatches, wt.Path)
			continue
		}
		if repoRoot != "" {
			if rel, relErr := filepath.Rel(repoRoot, wt.Path); relErr == nil && rel == worktreeName {
				relMatches = append(relMatches, wt.Path)
				continue
			}
		}
		if filepath.Base(wt.Path) == worktreeName {
			baseMatches = append(baseMatches, wt.Path)
		}
	}

	if match, err := uniqueWorktreeMatch(worktreeName, exactMatches, "exact path"); match != "" || err != nil {
		return match, err
	}
	if match, err := uniqueWorktreeMatch(worktreeName, relMatches, "repo-relative path"); match != "" || err != nil {
		return match, err
	}
	if match, err := uniqueWorktreeMatch(worktreeName, baseMatches, "basename"); match != "" || err != nil {
		return match, err
	}

	return "", fmt.Errorf("worktree not found: %s", worktreeName)
}

func (c *Client) currentTopLevel() string {
	result, err := c.runner.Run("rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	root := result.StdoutString(true)
	if root == "" {
		return ""
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root)
	}
	absRoot, absErr := filepath.Abs(root)
	if absErr != nil {
		return ""
	}
	return absRoot
}

func (c *Client) NormalizeSharedResourcePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("shared resource path cannot be empty")
	}

	if filepath.IsAbs(trimmed) {
		currentRoot := c.currentTopLevel()
		if currentRoot == "" {
			return "", fmt.Errorf("absolute shared resource path must be inside the current worktree: %s", path)
		}
		rel, err := filepath.Rel(currentRoot, trimmed)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("absolute shared resource path must stay within the current worktree: %s", path)
		}
		trimmed = rel
	}

	trimmed = filepath.Clean(trimmed)
	if trimmed == "." {
		return "", errors.New("shared resource path cannot be '.'")
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, ".."+string(filepath.Separator)) || trimmed == ".." {
		return "", fmt.Errorf("shared resource path must stay within the worktree: %s", path)
	}
	return trimmed, nil
}

func (c *Client) resolveSharedPaths(
	repoRoot, targetRoot string, res SharedResource,
) (srcPath string, targetPath string, skip bool, err error) {
	targetPath, err = sanitizeTargetRelativePath(res.Path)
	if err != nil {
		return "", "", false, err
	}

	roots, rootsErr := c.sharedSourceRoots()
	if rootsErr != nil {
		return "", "", false, rootsErr
	}
	if res.worktreeRelative {
		roots = c.primarySharedRoots(roots)
		if len(roots) == 0 {
			return "", targetPath, true, nil
		}
		repoRoot = roots[0]
	}
	for _, root := range roots {
		if _, statErr := os.Lstat(filepath.Join(root, ".git")); statErr != nil {
			continue
		}
		candidate := filepath.Join(root, targetPath)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, targetPath, false, nil
		}
	}

	parts := strings.SplitN(res.Path, string(filepath.Separator), 2)
	if len(parts) == 2 && !res.worktreeRelative {
		worktrees, listErr := c.ListCached()
		if listErr == nil {
			var baseMatches []string
			for _, wt := range worktrees {
				if filepath.Base(wt.Path) == parts[0] {
					baseMatches = append(baseMatches, wt.Path)
				}
			}
			if match, matchErr := uniqueWorktreeMatch(parts[0], baseMatches, "legacy basename"); matchErr != nil {
				return "", "", false, matchErr
			} else if match != "" {
				if match == targetRoot {
					return "", "", true, nil
				}
				srcPath = filepath.Join(match, parts[1])
				targetPath, err = sanitizeTargetRelativePath(parts[1])
				if err != nil {
					return "", "", false, err
				}
				return srcPath, targetPath, false, nil
			}
		}
	}

	srcPath = filepath.Join(repoRoot, targetPath)
	if _, statErr := os.Stat(srcPath); statErr == nil {
		return srcPath, targetPath, false, nil
	}

	for _, root := range roots {
		candidate := filepath.Join(root, targetPath)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, targetPath, false, nil
		}
	}

	return srcPath, targetPath, false, nil
}

func uniqueWorktreeMatch(input string, matches []string, matchType string) (string, error) {
	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", fmt.Errorf("ambiguous worktree %q by %s: %s", input, matchType, strings.Join(matches, ", "))
}

func sanitizeTargetRelativePath(path string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" || cleaned == "." {
		return "", errors.New("shared resource path cannot be empty")
	}
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("shared resource path must stay within the worktree: %s", path)
	}
	return cleaned, nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	sourceInfo, err := os.Stat(src)
	if err == nil {
		_ = os.Chmod(dst, sourceInfo.Mode())
	}

	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

func (c *Client) runHooks(worktreeRoot string, hooks []Hook, report *Report) error {
	if len(hooks) == 0 {
		return nil
	}

	for _, hook := range hooks {
		if hook.Disabled || hook.Cmd == "" {
			continue
		}

		label := hook.Cmd
		if hook.Desc != "" {
			label = hook.Desc
		}
		report.Info(fmt.Sprintf("Running hook: %s", label))

		cmd := exec.Command("sh", "-c", hook.Cmd)
		cmd.Dir = worktreeRoot
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook failed '%s': %w", hook.Cmd, err)
		}
	}

	return nil
}
