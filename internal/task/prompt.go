package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const TaskContextRelPath = ".gmc/TASK.md"

func WriteTaskContextFile(worktree string, rec Record, attempt AttemptRecord) (string, error) {
	dir := filepath.Join(worktree, filepath.Dir(TaskContextRelPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(worktree, TaskContextRelPath)
	if err := os.WriteFile(path, []byte(BuildTaskContextMarkdown(rec, attempt)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func BuildTaskContextMarkdown(rec Record, attempt AttemptRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", DisplayTitle(rec))
	fmt.Fprintf(&b, "- Task ID: %s\n", rec.ID)
	fmt.Fprintf(&b, "- State: %s\n", rec.State)
	if rec.Issue != "" {
		fmt.Fprintf(&b, "- Issue: #%s\n", rec.Issue)
	}
	if rec.SourceFile != "" {
		fmt.Fprintf(&b, "- Source file: %s\n", rec.SourceFile)
	}
	if attempt.Branch != "" {
		fmt.Fprintf(&b, "- Branch: %s\n", attempt.Branch)
	}
	if attempt.Worktree != "" {
		fmt.Fprintf(&b, "- Worktree: %s\n", attempt.Worktree)
	}
	if attempt.Agent != "" {
		fmt.Fprintf(&b, "- Agent: %s\n", attempt.Agent)
	}
	b.WriteString("\n## Source\n\n")
	b.WriteString(strings.TrimSpace(rec.Source))
	b.WriteString("\n\n## Contract\n\n")
	b.WriteString("Analyze the source, produce or refine the plan first, ")
	b.WriteString("then wait for the operator to move the task through code, review, and ship.\n")
	return b.String()
}

func InitialAgentPrompt(rec Record) string {
	return "gmc task: " + DisplayTitle(rec) + " - read .gmc/TASK.md and start with planning."
}
