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

// SyncSharedResources syncs shared resources defined in .gmc-shared.yml
func (c *Client) SyncSharedResources(worktreeName string) error {
	cfg, _, err := c.LoadSharedConfig()
	if err != nil {
		return err
	}

	// No config or empty, skip
	if len(cfg.Resources) == 0 {
		return nil
	}

	root, err := c.GetWorktreeRoot()
	if err != nil {
		return err
	}

	targetRoot := filepath.Join(root, worktreeName)

	for _, res := range cfg.Resources {
		if err := c.syncOneResource(root, targetRoot, res); err != nil {
			return err
		}
	}
	return nil
}

// syncOneResource handles the logic for a single resource sync
func (c *Client) syncOneResource(root, targetRoot string, res SharedResource) error {
	// Validate config fields
	if res.Path == "" {
		return errors.New("shared resource missing 'path' field")
	}
	if res.Strategy == "" {
		return fmt.Errorf("shared resource '%s' missing 'strategy' field", res.Path)
	}

	// Parse path to determine source and destination
	// Path can be:
	//   - "main/models" -> source in main worktree, target is "models"
	//   - ".env" -> source in project root, target is ".env"
	srcPath := filepath.Join(root, res.Path)

	// Determine target path (strip worktree prefix if present)
	targetPath := res.Path
	parts := strings.SplitN(res.Path, string(filepath.Separator), 2)
	if len(parts) == 2 {
		// Check if first part is a worktree directory
		potentialWorktree := filepath.Join(root, parts[0])
		if info, err := os.Stat(potentialWorktree); err == nil && info.IsDir() {
			// Check if it's actually a worktree (not .bare)
			if parts[0] != ".bare" {
				// First part is a worktree name, use second part as target
				targetPath = parts[1]

				// Skip if target worktree is the source worktree
				targetWorktreeName := filepath.Base(targetRoot)
				if targetWorktreeName == parts[0] {
					if c.verbose {
						fmt.Printf("Skipping %s: source worktree is target\n", res.Path)
					}
					return nil
				}
			}
		}
	}

	dstPath := filepath.Join(targetRoot, targetPath)

	// Check if source exists
	info, err := os.Stat(srcPath)
	if os.IsNotExist(err) {
		if c.verbose {
			fmt.Printf("Shared resource source not found: %s\n", srcPath)
		}
		return nil
	}

	// Skip if destination already exists to avoid overwriting user changes
	if _, err := os.Stat(dstPath); err == nil {
		return nil
	}

	// Ensure destination parent directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory for %s: %w", dstPath, err)
	}

	fmt.Printf("Syncing shared resource: %s -> %s (%s)\n", res.Path, targetPath, res.Strategy)

	switch res.Strategy {
	case StrategySymlink:
		// Use relative symlinks so worktrees can be moved if needed
		relSrc, err := filepath.Rel(filepath.Dir(dstPath), srcPath)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}
		if err := os.Symlink(relSrc, dstPath); err != nil {
			return fmt.Errorf("failed to symlink %s: %w", res.Path, err)
		}
	case StrategyCopy:
		if info.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy directory %s: %w", res.Path, err)
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy file %s: %w", res.Path, err)
			}
		}
	default:
		return fmt.Errorf("unknown strategy '%s' for resource '%s' (valid: copy, link)", res.Strategy, res.Path)
	}
	return nil
}

// LoadSharedConfig loads the shared configuration from .gmc-shared.yml
func (c *Client) LoadSharedConfig() (*SharedConfig, string, error) {
	root, err := c.GetWorktreeRoot()
	if err != nil {
		return nil, "", err
	}

	// Prefer .yml, verify .yaml
	configPath := filepath.Join(root, ".gmc-shared.yml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		yamlPath := filepath.Join(root, ".gmc-shared.yaml")
		if _, err := os.Stat(yamlPath); err == nil {
			configPath = yamlPath
		}
	}

	// If file doesn't exist, return empty config but valid path (for saving)
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

// SaveSharedConfig saves the configuration to file
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

// AddSharedResource adds or updates a resource in the configuration
func (c *Client) AddSharedResource(path string, strategy ResourceStrategy) error {
	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return err
	}

	// Check if already exists, update if so
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
		return err
	}

	fmt.Printf("Updated shared resource: %s (%s)\n", path, strategy)
	return nil
}

// RemoveSharedResource removes a resource from the configuration
func (c *Client) RemoveSharedResource(path string) error {
	cfg, configPath, err := c.LoadSharedConfig()
	if err != nil {
		return err
	}

	var newResources []SharedResource
	for _, res := range cfg.Resources {
		if res.Path != path {
			newResources = append(newResources, res)
		}
	}

	if len(newResources) == len(cfg.Resources) {
		return fmt.Errorf("resource not found in config: %s", path)
	}

	cfg.Resources = newResources
	if err := c.SaveSharedConfig(cfg, configPath); err != nil {
		return err
	}

	fmt.Printf("Removed shared resource: %s\n", path)
	return nil
}

// SyncAllSharedResources syncs shared resources to ALL existing worktrees
func (c *Client) SyncAllSharedResources() error {
	worktrees, err := c.List()
	if err != nil {
		return err
	}

	// Filter out bare worktrees
	var targets []WorktreeInfo
	for _, wt := range worktrees {
		if !wt.IsBare && filepath.Base(wt.Path) != ".bare" {
			targets = append(targets, wt)
		}
	}

	if len(targets) == 0 {
		fmt.Println("No worktrees to sync.")
		return nil
	}

	fmt.Printf("Syncing resources to %d worktrees...\n", len(targets))
	for _, wt := range targets {
		wtName := filepath.Base(wt.Path)
		if err := c.SyncSharedResources(wtName); err != nil {
			fmt.Printf("Warning: failed to sync %s: %v\n", wtName, err)
		}
	}

	return nil
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
