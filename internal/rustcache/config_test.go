package rustcache

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigurationPreservesExistingFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, Enable(root, "/tools/gmc-rustc"))
	config := filepath.Join(root, ".cargo", "config.toml")
	changed := []byte("[build]\njobs = 2\n")
	require.NoError(t, os.WriteFile(config, changed, 0600))
	require.Error(t, Enable(root, "/tools/new-gmc-rustc"))
	actual, err := os.ReadFile(config)
	require.NoError(t, err)
	require.Equal(t, changed, actual)
}

func TestInheritedWrapperPreventsInstallation(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	require.NoError(t, os.Mkdir(root, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[package]\n"), 0600))
	t.Setenv("CARGO_HOME", t.TempDir())
	eligible, reason := Eligible(root)
	require.True(t, eligible, reason)
	require.NoError(t, os.Mkdir(filepath.Join(parent, ".cargo"), 0755))
	path := filepath.Join(parent, ".cargo", "config.toml")
	for _, content := range []string{
		"[build]\nrustc-wrapper = ''\n", "[build]\nrustc-workspace-wrapper = 'custom'\n", "include = ['other.toml']\n",
	} {
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		eligible, _ = Eligible(root)
		require.False(t, eligible)
	}
	require.NoError(t, os.Remove(path))
	t.Setenv("RUSTC_WRAPPER", "")
	eligible, _ = Eligible(root)
	require.False(t, eligible)
}

func TestReleaseIntegrityAndExtraction(t *testing.T) {
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gz)
	contents := []byte("verified binary")
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "release/kache", Mode: 0755, Size: int64(len(contents))}))
	_, err := tarWriter.Write(contents)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gz.Close())
	r := release{name: "release.tar.gz", sha: fmt.Sprintf("%x", sha256.Sum256(compressed.Bytes()))}
	got, err := releaseBinary(compressed.Bytes(), r)
	require.NoError(t, err)
	require.Equal(t, contents, got)
	corrupt := append([]byte{}, compressed.Bytes()...)
	corrupt[len(corrupt)-1] ^= 1
	_, err = releaseBinary(corrupt, r)
	require.Error(t, err)
}
