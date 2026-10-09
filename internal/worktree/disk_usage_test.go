package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i)
	}
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func measure(t *testing.T, root string) uint64 {
	t.Helper()
	size, err := MeasureAllocated(root)
	require.NoError(t, err)
	return size
}

func TestMeasureAllocatedSkipsRootGitAndBare(t *testing.T) {
	base := t.TempDir()
	plain := filepath.Join(base, "plain")
	require.NoError(t, os.MkdirAll(plain, 0o755))
	writeSizedFile(t, filepath.Join(plain, "src", "main.go"), 64*1024)
	baseline := measure(t, plain)

	withGitDir := filepath.Join(base, "main")
	require.NoError(t, os.CopyFS(withGitDir, os.DirFS(plain)))
	writeSizedFile(t, filepath.Join(withGitDir, ".git", "objects", "pack"), 512*1024)
	writeSizedFile(t, filepath.Join(withGitDir, ".bare", "objects", "pack"), 512*1024)
	assert.Equal(t, baseline, measure(t, withGitDir))

	withGitFile := filepath.Join(base, "linked")
	require.NoError(t, os.CopyFS(withGitFile, os.DirFS(plain)))
	writeSizedFile(t, filepath.Join(withGitFile, ".git"), 256*1024)
	assert.Equal(t, baseline, measure(t, withGitFile))

	nested := filepath.Join(plain, "vendor", ".git")
	writeSizedFile(t, nested, 256*1024)
	assert.Greater(t, measure(t, plain), baseline)
}

func TestMeasureAllocatedDoesNotFollowSymlinks(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	writeSizedFile(t, filepath.Join(target, "node_modules", "big.bin"), 1024*1024)
	root := filepath.Join(base, "root")
	require.NoError(t, os.MkdirAll(root, 0o755))
	baseline := measure(t, root)

	if err := os.Symlink(target, filepath.Join(root, "node_modules")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	require.NoError(t, os.Symlink(filepath.Join(target, "node_modules", "big.bin"), filepath.Join(root, "big.bin")))

	assert.Less(t, measure(t, root)-baseline, uint64(64*1024))
	assert.GreaterOrEqual(t, measure(t, target), uint64(1024*1024))
}

func TestMeasureAllocatedCountsHardLinksOnce(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "a.bin")
	writeSizedFile(t, original, 1024*1024)
	single := measure(t, root)

	if err := os.Link(original, filepath.Join(root, "b.bin")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	assert.Equal(t, single, measure(t, root))
}

func TestMeasureAllocatedReportsMissingRoot(t *testing.T) {
	_, err := MeasureAllocated(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestMeasureAllocatedAllKeepsInputOrder(t *testing.T) {
	base := t.TempDir()
	var paths []string
	for i, size := range []int{4096, 512 * 1024, 0, 64 * 1024} {
		path := filepath.Join(base, string(rune('a'+i)))
		require.NoError(t, os.MkdirAll(path, 0o755))
		if size > 0 {
			writeSizedFile(t, filepath.Join(path, "data"), size)
		}
		paths = append(paths, path)
	}
	missing := filepath.Join(base, "missing")
	paths = append(paths, missing)

	results := MeasureAllocatedAll(paths)
	require.Len(t, results, len(paths))
	for i, path := range paths[:len(paths)-1] {
		assert.Equal(t, path, results[i].Path)
		require.NoError(t, results[i].Err)
		assert.Equal(t, measure(t, path), results[i].Bytes)
	}
	assert.Equal(t, missing, results[len(paths)-1].Path)
	assert.Error(t, results[len(paths)-1].Err)
}

func TestFormatAllocatedSize(t *testing.T) {
	const (
		k = uint64(1) << 10
		m = k << 10
		g = m << 10
		p = g << 20
	)
	tests := []struct {
		bytes uint64
		want  string
	}{
		{0, "0B"},
		{1, "1B"},
		{1023, "1023B"},
		{k, "1.0K"},
		{1536, "1.5K"},
		{10*k - 1, "10K"},
		{10 * k, "10K"},
		{348 * m, "348M"},
		{1288490188, "1.2G"},
		{m - 1, "1.0M"},
		{g - 41943, "1.0G"},
		{1023 * m, "1023M"},
		{p, "1.0P"},
		{2048 * p, "2048P"},
		{^uint64(0), "16384P"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FormatAllocatedSize(tt.bytes), "bytes=%d", tt.bytes)
	}
}
