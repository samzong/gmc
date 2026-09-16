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
