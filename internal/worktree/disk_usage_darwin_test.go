package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestReadBulkAttributes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "data")
	writeSizedFile(t, path, 8192)
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, unix.Close(fd)) })
	buffer := make([]byte, 16*1024)
	n, err := readBulkAttributes(fd, buffer)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	entry, _, err := parseBulkEntry(buffer)
	require.NoError(t, err)
	require.NoError(t, entry.err)
	assert.False(t, entry.fallback)
	assert.Equal(t, "data", entry.name)
	info, err := os.Lstat(path)
	require.NoError(t, err)
	want, err := statAllocated(path, info)
	require.NoError(t, err)
	assert.Equal(t, want, entry.allocated)
}

func TestMeasureAllocatedDarwinMatchesWalker(t *testing.T) {
	root := t.TempDir()
	writeSizedFile(t, filepath.Join(root, "src", "data"), 128*1024)
	writeSizedFile(t, filepath.Join(root, ".git", "objects", "pack"), 1024*1024)
	writeSizedFile(t, filepath.Join(root, ".bare", "objects", "pack"), 1024*1024)
	writeSizedFile(t, filepath.Join(root, "vendor", ".git", "data"), 8192)
	require.NoError(t, os.Link(filepath.Join(root, "src", "data"), filepath.Join(root, "vendor", "linked")))
	require.NoError(t, os.Symlink(root, filepath.Join(root, "src", "cycle")))
	require.NoError(t, os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "dangling")))
	outside := t.TempDir()
	writeSizedFile(t, filepath.Join(outside, "large"), 1024*1024)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "outside")))
	sparse := filepath.Join(root, "sparse")
	f, err := os.Create(sparse)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(1<<30))
	_, err = f.WriteAt(make([]byte, 8192), (1<<30)-8192)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	fork := filepath.Join(root, "fork")
	writeSizedFile(t, fork, 4096)
	require.NoError(t, os.WriteFile(fork+"/..namedfork/rsrc", make([]byte, 64*1024), 0o600))
	for _, path := range []string{root, sparse, fork, filepath.Join(root, "outside")} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			want, err := measureAllocatedWalk(path)
			require.NoError(t, err)
			got, err := MeasureAllocated(path)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestMeasureAllocatedDarwinErrorsMatchWalker(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	require.NoError(t, os.Mkdir(blocked, 0o700))
	writeSizedFile(t, filepath.Join(blocked, "data"), 4096)
	require.NoError(t, os.Chmod(blocked, 0))
	t.Cleanup(func() { require.NoError(t, os.Chmod(blocked, 0o700)) })
	paths := []string{filepath.Join(t.TempDir(), "missing")}
	if os.Geteuid() != 0 {
		paths = append(paths, root)
	}
	for i, size := range MeasureAllocatedAll(paths) {
		want, err := measureAllocatedWalk(paths[i])
		require.Error(t, err)
		assert.Equal(t, want, size.Bytes)
		require.EqualError(t, size.Err, err.Error())
		assert.Equal(t, MeasureErrorCode(err), MeasureErrorCode(size.Err))
		assert.Equal(t, i == 0, size.RootMissing())
	}
}
