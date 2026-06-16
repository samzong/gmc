package task

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultWorkflowName = "default"

func LoadWorkflowConfig() (WorkflowConfig, string, error) {
	for _, path := range workflowConfigPaths() {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return WorkflowConfig{}, "", err
		}
		cfg, err := ParseWorkflowConfig(data)
		if err != nil {
			return WorkflowConfig{}, "", fmt.Errorf("%s: %w", path, err)
		}
		return cfg, path, nil
	}
	return DefaultWorkflowConfig(), "", nil
}

func workflowConfigPaths() []string {
	home, _ := os.UserHomeDir()
	var paths []string
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "gmc", "workflow.yaml"))
	} else if home != "" {
		paths = append(paths, filepath.Join(home, ".config", "gmc", "workflow.yaml"))
	}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".gmc", "workflow.yaml"))
	}
	return paths
}

func ParseWorkflowConfig(data []byte) (WorkflowConfig, error) {
	var cfg WorkflowConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return WorkflowConfig{}, err
	}
	if cfg.Default == "" {
		cfg.Default = DefaultWorkflowName
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Version != 1 {
		return WorkflowConfig{}, fmt.Errorf("unsupported workflow version %d", cfg.Version)
	}
	if len(cfg.Workflows) == 0 && len(cfg.Nodes) > 0 {
		cfg.Workflows = map[string]WorkflowDefinition{
			DefaultWorkflowName: {
				Start: cfg.Start,
				Nodes: cfg.Nodes,
			},
		}
	}
	if len(cfg.Workflows) == 0 {
		return WorkflowConfig{}, errors.New("workflow config has no workflows")
	}
	for name, wf := range cfg.Workflows {
		normalized, err := NormalizeWorkflowDefinition(name, wf)
		if err != nil {
			return WorkflowConfig{}, err
		}
		cfg.Workflows[name] = normalized
	}
	if _, ok := cfg.Workflows[cfg.Default]; !ok {
		return WorkflowConfig{}, fmt.Errorf("default workflow %q not found", cfg.Default)
	}
	return cfg, nil
}

func DefaultWorkflowConfig() WorkflowConfig {
	wf := WorkflowDefinition{
		Name:  DefaultWorkflowName,
		Start: "plan",
		Nodes: map[string]WorkflowNode{
			"plan": {
				Prompt: strings.Join([]string{
					"Read .gmc/TASK.md.",
					"Analyze the task and produce a concise implementation plan.",
					"Do not edit project files yet.",
				}, " "),
				Next: "code",
			},
			"code": {
				Prompt: strings.Join([]string{
					"Implement the approved plan with the smallest correct change.",
					"Run focused checks.",
					"Stop after summarizing changed files and verification.",
				}, " "),
				Next: "review",
			},
			"review": {
				Prompt: strings.Join([]string{
					"Review the current diff for correctness, regressions, and missing verification.",
					"Run make check when practical.",
					"Stop with findings first.",
				}, " "),
				Next: "ship",
			},
			"ship": {
				Prompt: strings.Join([]string{
					"Prepare a ship summary: changed files, verification, remaining risk, and suggested commit message.",
					"Do not commit.",
				}, " "),
				Next: "done",
			},
		},
	}
	cfg := WorkflowConfig{
		Version:   1,
		Default:   DefaultWorkflowName,
		Workflows: map[string]WorkflowDefinition{DefaultWorkflowName: wf},
	}
	cfg.Workflows[DefaultWorkflowName], _ = NormalizeWorkflowDefinition(DefaultWorkflowName, wf)
	return cfg
}

func NormalizeWorkflowDefinition(name string, wf WorkflowDefinition) (WorkflowDefinition, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return WorkflowDefinition{}, errors.New("workflow name is required")
	}
	if len(wf.Nodes) == 0 {
		return WorkflowDefinition{}, fmt.Errorf("workflow %q has no nodes", name)
	}
	wf.Name = name
	for id, node := range wf.Nodes {
		id = strings.TrimSpace(id)
		if id == "" {
			return WorkflowDefinition{}, fmt.Errorf("workflow %q has an empty node id", name)
		}
		node.ID = id
		if strings.TrimSpace(node.Agent) != "" {
			node.Agent = NormalizeTaskAgent(node.Agent)
			if _, err := NormalizeAgentAdapter(node.Agent); err != nil {
				return WorkflowDefinition{}, fmt.Errorf("workflow %q node %q: %w", name, id, err)
			}
		}
		if node.Skill != "" && len(node.Skills) == 0 {
			node.Skills = []string{node.Skill}
		}
		wf.Nodes[id] = node
	}
	if strings.TrimSpace(wf.Start) == "" {
		start, err := inferWorkflowStart(wf)
		if err != nil {
			return WorkflowDefinition{}, fmt.Errorf("workflow %q: %w", name, err)
		}
		wf.Start = start
	}
	if _, ok := wf.Nodes[wf.Start]; !ok {
		return WorkflowDefinition{}, fmt.Errorf("workflow %q start node %q not found", name, wf.Start)
	}
	for id, node := range wf.Nodes {
		next := strings.TrimSpace(node.Next)
		if next == "" || next == "done" {
			continue
		}
		if _, ok := wf.Nodes[next]; !ok {
			return WorkflowDefinition{}, fmt.Errorf("workflow %q node %q next node %q not found", name, id, next)
		}
	}
	return wf, nil
}

func inferWorkflowStart(wf WorkflowDefinition) (string, error) {
	incoming := map[string]bool{}
	for _, node := range wf.Nodes {
		next := strings.TrimSpace(node.Next)
		if next != "" && next != "done" {
			incoming[next] = true
		}
	}
	var starts []string
	for id := range wf.Nodes {
		if !incoming[id] {
			starts = append(starts, id)
		}
	}
	sort.Strings(starts)
	if len(starts) != 1 {
		return "", errors.New("start node is ambiguous; set start explicitly")
	}
	return starts[0], nil
}

func SelectWorkflow(cfg WorkflowConfig, name string) (WorkflowDefinition, error) {
	if strings.TrimSpace(name) == "" {
		name = cfg.Default
	}
	wf, ok := cfg.Workflows[name]
	if !ok {
		return WorkflowDefinition{}, fmt.Errorf("workflow %q not found", name)
	}
	return NormalizeWorkflowDefinition(name, wf)
}

func WorkflowNodeAgent(node WorkflowNode, fallback string) (string, error) {
	agent := node.Agent
	if strings.TrimSpace(agent) == "" {
		agent = fallback
	}
	return NormalizeAgentAdapter(agent)
}

func WorkflowNodeModel(node WorkflowNode, fallback string) string {
	if strings.TrimSpace(node.Model) != "" {
		return node.Model
	}
	return fallback
}
