package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/samzong/gmc/internal/exitcode"
	"github.com/samzong/gmc/internal/git"
	"github.com/samzong/gmc/internal/llm"
	"github.com/samzong/gmc/internal/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyErrorMapsFailuresToExitCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "llm error", err: llm.ErrLLM, want: exitcode.LLMError},
		{name: "wrapped llm error", err: fmt.Errorf("call: %w", llm.ErrLLM), want: exitcode.LLMError},
		{name: "no commit subject", err: workflow.ErrNoCommitSubject, want: exitcode.LLMError},
		{
			name: "wrapped no commit subject",
			err:  fmt.Errorf("generate: %w", workflow.ErrNoCommitSubject),
			want: exitcode.LLMError,
		},
		{name: "not a git repository", err: git.ErrNotGitRepo, want: exitcode.NotGitRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classified := classifyError(tt.err)
			require.NotNil(t, classified)
			assert.Equal(t, tt.want, classified.Code)
			assert.ErrorIs(t, classified, tt.err)
		})
	}
}

func TestClassifyErrorLeavesOtherErrorsUnclassified(t *testing.T) {
	assert.Nil(t, classifyError(errors.New("something else")))
	assert.Nil(t, classifyError(workflow.ErrNoChanges))
}
