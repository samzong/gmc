package worktree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddReturnsCreateResult(t *testing.T) {
	repoDir := initTestRepo(t)
	t.Chdir(repoDir)
	runGit(t, repoDir, "branch", "existing")
	client := NewClient(Options{})

	created, err := client.Add("feature", AddOptions{BaseBranch: "main"})
	require.NoError(t, err)
	assert.Equal(t, "feature", created.Name)
	assert.Equal(t, "feature", created.Branch)
	assert.Equal(t, "main", created.Base)
	assert.True(t, created.Created)
	assert.NoError(t, created.Err)
	assert.DirExists(t, created.Path)

	reused, err := client.Add("existing", AddOptions{})
	require.NoError(t, err)
	assert.True(t, reused.Created)
	assert.Empty(t, reused.Base)

	blocked := filepath.Join(filepath.Dir(repoDir), filepath.Base(repoDir)+"--blocked")
	require.NoError(t, os.Mkdir(blocked, 0o755))
	failed, err := client.Add("blocked", AddOptions{})
	require.Error(t, err)
	assert.False(t, failed.Created)
	assert.Equal(t, blocked, failed.Path)
	assert.Equal(t, "blocked", failed.Branch)
	assert.Equal(t, err, failed.Err)
}

func TestDupPartialFailureKeepsCreatedItems(t *testing.T) {
	repoDir := initTestRepo(t)
	t.Chdir(repoDir)
	blocked := filepath.Join(filepath.Dir(repoDir), ".dup-2")
	require.NoError(t, os.Mkdir(blocked, 0o755))

	result, err := NewClient(Options{}).Dup(DupOptions{BaseBranch: "main", Count: 3})
	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Items, 2)

	first := result.Items[0]
	assert.Equal(t, ".dup-1", first.Name)
	assert.True(t, first.Created)
	assert.NoError(t, first.Err)
	assert.Equal(t, "main", first.Base)
	assert.DirExists(t, first.Path)

	second := result.Items[1]
	assert.Equal(t, ".dup-2", second.Name)
	assert.False(t, second.Created)
	assert.Equal(t, err, second.Err)
	assert.Contains(t, second.Err.Error(), "directory already exists")
	assert.NoDirExists(t, filepath.Join(filepath.Dir(repoDir), ".dup-3"))
}

func TestMeasureErrorCode(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{&fs.PathError{Op: "lstat", Path: "/x", Err: fs.ErrNotExist}, MeasureNotFound},
		{fmt.Errorf("walk: %w", &fs.PathError{Op: "open", Path: "/x", Err: fs.ErrPermission}), MeasurePermissionDenied},
		{fmt.Errorf("/x: %w", errAllocatedOverflow), MeasureOverflow},
		{errors.New("missing platform file metadata"), MeasureScanFailed},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, MeasureErrorCode(tt.err), tt.err.Error())
	}

	_, err := MeasureAllocated(filepath.Join(t.TempDir(), "missing"))
	assert.Equal(t, MeasureNotFound, MeasureErrorCode(err))
}
