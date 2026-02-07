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

type SharedConfig struct {
	Resources []SharedResource `yaml:"shared"`
}

func (c *Client) SyncSharedResources(worktreeName string) (Report, error) {
	var report Report

	cfg, _, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	if len(cfg.Resources) == 0 {
		return report, nil
	}

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return report, err
	}

	targetRoot := filepath.Join(root, worktreeName)

	for _, res := range cfg.Resources {
		resourceReport, err := c.syncOneResource(root, targetRoot, res)
		report.Merge(resourceReport)
		if err != nil {
			return report, err
		}
	}
	return report, nil
}

func (c *Client) syncOneResource(root, targetRoot string, res SharedResource) (Report, error) {
	var report Report

	if res.Path == "" {
		return report, errors.New("shared resource missing 'path' field")
	}
	if res.Strategy == "" {
		return report, fmt.Errorf("shared resource '%s' missing 'strategy' field", res.Path)
	}

	srcPath := filepath.Join(root, res.Path)
	targetPath := res.Path
	parts := strings.SplitN(res.Path, string(filepath.Separator), 2)
	if len(parts) == 2 {
		potentialWorktree := filepath.Join(root, parts[0])
		if info, err := os.Stat(potentialWorktree); err == nil && info.IsDir() {
			if parts[0] != ".bare" {
				targetPath = parts[1]
				targetWorktreeName := filepath.Base(targetRoot)
				if targetWorktreeName == parts[0] {
					if c.verbose {
						report.Warn(fmt.Sprintf("Skipping %s: source worktree is target", res.Path))
					}
					return report, nil
				}
			}
		}
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
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return nil, "", err
	}

	configPath := filepath.Join(root, ".gmc-shared.yml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		yamlPath := filepath.Join(root, ".gmc-shared.yaml")
		if _, err := os.Stat(yamlPath); err == nil {
			configPath = yamlPath
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

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write shared config: %w", err)
	}
	return nil
}

func (c *Client) AddSharedResource(path string, strategy ResourceStrategy) (Report, error) {
	var report Report

	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return report, err
	}

	found := false
	for i, res := range cfg.Resources {
		if res.Path == path {
			cfg.Resources[i].Strategy = strategy
			found = true
			break
		}
	}

	if !found {
		cfg.Resources = append(cfg.Resources, SharedResource{
			Path:     path,
			Strategy: strategy,
		})
	}

	if err := c.SaveSharedConfig(cfg, configPath); err != nil {
		return report, err
	}

	report.Info(fmt.Sprintf("Updated shared resource: %s (%s)", path, strategy))
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
		wtName := filepath.Base(wt.Path)
		resourceReport, err := c.SyncSharedResources(wtName)
		report.Merge(resourceReport)
		if err != nil {
			report.Warn(fmt.Sprintf("Warning: failed to sync %s: %v", wtName, err))
		}
	}

	return report, nil
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
