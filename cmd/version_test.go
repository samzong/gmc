package cmd

import (
	builddebug "runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolvedVersion(t *testing.T) {
	for _, tt := range []struct {
		name      string
		linked    string
		module    string
		available bool
		want      string
	}{
		{"linker", "v1.2.3-rc.1", "v9.0.0", true, "1.2.3-rc.1"},
		{"installed module", "dev", "v1.2.3-rc.1+build.01", true, "1.2.3-rc.1+build.01"},
		{"empty linker", "", "v1.2.3", true, "1.2.3"},
		{"source build", "dev", "(devel)", true, "dev"},
		{"empty module", "dev", "", true, "dev"},
		{"no metadata", "dev", "v1.2.3", false, "dev"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setTestValue(t, &Version, tt.linked)
			setTestValue(t, &readBuildInfo, func() (*builddebug.BuildInfo, bool) {
				return &builddebug.BuildInfo{Main: builddebug.Module{Version: tt.module}}, tt.available
			})
			assert.Equal(t, tt.want, resolvedVersion())
		})
	}
}
