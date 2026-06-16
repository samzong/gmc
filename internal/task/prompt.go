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
	if rec.Workflow != "" {
		fmt.Fprintf(&b, "- Workflow: %s\n", rec.Workflow)
	}
	if rec.CurrentNode != "" {
		fmt.Fprintf(&b, "- Current node: %s\n", rec.CurrentNode)
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
	b.WriteString("Follow the current workflow node only. ")
	b.WriteString("Stop when that node is complete and wait for the operator to run gmc task advance.\n")
	return b.String()
}

func BuildWorkflowNodePrompt(rec Record, node WorkflowNode) string {
	var b strings.Builder
	fmt.Fprintf(&b, "gmc task workflow: %s\n", DisplayTitle(rec))
	fmt.Fprintf(&b, "Task ID: %s\n", rec.ID)
	if rec.Workflow != "" {
		fmt.Fprintf(&b, "Workflow: %s\n", rec.Workflow)
	}
	fmt.Fprintf(&b, "Node: %s\n\n", node.ID)
	b.WriteString("Read .gmc/TASK.md and any previous handoff files under .gmc/workflow/.\n\n")
	if len(node.Skills) > 0 {
		b.WriteString("Requested skills:\n")
		for _, skill := range node.Skills {
			fmt.Fprintf(&b, "- %s\n", skill)
		}
		b.WriteString("\n")
	}
	b.WriteString("Node instructions:\n")
	b.WriteString(strings.TrimSpace(node.Prompt))
	b.WriteString("\n\nBefore stopping, write or update .gmc/workflow/")
	b.WriteString(node.ID)
	b.WriteString(".md with a concise handoff: result, changed files, verification, risks, ")
	b.WriteString("and recommended next step. Then stop and wait for the operator.")
	return b.String()
}
