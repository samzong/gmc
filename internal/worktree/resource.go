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
	Path     string           `yaml:"path"`
	Strategy ResourceStrategy `yaml:"strategy"`
}

type Hook struct {
	Cmd  string `yaml:"cmd"`
	Desc string `yaml:"desc,omitempty"`
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

	cfg, _, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	if len(cfg.Resources) == 0 && (!runHooks || len(cfg.Hooks) == 0) {
		return report, nil
	}

	if err := c.ensureInit(); err != nil {
		return report, err
	}

	for _, res := range cfg.Resources {
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

	if _, err := os.Stat(dstPath); err == nil {
		return report, nil
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
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal shared config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create shared config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write shared config: %w", err)
	}
	return nil
}

func (c *Client) AddSharedResource(path string, strategy ResourceStrategy) (Report, error) {
	var report Report

	normalizedPath, err := c.NormalizeSharedResourcePath(path)
	if err != nil {
		return report, err
	}

	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	found := false
	for i, res := range cfg.Resources {
		if res.Path == normalizedPath {
			cfg.Resources[i].Strategy = strategy
			found = true
			break
		}
	}

	if !found {
		cfg.Resources = append(cfg.Resources, SharedResource{
			Path:     normalizedPath,
			Strategy: strategy,
		})
	}

	if err := c.SaveSharedConfig(cfg, configPath); err != nil {
		return report, err
	}

	report.Info(fmt.Sprintf("Updated shared resource: %s (%s)", normalizedPath, strategy))
	return report, nil
}

func (c *Client) RemoveSharedResource(path string) (Report, error) {
	var report Report

	normalizedPath, err := c.NormalizeSharedResourcePath(path)
	if err != nil {
		return report, err
	}

	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	var newResources []SharedResource
	for _, res := range cfg.Resources {
		if res.Path != normalizedPath {
			newResources = append(newResources, res)
		}
	}

	if len(newResources) == len(cfg.Resources) {
		return report, fmt.Errorf("resource not found in config: %s", normalizedPath)
	}

	cfg.Resources = newResources
	if err := c.SaveSharedConfig(cfg, configPath); err != nil {
		return report, err
	}

	report.Info("Removed shared resource: " + normalizedPath)
	return report, nil
}

func (c *Client) AddHook(hook Hook) (Report, error) {
	var report Report

	if hook.Cmd == "" {
		return report, errors.New("hook command cannot be empty")
	}

	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	cfg.Hooks = append(cfg.Hooks, hook)

	if err := c.SaveSharedConfig(cfg, configPath); err != nil {
		return report, err
	}

	desc := hook.Desc
	if desc == "" {
		desc = hook.Cmd
	}
	report.Info(fmt.Sprintf("Added hook: %s", desc))
	return report, nil
}

func (c *Client) RemoveHook(index int) (Report, error) {
	var report Report

	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	if index < 0 || index >= len(cfg.Hooks) {
		return report, fmt.Errorf("hook index %d out of range (total: %d)", index, len(cfg.Hooks))
	}

	removed := cfg.Hooks[index]
	cfg.Hooks = append(cfg.Hooks[:index], cfg.Hooks[index+1:]...)

	if err := c.SaveSharedConfig(cfg, configPath); err != nil {
		return report, err
	}

	desc := removed.Desc
	if desc == "" {
		desc = removed.Cmd
	}
	report.Info(fmt.Sprintf("Removed hook: %s", desc))
	return report, nil
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
	for _, wt := range targets {
		resourceReport, err := c.syncSharedResourcesToPath(wt.Path, false)
		report.Merge(resourceReport)
		if err != nil {
			report.Warn(fmt.Sprintf("Warning: failed to sync %s: %v", filepath.Base(wt.Path), err))
		}
	}

	return report, nil
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

	parts := strings.SplitN(res.Path, string(filepath.Separator), 2)
	if len(parts) == 2 {
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

	currentRoot := c.currentTopLevel()
	if currentRoot != "" {
		candidate := filepath.Join(currentRoot, targetPath)
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
		if hook.Cmd == "" {
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
