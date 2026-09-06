package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func (c *Client) LoadGlobalSharedConfig() (*SharedConfig, string, error) {
	var document struct {
		Worktree SharedConfig `yaml:"worktree"`
	}
	if c.globalConfigPath == "" {
		return &document.Worktree, "", nil
	}
	data, err := os.ReadFile(c.globalConfigPath)
	if os.IsNotExist(err) {
		return &document.Worktree, c.globalConfigPath, nil
	}
	if err != nil {
		return nil, c.globalConfigPath, fmt.Errorf("read global worktree config: %w", err)
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, c.globalConfigPath, fmt.Errorf("parse global worktree config: %w", err)
	}
	return &document.Worktree, c.globalConfigPath, nil
}

func (c *Client) LoadEffectiveSharedConfig() (*SharedConfig, error) {
	global, _, err := c.LoadGlobalSharedConfig()
	if err != nil {
		return nil, err
	}
	local, _, err := c.LoadSharedConfig()
	if err != nil {
		return nil, err
	}
	var merged SharedConfig
	resourceIndex := make(map[string]int)
	hookIndex := make(map[string]int)
	for i, cfg := range []*SharedConfig{global, local} {
		origin := "global"
		if i == 1 {
			origin = "repository"
		}
		if err := validateSharedConfig(cfg); err != nil {
			return nil, fmt.Errorf("%s worktree config: %w", origin, err)
		}
		for _, res := range cfg.Resources {
			res.Path = filepath.ToSlash(filepath.Clean(res.Path))
			res.Origin = origin
			if index, ok := resourceIndex[res.Path]; ok {
				merged.Resources[index] = res
			} else {
				resourceIndex[res.Path] = len(merged.Resources)
				merged.Resources = append(merged.Resources, res)
			}
		}
		for _, hook := range cfg.Hooks {
			hook.Origin = origin
			key := hookKey(hook)
			if index, ok := hookIndex[key]; ok {
				merged.Hooks[index] = hook
			} else {
				hookIndex[key] = len(merged.Hooks)
				merged.Hooks = append(merged.Hooks, hook)
			}
		}
	}
	return &merged, nil
}

func validateSharedConfig(cfg *SharedConfig) error {
	paths := make(map[string]bool)
	for _, res := range cfg.Resources {
		if err := validateSharedPattern(res.Path); err != nil {
			return err
		}
		key := filepath.ToSlash(filepath.Clean(res.Path))
		if paths[key] {
			return fmt.Errorf("duplicate shared resource: %s", res.Path)
		}
		paths[key] = true
		if !res.Disabled && res.Strategy != StrategyCopy && res.Strategy != StrategySymlink {
			return fmt.Errorf("invalid strategy for %s: %q (use copy or link)", res.Path, res.Strategy)
		}
	}
	ids := make(map[string]bool)
	for _, hook := range cfg.Hooks {
		if hook.ID != "" {
			_, numeric := strconv.Atoi(hook.ID)
			if numeric == nil || strings.TrimSpace(hook.ID) != hook.ID || strings.ContainsAny(hook.ID, " \t\r\n") {
				return fmt.Errorf("hook id must be a nonnumeric name without whitespace: %q", hook.ID)
			}
			if ids[hook.ID] {
				return fmt.Errorf("duplicate hook id: %s", hook.ID)
			}
			ids[hook.ID] = true
		}
		if strings.TrimSpace(hook.Cmd) == "" && (!hook.Disabled || hook.ID == "") {
			return errors.New("hook requires a command or a disabled named id")
		}
	}
	return nil
}

func hookKey(hook Hook) string {
	if hook.ID != "" {
		return "id:" + hook.ID
	}
	return "cmd:" + hook.Cmd
}

func (c *Client) SaveGlobalSharedConfig(cfg *SharedConfig) error {
	if c.globalConfigPath == "" {
		return errors.New("global config path is unavailable")
	}
	if err := validateSharedConfig(cfg); err != nil {
		return err
	}
	var document yaml.Node
	data, err := os.ReadFile(c.globalConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(data) > 0 {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&document); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("parse global config: %w", err)
		}
		var trailing yaml.Node
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return errors.New("global config must contain a single YAML document")
		}
	}
	if len(document.Content) == 0 {
		document = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return errors.New("global config must be a YAML mapping")
	}
	var value yaml.Node
	if err := value.Encode(cfg); err != nil {
		return err
	}
	found := false
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "worktree" {
			value.Anchor = root.Content[i+1].Anchor
			root.Content[i+1] = &value
			found = true
			break
		}
	}
	if !found {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "worktree"}, &value)
	}
	data, err = yaml.Marshal(&document)
	if err != nil {
		return err
	}
	var checked yaml.Node
	if err := yaml.Unmarshal(data, &checked); err != nil {
		return fmt.Errorf("cannot preserve global config references: %w", err)
	}
	return writeSharedConfig(c.globalConfigPath, data, 0o600)
}

func writeSharedConfig(path string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		path = resolved
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".gmc-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

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

func (c *Client) AddGlobalSharedResource(path string, strategy ResourceStrategy) (Report, error) {
	return c.addSharedResource(path, strategy, true)
}

func (c *Client) addSharedResource(path string, strategy ResourceStrategy, global bool) (Report, error) {
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
	found := false
	for i, existing := range cfg.Resources {
		if filepath.ToSlash(filepath.Clean(existing.Path)) == resource.Path {
			cfg.Resources[i] = resource
			found = true
			break
		}
	}
	if !found {
		cfg.Resources = append(cfg.Resources, resource)
	}
	if err := c.saveSharedScope(cfg, configPath, global); err != nil {
		return report, err
	}
	report.Info(fmt.Sprintf("Updated shared resource: %s (%s)", path, strategy))
	return report, nil
}

func (c *Client) RemoveGlobalSharedResource(path string) (Report, error) {
	return c.removeSharedResource(path, true)
}

func (c *Client) removeSharedResource(path string, global bool) (Report, error) {
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

func (c *Client) AddGlobalHook(hook Hook) (Report, error) {
	return c.addHook(hook, true)
}

func (c *Client) addHook(hook Hook, global bool) (Report, error) {
	var report Report
	cfg, configPath, err := c.loadSharedScope(global)
	if err != nil {
		return report, err
	}
	found := false
	for i, existing := range cfg.Hooks {
		if hookKey(existing) == hookKey(hook) {
			cfg.Hooks[i] = hook
			found = true
			break
		}
	}
	if !found {
		cfg.Hooks = append(cfg.Hooks, hook)
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

func (c *Client) RemoveGlobalHook(index int) (Report, error) {
	return c.removeHook(index, true)
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
			return c.removeHook(i, global)
		}
	}
	return report, fmt.Errorf("hook id not found: %s", id)
}

func (c *Client) removeHook(index int, global bool) (Report, error) {
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
	remaining := make([]Hook, 0, len(cfg.Hooks))
	for _, hook := range cfg.Hooks {
		if hookKey(hook) != hookKey(removed) {
			remaining = append(remaining, hook)
		}
	}
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
