package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReviewCommandCodex(t *testing.T) {
	args, err := BuildReviewCommand("codex", "gpt-5", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"codex", "review", "--uncommitted", "-m", "gpt-5"}, args)
}
