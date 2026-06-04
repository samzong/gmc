package task

import (
	"os/exec"
	"strconv"
	"strings"
)

// RuntimeStatus describes tmux/agent activity (ephemeral; not the same as task STATE).
type RuntimeStatus string

const (
	// RuntimeInteractive: agent TUI is in the pane; gmc cannot know if assigned work finished.
	RuntimeInteractive RuntimeStatus = "interactive"
	// RuntimeAwaitingInput: pane text suggests the agent may be waiting (best-effort).
	RuntimeAwaitingInput RuntimeStatus = "awaiting-input"
	// RuntimeIdle: shell owns the pane (agent exited). Rare for Codex interactive sessions.
	RuntimeIdle RuntimeStatus = "idle"
	// RuntimeOffline: no tmux session.
	RuntimeOffline RuntimeStatus = "offline"
	RuntimeUnknown RuntimeStatus = "unknown"
)

// SessionInspect is the result of inspecting a tmux session.
type SessionInspect struct {
	Status      RuntimeStatus
	PaneCommand string
	UserMessage string
}

// InspectSessionRuntime probes tmux without changing task state.
func InspectSessionRuntime(attempt AttemptRecord, runs []RunRecord) SessionInspect {
	if attempt.TmuxSession == "" {
		return SessionInspect{
			Status:      RuntimeOffline,
			UserMessage: "No tmux session recorded for this attempt.",
		}
	}
	profile, err := TmuxProfileForAttempt(attempt)
	if err != nil {
		return SessionInspect{Status: RuntimeUnknown, UserMessage: err.Error()}
	}
	if !tmuxHasSession(profile) {
		if profile.Socket != "" && tmuxHasSession(TmuxProfile{Session: profile.Session}) {
			profile = TmuxProfile{Session: profile.Session}
		} else {
			return SessionInspect{
				Status:      RuntimeOffline,
				UserMessage: "Tmux session is gone.",
			}
		}
	}
	cmd := tmuxPaneCommand(profile)
	status := ClassifyPaneCommand(cmd, attempt.Agent)
	if status == RuntimeInteractive && detectAwaitingInput(profile, attempt.Agent) {
		status = RuntimeAwaitingInput
	}
	return SessionInspect{
		Status:      status,
		PaneCommand: cmd,
		UserMessage: runtimeUserMessage(status, cmd, attempt, runs),
	}
}

func tmuxPaneCommand(profile TmuxProfile) string {
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return ""
	}
	args := append(base, "list-panes", "-t", profile.Session, "-F", "#{pane_current_command}")
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

func tmuxCapturePaneTail(profile TmuxProfile, lineCount int) string {
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return ""
	}
	args := append(base, "capture-pane", "-t", profile.Session, "-p", "-S", "-"+strconv.Itoa(lineCount))
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// ClassifyPaneCommand maps tmux pane_current_command and agent type to runtime status.
func ClassifyPaneCommand(command, agent string) RuntimeStatus {
	cmd := strings.ToLower(strings.TrimSpace(command))
	agent = strings.ToLower(strings.TrimSpace(agent))
	switch cmd {
	case "", "tmux":
		return RuntimeUnknown
	case "zsh", "bash", "sh", "fish", "dash", "ksh", "tcsh":
		return RuntimeIdle
	}
	if isInteractiveAgent(agent) || isLikelyAgentProcess(cmd) {
		return RuntimeInteractive
	}
	return RuntimeUnknown
}

func isInteractiveAgent(agent string) bool {
	switch agent {
	case "codex", "claude", "opencode":
		return true
	default:
		return false
	}
}

func isLikelyAgentProcess(cmd string) bool {
	return cmd == "node" || cmd == "codex" || strings.Contains(cmd, "claude") ||
		strings.Contains(cmd, "opencode")
}

// detectAwaitingInput is a best-effort pane scrape; false negatives are expected.
func detectAwaitingInput(profile TmuxProfile, agent string) bool {
	tail := strings.ToLower(tmuxCapturePaneTail(profile, 40))
	if tail == "" {
		return false
	}
	switch strings.ToLower(agent) {
	case "codex":
		// Codex interactive UI often keeps node in the pane; look for input-area hints.
		return strings.Contains(tail, "send a message") ||
			strings.Contains(tail, "ask codex") ||
			strings.Contains(tail, "message codex") ||
			(!strings.Contains(tail, "esc to interrupt") && strings.Contains(tail, "›"))
	default:
		return false
	}
}

func runtimeUserMessage(status RuntimeStatus, paneCommand string, attempt AttemptRecord, runs []RunRecord) string {
	var parts []string
	agent := attempt.Agent
	if agent == "" {
		agent = "agent"
	}

	switch status {
	case RuntimeInteractive:
		parts = append(parts,
			"Interactive "+agent+" session in tmux (pane: "+paneCommand+"). "+
				"Codex-style agents stay in the TUI and do not exit to shell, "+
				"so gmc cannot tell if the task finished.",
		)
	case RuntimeAwaitingInput:
		parts = append(parts,
			"Interactive "+agent+" may be waiting for input (pane: "+paneCommand+"). "+
				"Use watch to read the screen; this is a best-effort hint, not proof the task is done.",
		)
	case RuntimeIdle:
		parts = append(parts,
			"Tmux is at a shell prompt ("+paneCommand+"); the interactive agent likely exited.",
		)
	case RuntimeOffline:
		return "Tmux session is not running."
	default:
		parts = append(parts, "Could not classify tmux activity (pane: "+paneCommand+").")
	}

	if hint := lastRunHint(runs); hint != "" {
		parts = append(parts, hint)
	}
	parts = append(parts,
		"For verifiable completion, use headless runs: gmc task run <id> -- <command>.",
	)
	return strings.Join(parts, " ")
}

func lastRunHint(runs []RunRecord) string {
	for i := len(runs) - 1; i >= 0; i-- {
		r := runs[i]
		if r.Runtime != RuntimeHeadless {
			continue
		}
		exit := "?"
		if r.ExitCode != nil {
			exit = strconv.Itoa(*r.ExitCode)
		}
		switch r.State {
		case RunPassed:
			return "Last headless run " + r.ID + " (" + r.Type + ") passed (exit " + exit + ")."
		case RunFailed:
			return "Last headless run " + r.ID + " (" + r.Type + ") failed (exit " + exit + ")."
		case RunRunning:
			return "Headless run " + r.ID + " (" + r.Type + ") is still running."
		}
	}
	return ""
}

// PrimaryRuntimeStatus inspects the latest attempt on a summary.
func PrimaryRuntimeStatus(sum Summary) RuntimeStatus {
	if len(sum.Attempts) == 0 {
		return RuntimeOffline
	}
	attempt := sum.Attempts[len(sum.Attempts)-1]
	return InspectSessionRuntime(attempt, sum.Runs).Status
}

// FormatRuntimeLabel is a short label for task list.
func FormatRuntimeLabel(status RuntimeStatus) string {
	switch status {
	case RuntimeInteractive:
		return "interactive"
	case RuntimeAwaitingInput:
		return "awaiting"
	case RuntimeIdle:
		return "idle"
	case RuntimeOffline:
		return "offline"
	default:
		return "unknown"
	}
}
