package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskID(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Now().UTC()
	for _, id := range []string{"t-20260603-aaaa", "t-20260603-bbbb"} {
		require.NoError(t, store.CreateTask(Record{
			ID: id, State: TaskIntake, Source: "x", CreatedAt: now, UpdatedAt: now,
		}))
	}
	got, err := store.ResolveTaskID("t-20260603-aaaa")
	require.NoError(t, err)
	assert.Equal(t, "t-20260603-aaaa", got)

	got, err = store.ResolveTaskID("t-20260603-a")
	require.NoError(t, err)
	assert.Equal(t, "t-20260603-aaaa", got)

	_, err = store.ResolveTaskID("t-20260603")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}
