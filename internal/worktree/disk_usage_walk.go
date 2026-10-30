//go:build !darwin

package worktree

import "sync"

func measureAllocated(root string) (uint64, error) {
	return measureAllocatedWalk(root)
}

func measureAllocatedPaths(paths []string) []AllocatedSize {
	results := make([]AllocatedSize, len(paths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(diskUsageWorkers, len(paths)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				bytes, err := measureAllocatedWalk(paths[i])
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
