package worktree

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"strconv"
	"sync"
)

const diskUsageWorkers = 2

var errAllocatedOverflow = errors.New("allocated size overflows uint64")

type fileID struct {
	volume uint64
	index  [2]uint64
}

type allocatedEntry struct {
	bytes     uint64
	id        fileID
	hardlinks bool
}

type AllocatedSize struct {
	Path  string
	Bytes uint64
	Err   error
}

func MeasureAllocated(root string) (uint64, error) {
	root = filepath.Clean(root)
	var total uint64
	seen := make(map[fileID]struct{})
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filepath.Dir(path) == root && path != root && (entry.Name() == ".git" || entry.Name() == ".bare") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		allocated, err := statAllocated(path, info)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if allocated.hardlinks {
			if _, ok := seen[allocated.id]; ok {
				return nil
			}
			seen[allocated.id] = struct{}{}
		}
		if allocated.bytes > math.MaxUint64-total {
			return fmt.Errorf("%s: %w", root, errAllocatedOverflow)
		}
		total += allocated.bytes
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func MeasureAllocatedAll(paths []string) []AllocatedSize {
	results := make([]AllocatedSize, len(paths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(diskUsageWorkers, len(paths)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				bytes, err := MeasureAllocated(paths[i])
				results[i] = AllocatedSize{Path: paths[i], Bytes: bytes, Err: err}
			}
		}()
	}
	for i := range paths {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func FormatAllocatedSize(n uint64) string {
	const units = "KMGTP"
	if n < 1024 {
		return strconv.FormatUint(n, 10) + "B"
	}
	value := float64(n) / 1024
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if value < 10 {
		rounded := math.Round(value*10) / 10
		if rounded < 10 {
			return strconv.FormatFloat(rounded, 'f', 1, 64) + units[unit:unit+1]
		}
		value = rounded
	}
	rounded := math.Round(value)
	if rounded >= 1024 && unit < len(units)-1 {
		return strconv.FormatFloat(rounded/1024, 'f', 1, 64) + units[unit+1:unit+2]
	}
	return strconv.FormatFloat(rounded, 'f', 0, 64) + units[unit:unit+1]
}
