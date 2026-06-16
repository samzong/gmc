package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflowConfigDirectNodes(t *testing.T) {
	cfg, err := ParseWorkflowConfig([]byte(`
version: 1
start: plan
nodes:
  plan:
    agent: grok
    prompt: Plan it.
    next: code
  code:
    skill: simplify
    prompt: Code it.
    next: done
`))
	require.NoError(t, err)

	wf, err := SelectWorkflow(cfg, "")
	require.NoError(t, err)
	assert.Equal(t, DefaultWorkflowName, wf.Name)
	assert.Equal(t, "plan", wf.Start)
	assert.Equal(t, "grok", wf.Nodes["plan"].Agent)
	assert.Equal(t, []string{"simplify"}, wf.Nodes["code"].Skills)
}

func TestDefaultWorkflowConfigUsesEmbeddedWorkflow(t *testing.T) {
	cfg := DefaultWorkflowConfig()
	wf, err := SelectWorkflow(cfg, "")
	require.NoError(t, err)

	assert.Equal(t, "plan", wf.Start)
	assert.Equal(t, "codex", wf.Nodes["plan"].Agent)
	assert.Equal(t, "codex --dangerously-bypass-approvals-and-sandbox", wf.Nodes["plan"].Command)
	assert.Equal(t, []string{"systematic-debugging"}, wf.Nodes["plan"].Skills)
	assert.Equal(t, "code", wf.Nodes["plan"].Next)
	assert.Equal(t, "grok", wf.Nodes["code"].Agent)
	assert.Equal(t, "grok --yolo", wf.Nodes["code"].Command)
	assert.Equal(t, "cursor-agent", wf.Nodes["review"].Agent)
	assert.Equal(t, "ship", wf.Nodes["review"].Next)
	assert.Equal(t, "done", wf.Nodes["ship"].Next)

	agent, err := WorkflowNodeAgent(wf.Nodes["plan"], "grok")
	require.NoError(t, err)
	assert.Equal(t, "codex", agent)
}

func TestParseWorkflowConfigRejectsUnsupportedAgent(t *testing.T) {
	_, err := ParseWorkflowConfig([]byte(`
version: 1
nodes:
  plan:
    agent: claude
    prompt: Plan it.
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "codex, grok, cursor-agent, or opencode")
}
