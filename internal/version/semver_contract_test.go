package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSemVerContract(t *testing.T) {
	for _, tag := range []string{"v1.2.3-alpha.1", "v1.2.3-beta.2", "v1.2.3-rc.10+build.01", "v1.2.3+build.1", "v0.0.0"} {
		t.Run(tag, func(t *testing.T) {
			v, err := ParseSemVer(tag)
			require.NoError(t, err)
			require.Equal(t, tag, v.String())
		})
	}
	for _, tag := range []string{
		"v1.02.3", "v01.2.3", "v1.2.03", "v1.2.3-fix.1", "v1.2.3-rc.01",
		"v1.2.3+", "v1.2.3+build..1", "", "v1.2", "v-1.2.3",
	} {
		t.Run(tag, func(t *testing.T) {
			_, err := ParseSemVer(tag)
			require.Error(t, err)
		})
	}
}

func TestSemVerPrecedence(t *testing.T) {
	for _, pair := range [][2]string{
		{"v1.9.0", "v1.10.0"}, {"v1.2.3-alpha.1", "v1.2.3-beta.1"},
		{"v1.2.3-beta.1", "v1.2.3-rc.1"}, {"v1.2.3-rc.2", "v1.2.3-rc.10"},
		{"v1.2.3-rc.1", "v1.2.3"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			lower, err := ParseSemVer(pair[0])
			require.NoError(t, err)
			higher, err := ParseSemVer(pair[1])
			require.NoError(t, err)
			require.True(t, lower.LessThan(higher))
			require.False(t, higher.LessThan(lower))
			require.False(t, lower.Equal(higher))
		})
	}
	left, err := ParseSemVer("v1.2.3+build.1")
	require.NoError(t, err)
	right, err := ParseSemVer("v1.2.3+build.2")
	require.NoError(t, err)
	require.True(t, left.Equal(right))
	require.False(t, left.LessThan(right))
	require.False(t, right.LessThan(left))
}
