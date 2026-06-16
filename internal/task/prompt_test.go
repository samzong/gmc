package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTaskContextFile(t *testing.T) {
	dir := t.TempDir()
	rec := Record{ID: "t-1", State: TaskPlan, Source: "fix the bug", Title: "Fix bug"}
	attempt := AttemptRecord{ID: "attempt-1", Worktree: dir, Branch: "_task/t-1/1", Agent: "codex"}

	path, err := WriteTaskContextFile(dir, rec, attempt)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, TaskContextRelPath), path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "Fix bug")
	assert.Contains(t, text, "fix the bug")
	assert.Contains(t, text, "_task/t-1/1")
}

func TestBuildWorkflowNodePrompt(t *testing.T) {
	rec := Record{ID: "t-1", Title: "Fix bug", Workflow: "default"}
	node := WorkflowNode{ID: "review", Prompt: "Review the diff.", Skills: []string{"local-code-liability-review"}}
	text := BuildWorkflowNodePrompt(rec, node)
	assert.Contains(t, text, ".gmc/TASK.md")
	assert.Contains(t, text, "Node: review")
	assert.Contains(t, text, "local-code-liability-review")
	assert.Contains(t, text, ".gmc/workflow/review.md")
}

func TestAgentCommandCodex(t *testing.T) {
	args, err := AgentCommand("codex", "gpt-5", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"codex", "-m", "gpt-5", "do the task"}, args)
}

func TestAgentCommandGrok(t *testing.T) {
	args, err := AgentCommand("grok", "grok-4", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"grok", "-m", "grok-4", "do the task"}, args)
}

func TestAgentCommandCursorAgent(t *testing.T) {
	args, err := AgentCommand("cursor-agent", "gpt-5", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"cursor-agent", "--model", "gpt-5", "do the task"}, args)
}

func TestAgentCommandOpencode(t *testing.T) {
	args, err := AgentCommand("opencode", "", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{"opencode", "run", "do the task"}, args)
}

func TestAgentCommandUnsupported(t *testing.T) {
	_, err := AgentCommand("claude", "", "do the task")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "codex, grok, cursor-agent, or opencode")
}

func TestWorkflowNodeCommandOverride(t *testing.T) {
	node := WorkflowNode{ID: "plan", Command: "codex --dangerously-bypass-approvals-and-sandbox --model gpt-5"}
	args, err := WorkflowNodeCommand(node, "codex", "ignored-model", "do the task")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"codex",
		"--dangerously-bypass-approvals-and-sandbox",
		"--model",
		"gpt-5",
		"do the task",
	}, args)
}
