package task

import (
	"fmt"
	"strings"
)

func NormalizeTaskAgent(agent string) string {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "", "codex", "codex-cli":
		return "codex"
	case "grok":
		return "grok"
	case "cursor", "cursor-agent", "cursor_agent":
		return "cursor-agent"
	case "opencode":
		return "opencode"
	default:
		return strings.ToLower(strings.TrimSpace(agent))
	}
}

func NormalizeAgentAdapter(agent string) (string, error) {
	normalized := NormalizeTaskAgent(agent)
	switch normalized {
	case "codex", "grok", "cursor-agent", "opencode":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported task agent %q (use codex, grok, cursor-agent, or opencode)", agent)
	}
}

func AgentCommand(agent, model, prompt string) ([]string, error) {
	agent, err := NormalizeAgentAdapter(agent)
	if err != nil {
		return nil, err
	}
	prompt = strings.TrimSpace(prompt)
	args := []string{agent}
	if agent == "opencode" {
		if prompt != "" {
			args = append(args, "run", prompt)
		}
		return args, nil
	}
	if model != "" {
		flag := "-m"
		if agent == "cursor-agent" {
			flag = "--model"
		}
		args = append(args, flag, model)
	}
	if prompt != "" {
		args = append(args, prompt)
	}
	return args, nil
}

func WorkflowNodeCommand(node WorkflowNode, agent, model, prompt string) ([]string, error) {
	if strings.TrimSpace(node.Command) != "" {
		args := strings.Fields(node.Command)
		if len(args) == 0 {
			return nil, fmt.Errorf("empty command for workflow node %q", node.ID)
		}
		if strings.TrimSpace(prompt) != "" {
			args = append(args, strings.TrimSpace(prompt))
		}
		return args, nil
	}
	return AgentCommand(agent, model, prompt)
}
