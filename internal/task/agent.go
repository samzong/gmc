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

func AgentCommand(agent, model, mode, prompt string) ([]string, error) {
	agent, err := NormalizeAgentAdapter(agent)
	if err != nil {
		return nil, err
	}
	prompt = strings.TrimSpace(prompt)
	switch agent {
	case "codex":
		args := []string{"codex"}
		if model != "" {
			args = append(args, "-m", model)
		}
		if mode != "" && mode != "coding" {
			args = append(args, mode)
		}
		if prompt != "" {
			args = append(args, prompt)
		}
		return args, nil
	case "grok":
		args := []string{"grok"}
		if model != "" {
			args = append(args, "-m", model)
		}
		if prompt != "" {
			args = append(args, prompt)
		}
		return args, nil
	case "cursor-agent":
		args := []string{"cursor-agent"}
		if model != "" {
			args = append(args, "--model", model)
		}
		if mode != "" && mode != "coding" {
			args = append(args, "--mode", mode)
		}
		if prompt != "" {
			args = append(args, prompt)
		}
		return args, nil
	case "opencode":
		if prompt == "" {
			return []string{"opencode"}, nil
		}
		return []string{"opencode", "run", prompt}, nil
	default:
		return nil, fmt.Errorf("unsupported agent %q (use codex, grok, cursor-agent, or opencode)", agent)
	}
}
