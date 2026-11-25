package git

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLatestSemVerTag(t *testing.T) {
	client := setupRepo(t)
	require.NoError(t, os.WriteFile("file", []byte("initial"), 0o600))
	require.NoError(t, client.AddAll())
	require.NoError(t, client.Commit("chore: initialize"))
	for _, tag := range []string{"develop", "v1.02.3", "v1.2.3-fix.1"} {
		require.NoError(t, client.CreateAnnotatedTag(tag, tag))
	}
	tag, err := client.GetLatestTag()
	require.NoError(t, err)
	require.Empty(t, tag)
	for _, tag := range []string{"v1.2.3-rc.1", "v1.2.3", "v1.10.0", "v1.9.0"} {
		require.NoError(t, client.CreateAnnotatedTag(tag, tag))
	}
	tag, err = client.GetLatestTag()
	require.NoError(t, err)
	require.Equal(t, "v1.10.0", tag)
}
