package worktree

import (
	"errors"
	"fmt"
	"io"
	"os"
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

const (
	sharedConfigName       = "gmc-share.yml"
	legacySharedConfigYML  = ".gmc-shared.yml"
	legacySharedConfigYAML = ".gmc-shared.yaml"
)

type SharedConfig struct {
	Resources []SharedResource `yaml:"shared"`
}

func (c *Client) SyncSharedResources(worktreeName string) (Report, error) {
	var report Report

	targetRoot, err := c.resolveWorktreePath(worktreeName)
	if err != nil {
		return report, err
	}

	return c.syncSharedResourcesToPath(targetRoot)
}

func (c *Client) syncSharedResourcesToPath(targetRoot string) (Report, error) {
	var report Report

	cfg, _, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	if len(cfg.Resources) == 0 {
		return report, nil
	}

	repoRoot, err := c.GetRepoRoot()
	if err != nil {
		return report, err
	}

	for _, res := range cfg.Resources {
		resourceReport, err := c.syncOneResource(repoRoot, targetRoot, res)
		report.Merge(resourceReport)
		if err != nil {
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
	commonDir, err := c.GetGitCommonDir()
	if err != nil {
		if root, bareErr := FindBareRoot(""); bareErr == nil {
			commonDir = root
		} else {
			return nil, "", err
		}
	}

	configPath := filepath.Join(commonDir, sharedConfigName)
	legacyCandidates := []string{
		filepath.Join(commonDir, legacySharedConfigYML),
		filepath.Join(commonDir, legacySharedConfigYAML),
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
		return &SharedConfig{Resources: []SharedResource{}}, configPath, nil
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

	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	var newResources []SharedResource
	for _, res := range cfg.Resources {
		if res.Path != path {
			newResources = append(newResources, res)
		}
	}

	if len(newResources) == len(cfg.Resources) {
		return report, fmt.Errorf("resource not found in config: %s", path)
	}

	cfg.Resources = newResources
	if err := c.SaveSharedConfig(cfg, configPath); err != nil {
		return report, err
	}

	report.Info("Removed shared resource: " + path)
	return report, nil
}

func (c *Client) SyncAllSharedResources() (Report, error) {
	var report Report

	worktrees, err := c.List()
	if err != nil {
		return report, err
	}

	var targets []Info
	for _, wt := range worktrees {
		if !wt.IsBare && filepath.Base(wt.Path) != ".bare" {
			targets = append(targets, wt)
		}
	}

	if len(targets) == 0 {
		report.Info("No worktrees to sync.")
		return report, nil
	}

	report.Info(fmt.Sprintf("Syncing resources to %d worktrees...", len(targets)))
	for _, wt := range targets {
		resourceReport, err := c.syncSharedResourcesToPath(wt.Path)
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

	repoRoot, _ := c.GetRepoRoot()
	worktrees, err := c.List()
	if err != nil {
		if repoRoot != "" {
			return filepath.Join(repoRoot, worktreeName), nil
		}
		return "", err
	}
	for _, wt := range worktrees {
		if wt.Path == worktreeName {
			return wt.Path, nil
		}
		if filepath.Base(wt.Path) == worktreeName {
			return wt.Path, nil
		}
		if repoRoot != "" {
			if rel, relErr := filepath.Rel(repoRoot, wt.Path); relErr == nil && rel == worktreeName {
				return wt.Path, nil
			}
		}
	}

	if repoRoot != "" {
		return filepath.Join(repoRoot, worktreeName), nil
	}
	return worktreeName, nil
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
		if currentRoot != "" {
			rel, err := filepath.Rel(currentRoot, trimmed)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				trimmed = rel
			}
		}
	}

	trimmed = filepath.Clean(trimmed)
	if trimmed == "." {
		return "", errors.New("shared resource path cannot be '.'")
	}
	if strings.HasPrefix(trimmed, ".."+string(filepath.Separator)) || trimmed == ".." {
		return "", fmt.Errorf("shared resource path must stay within the worktree: %s", path)
	}
	return trimmed, nil
}

func (c *Client) resolveSharedPaths(repoRoot, targetRoot string, res SharedResource) (srcPath string, targetPath string, skip bool, err error) {
	targetPath = res.Path

	parts := strings.SplitN(res.Path, string(filepath.Separator), 2)
	if len(parts) == 2 {
		worktrees, listErr := c.List()
		if listErr == nil {
			for _, wt := range worktrees {
				if filepath.Base(wt.Path) != parts[0] {
					continue
				}
				if wt.Path == targetRoot {
					if c.verbose {
						var report Report
						report.Warn(fmt.Sprintf("Skipping %s: source worktree is target", res.Path))
					}
					return "", "", true, nil
				}
				srcPath = filepath.Join(wt.Path, parts[1])
				targetPath = parts[1]
				return srcPath, targetPath, false, nil
			}
		}
	}

	srcPath = filepath.Join(repoRoot, res.Path)
	if _, statErr := os.Stat(srcPath); statErr == nil {
		return srcPath, targetPath, false, nil
	}

	currentRoot := c.currentTopLevel()
	if currentRoot != "" {
		candidate := filepath.Join(currentRoot, res.Path)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, targetPath, false, nil
		}
	}

	return srcPath, targetPath, false, nil
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
