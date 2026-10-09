//go:build unix

package worktree

import (
	"errors"
	"io/fs"
	"math"
	"syscall"
)

func statAllocated(_ string, info fs.FileInfo) (allocatedEntry, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return allocatedEntry{}, errors.New("missing platform file metadata")
	}
	if stat.Blocks < 0 || uint64(stat.Blocks) > math.MaxUint64/512 {
		return allocatedEntry{}, errAllocatedOverflow
	}
	return allocatedEntry{
		bytes:     uint64(stat.Blocks) * 512,
		id:        fileID{volume: deviceID(stat.Dev), index: [2]uint64{0, stat.Ino}},
		hardlinks: stat.Nlink > 1 && !info.IsDir(),
	}, nil
}

func deviceID[T int32 | uint32 | uint64](dev T) uint64 {
	return uint64(dev)
}
