package task

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// WorkflowConfig holds stage commands for one agent adapter.
type WorkflowConfig struct {
	ReviewCommand []string
	VerifyCommand []string
}

// LoadWorkflowForAgent reads task stage commands for the given agent.
//
// ~/.gmc.yaml example:
//
//	task:
//	  agents:
//	    codex:
//	      review_command: ["codex", "review", "--uncommitted"]
//	      verify_command: ["make", "check"]
//	    claude-code:
//	      review_command: ["claude", "-p", "review the diff"]
//	      verify_command: ["make", "check"]
//
// Legacy global keys task.review_command / task.verify_command are used when
// no per-agent block is set.
func LoadWorkflowForAgent(agent string) WorkflowConfig {
	agent = NormalizeTaskAgent(agent)
	review := commandSliceForAgent(agent, "review_command")
	verify := commandSliceForAgent(agent, "verify_command")
	if len(review) == 0 {
		review = viper.GetStringSlice("task.review_command")
	}
	if len(verify) == 0 {
		verify = viper.GetStringSlice("task.verify_command")
	}
	if len(verify) == 0 {
		verify = []string{"make", "check"}
	}
	return WorkflowConfig{ReviewCommand: review, VerifyCommand: verify}
}

func commandSliceForAgent(agent, field string) []string {
	for _, key := range agentConfigLookupKeys(agent) {
		path := fmt.Sprintf("task.agents.%s.%s", key, field)
		if s := viper.GetStringSlice(path); len(s) > 0 {
			return s
		}
	}
	return nil
}

// agentConfigLookupKeys returns yaml map keys to try (normalized + common aliases).
func agentConfigLookupKeys(agent string) []string {
	n := NormalizeTaskAgent(agent)
	seen := map[string]bool{}
	var keys []string
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			return
		}
		seen[k] = true
		keys = append(keys, k)
	}
	add(n)
	raw := strings.ToLower(strings.TrimSpace(agent))
	if raw != n {
		add(raw)
	}
	switch n {
	case "claude":
		add("claude-code")
		add("claude_code")
	case "codex":
		add("codex-cli")
	}
	return keys
}

// ResolveReviewCommand picks review argv: config > built-in adapter defaults.
func ResolveReviewCommand(agent, model, baseBranch string) ([]string, error) {
	wf := LoadWorkflowForAgent(agent)
	if len(wf.ReviewCommand) > 0 {
		return append([]string(nil), wf.ReviewCommand...), nil
	}
	return BuildReviewCommand(agent, model, baseBranch)
}

// ResolveVerifyCommand picks verify argv from config (required unless legacy/global set).
func ResolveVerifyCommand(agent string) ([]string, error) {
	wf := LoadWorkflowForAgent(agent)
	if len(wf.VerifyCommand) == 0 {
		return nil, fmt.Errorf(
			"no verify_command for agent %q; set task.agents.%s.verify_command in ~/.gmc.yaml",
			agent, NormalizeTaskAgent(agent),
		)
	}
	return append([]string(nil), wf.VerifyCommand...), nil
}

// NextTaskState returns the default state after advance from current.
func NextTaskState(current string) (string, bool) {
	switch current {
	case TaskRunning:
		return TaskReviewing, true
	case TaskReviewing:
		return TaskVerifying, true
	case TaskVerifying:
		return TaskReadyForPR, true
	case TaskReadyForPR:
		return TaskDone, true
	default:
		return "", false
	}
}
