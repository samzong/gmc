package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyPaneCommand(t *testing.T) {
	assert.Equal(t, RuntimeIdle, ClassifyPaneCommand("zsh", "codex"))
	assert.Equal(t, RuntimeInteractive, ClassifyPaneCommand("node", "codex"))
	assert.Equal(t, RuntimeInteractive, ClassifyPaneCommand("node", ""))
	assert.Equal(t, RuntimeOffline, InspectSessionRuntime(AttemptRecord{}, nil).Status)
}

func TestRuntimeUserMessageInteractive(t *testing.T) {
	msg := runtimeUserMessage(RuntimeInteractive, "node", AttemptRecord{Agent: "codex"}, nil)
	assert.Contains(t, msg, "cannot tell if the task finished")
	assert.Contains(t, msg, "headless runs")
}

func TestLastRunHint(t *testing.T) {
	exit := 0
	msg := lastRunHint([]RunRecord{{
		ID: "run-1", Type: RunTypeCommandCheck, Runtime: RuntimeHeadless,
		State: RunPassed, ExitCode: &exit,
	}})
	assert.Contains(t, msg, "passed")
}
