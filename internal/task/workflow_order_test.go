package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowNodeOrderDefaultWorkflow(t *testing.T) {
	cfg := DefaultWorkflowConfig()
	wf, err := SelectWorkflow(cfg, "")
	require.NoError(t, err)

	assert.Equal(t, []string{"plan", "code", "review", "ship"}, WorkflowNodeOrder(wf))
}

func TestWorkflowNodeOrderAppendsUnlinkedNodes(t *testing.T) {
	wf := WorkflowDefinition{
		Name:  "test",
		Start: "a",
		Nodes: map[string]WorkflowNode{
			"a": {ID: "a", Next: "b"},
			"b": {ID: "b", Next: "done"},
			"z": {ID: "z"},
		},
	}
	assert.Equal(t, []string{"a", "b", "z"}, WorkflowNodeOrder(wf))
}
