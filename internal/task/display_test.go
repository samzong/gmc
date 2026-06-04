package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListAgentMode(t *testing.T) {
	agent, mode := ListAgentMode(Summary{})
	assert.Equal(t, "-", agent)
	assert.Equal(t, "-", mode)

	agent, mode = ListAgentMode(Summary{
		Attempts: []AttemptRecord{{Agent: "codex", Mode: "coding"}},
	})
	assert.Equal(t, "codex", agent)
	assert.Equal(t, "coding", mode)
}
