package worktree

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
)

func (c *Client) loadSharedScope(global bool) (*SharedConfig, string, error) {
	if global {
		return c.LoadGlobalSharedConfig()
	}
	return c.LoadSharedConfig()
}

func (c *Client) saveSharedScope(cfg *SharedConfig, path string, global bool) error {
	if err := validateSharedConfig(cfg); err != nil {
		return err
	}
	if global {
		return c.SaveGlobalSharedConfig(cfg)
	}
	return c.SaveSharedConfig(cfg, path)
}

func (c *Client) AddSharedResource(path string, strategy ResourceStrategy, global bool) (Report, error) {
	var report Report
	if global && filepath.IsAbs(path) {
		return report, errors.New("global shared paths must be worktree-relative")
	}
	path, err := c.NormalizeSharedResourcePath(path)
	if err != nil {
		return report, err
	}
	cfg, configPath, err := c.loadSharedScope(global)
	if err != nil {
		return report, err
	}
	resource := SharedResource{Path: filepath.ToSlash(path), Strategy: strategy}
	index := slices.IndexFunc(cfg.Resources, func(existing SharedResource) bool {
		return filepath.ToSlash(filepath.Clean(existing.Path)) == resource.Path
	})
	if index < 0 {
		cfg.Resources = append(cfg.Resources, resource)
	} else {
		cfg.Resources[index] = resource
	}
	if err := c.saveSharedScope(cfg, configPath, global); err != nil {
		return report, err
	}
	report.Info(fmt.Sprintf("Updated shared resource: %s (%s)", path, strategy))
	return report, nil
}

func (c *Client) RemoveSharedResource(path string, global bool) (Report, error) {
	var report Report
	path, err := c.NormalizeSharedResourcePath(path)
	if err != nil {
		return report, err
	}
	path = filepath.ToSlash(path)
	cfg, configPath, err := c.loadSharedScope(global)
	if err != nil {
		return report, err
	}
	found := false
	remaining := make([]SharedResource, 0, len(cfg.Resources))
	for _, resource := range cfg.Resources {
		if filepath.ToSlash(filepath.Clean(resource.Path)) == path {
			found = true
		} else {
			remaining = append(remaining, resource)
		}
	}
	if !global {
		defaults, _, err := c.LoadGlobalSharedConfig()
		if err != nil {
			return report, err
		}
		for _, resource := range defaults.Resources {
			if resource.Path == path || MatchSharedPath(resource.Path, path) {
				remaining = append(remaining, SharedResource{Path: path, Disabled: true})
				found = true
				break
			}
		}
	}
	if !found {
		return report, fmt.Errorf("resource not found in config: %s", path)
	}
	cfg.Resources = remaining
	if err := c.saveSharedScope(cfg, configPath, global); err != nil {
		return report, err
	}
	report.Info("Removed or disabled shared resource: " + path)
	return report, nil
}

func (c *Client) AddHook(hook Hook, global bool) (Report, error) {
	var report Report
	cfg, configPath, err := c.loadSharedScope(global)
	if err != nil {
		return report, err
	}
	index := slices.IndexFunc(cfg.Hooks, func(existing Hook) bool { return hookKey(existing) == hookKey(hook) })
	if index < 0 {
		cfg.Hooks = append(cfg.Hooks, hook)
	} else {
		cfg.Hooks[index] = hook
	}
	if err := c.saveSharedScope(cfg, configPath, global); err != nil {
		return report, err
	}
	label := hook.Desc
	if label == "" {
		label = hook.Cmd
	}
	report.Info("Updated hook: " + label)
	return report, nil
}

func (c *Client) hookList(global bool) (*SharedConfig, error) {
	if global {
		cfg, _, err := c.LoadGlobalSharedConfig()
		return cfg, err
	}
	return c.LoadEffectiveSharedConfig()
}

func (c *Client) RemoveHookByID(id string, global bool) (Report, error) {
	var report Report
	cfg, err := c.hookList(global)
	if err != nil {
		return report, err
	}
	for i, hook := range cfg.Hooks {
		if hook.ID == id {
			return c.RemoveHook(i, global)
		}
	}
	return report, fmt.Errorf("hook id not found: %s", id)
}

func (c *Client) RemoveHook(index int, global bool) (Report, error) {
	var report Report
	visible, err := c.hookList(global)
	if err != nil {
		return report, err
	}
	if index < 0 || index >= len(visible.Hooks) {
		return report, fmt.Errorf("hook index out of range: %d", index+1)
	}
	removed := visible.Hooks[index]
	cfg, configPath, err := c.loadSharedScope(global)
	if err != nil {
		return report, err
	}
	remaining := slices.DeleteFunc(cfg.Hooks, func(hook Hook) bool { return hookKey(hook) == hookKey(removed) })
	if !global {
		defaults, _, err := c.LoadGlobalSharedConfig()
		if err != nil {
			return report, err
		}
		for _, hook := range defaults.Hooks {
			if hookKey(hook) == hookKey(removed) {
				hook.Disabled = true
				remaining = append(remaining, hook)
				break
			}
		}
	}
	cfg.Hooks = remaining
	if err := c.saveSharedScope(cfg, configPath, global); err != nil {
		return report, err
	}
	report.Info(fmt.Sprintf("Removed or disabled hook: %d", index+1))
	return report, nil
}
