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

func TestDefaultWorkflowNodeInheritsStartAgent(t *testing.T) {
	cfg := DefaultWorkflowConfig()
	wf, err := SelectWorkflow(cfg, "")
	require.NoError(t, err)
	node, err := workflowStartNode(wf)
	require.NoError(t, err)

	agent, err := WorkflowNodeAgent(node, "grok")
	require.NoError(t, err)
	assert.Equal(t, "grok", agent)
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
