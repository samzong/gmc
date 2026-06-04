package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TaskContextRelPath is the worktree-local task brief for agents and humans.
const TaskContextRelPath = ".gmc/TASK.md"

// WriteTaskContextFile writes the full task brief into the attempt worktree.
func WriteTaskContextFile(worktree string, task Record, attempt AttemptRecord) (string, error) {
	content := BuildTaskContextMarkdown(task, attempt)
	dir := filepath.Join(worktree, filepath.Dir(TaskContextRelPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	abs := filepath.Join(worktree, TaskContextRelPath)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	return abs, nil
}

// BuildTaskContextMarkdown is the durable task brief copied into the worktree.
func BuildTaskContextMarkdown(task Record, attempt AttemptRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", DisplayTitle(task))
	fmt.Fprintf(&b, "- **Task ID:** %s\n", task.ID)
	if task.PR != "" {
		fmt.Fprintf(&b, "- **PR:** #%s\n", task.PR)
	}
	if task.Issue != "" {
		fmt.Fprintf(&b, "- **Issue:** #%s\n", task.Issue)
	}
	if attempt.Branch != "" {
		fmt.Fprintf(&b, "- **Branch:** %s\n", attempt.Branch)
	}
	if attempt.Worktree != "" {
		fmt.Fprintf(&b, "- **Worktree:** %s\n", attempt.Worktree)
	}
	if attempt.Agent != "" {
		line := fmt.Sprintf("- **Agent:** %s", attempt.Agent)
		if attempt.Model != "" {
			line += fmt.Sprintf(" (%s)", attempt.Model)
		}
		if attempt.Mode != "" {
			line += fmt.Sprintf(" mode=%s", attempt.Mode)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n## Source\n\n")
	b.WriteString(task.Source)
	b.WriteString("\n\n## Instructions\n\n")
	b.WriteString("Execute the task described in **Source** above in this worktree. ")
	b.WriteString("Treat this file as the task contract for the gmc control plane.\n")
	return b.String()
}

// InitialAgentPrompt is the argv prompt passed when starting an interactive agent.
func InitialAgentPrompt(task Record) string {
	var parts []string
	parts = append(parts, "gmc task:")
	parts = append(parts, DisplayTitle(task))
	if task.PR != "" {
		parts = append(parts, fmt.Sprintf("(PR #%s)", task.PR))
	}
	parts = append(parts, "— read and execute .gmc/TASK.md in this worktree.")
	return strings.Join(parts, " ")
}
