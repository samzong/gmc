package task

import (
	"errors"
	"fmt"
	"strings"
)

// NormalizeTaskAgent maps CLI/config names to a canonical agent id.
func NormalizeTaskAgent(agent string) string {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "", "codex", "codex-cli":
		return "codex"
	case "claude", "claude-code", "claude_code":
		return "claude"
	case "opencode":
		return "opencode"
	case "grok":
		return "grok"
	case "custom":
		return "custom"
	default:
		return strings.ToLower(strings.TrimSpace(agent))
	}
}

// AgentCommand builds the argv for an interactive agent session.
// prompt is passed to the agent when supported (codex/claude positional prompt).
func AgentCommand(agent, model, mode, prompt string) ([]string, error) {
	agent = NormalizeTaskAgent(agent)
	if agent == "" {
		agent = "codex"
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
	case "claude":
		args := []string{"claude"}
		if prompt != "" {
			args = append(args, prompt)
		}
		return args, nil
	case "opencode":
		if prompt != "" {
			return append([]string{"opencode", "run"}, prompt), nil
		}
		return []string{"opencode"}, nil
	case "custom":
		if mode == "" {
			return nil, errors.New("custom agent requires --mode as the executable command")
		}
		return strings.Fields(mode), nil
	default:
		return nil, fmt.Errorf("unsupported agent %q (use codex, claude, opencode, or custom)", agent)
	}
}

// BuildReviewCommand builds argv for a non-interactive review step in a stage tmux session.
func BuildReviewCommand(agent, model, baseBranch string) ([]string, error) {
	agent = NormalizeTaskAgent(agent)
	if agent == "" {
		agent = "codex"
	}
	switch agent {
	case "codex":
		args := []string{"codex", "review"}
		if strings.TrimSpace(baseBranch) != "" {
			args = append(args, "--base", strings.TrimSpace(baseBranch))
		} else {
			args = append(args, "--uncommitted")
		}
		if model != "" {
			args = append(args, "-m", model)
		}
		return args, nil
	default:
		return nil, fmt.Errorf(
			"review command not defined for agent %q; set task.agents.%s.review_command in ~/.gmc.yaml",
			agent, agent,
		)
	}
}
