package task

import (
	"errors"
	"fmt"
	"strings"
)

func NormalizeTaskAgent(agent string) string {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "", "codex", "codex-cli":
		return "codex"
	case "claude", "claude-code", "claude_code":
		return "claude"
	case "opencode":
		return "opencode"
	case "custom":
		return "custom"
	default:
		return strings.ToLower(strings.TrimSpace(agent))
	}
}

func AgentCommand(agent, model, mode, prompt string) ([]string, error) {
	agent = NormalizeTaskAgent(agent)
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
	case "claude":
		if prompt == "" {
			return []string{"claude"}, nil
		}
		return []string{"claude", prompt}, nil
	case "opencode":
		if prompt == "" {
			return []string{"opencode"}, nil
		}
		return []string{"opencode", "run", prompt}, nil
	case "custom":
		if strings.TrimSpace(mode) == "" || mode == "coding" {
			return nil, errors.New("custom agent requires --mode as the executable command")
		}
		return strings.Fields(mode), nil
	default:
		return nil, fmt.Errorf("unsupported agent %q (use codex, claude, opencode, or custom)", agent)
	}
}
