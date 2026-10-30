package worktree

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

type bulkRoot struct {
	bytes uint64
	seen  map[fileID]struct{}
	err   error
}

type bulkParent struct {
	fd    int
	refs  atomic.Int64
	slots chan struct{}
}

func (p *bulkParent) release() {
	if p != nil && p.refs.Add(-1) == 0 {
		_ = unix.Close(p.fd)
		<-p.slots
	}
}

type bulkJob struct {
	path   string
	name   string
	parent *bulkParent
	root   int
	top    bool
}

type bulkScan struct {
	mu      sync.Mutex
	ready   *sync.Cond
	jobs    []bulkJob
	pending int
	roots   []bulkRoot
	slots   chan struct{}
}

func measureAllocated(root string) (uint64, error) {
	result := measureAllocatedPaths([]string{root})[0]
	return result.Bytes, result.Err
}

func measureAllocatedPaths(paths []string) []AllocatedSize {
	results := make([]AllocatedSize, len(paths))
	if len(paths) == 0 {
		return results
	}
	s := &bulkScan{roots: make([]bulkRoot, len(paths)), slots: make(chan struct{}, 32)}
	s.ready = sync.NewCond(&s.mu)
	for i, path := range paths {
		results[i].Path = path
		clean := filepath.Clean(path)
		info, err := os.Lstat(clean)
		if err != nil {
			s.roots[i].err = err
			continue
		}
		entry, err := statAllocated(clean, info)
		if err != nil {
			s.roots[i].err = err
			continue
		}
		s.roots[i].bytes = entry.bytes
		s.roots[i].seen = make(map[fileID]struct{})
		if info.IsDir() {
			s.enqueue(bulkJob{path: clean, root: i, top: true})
		}
	}
	var workers sync.WaitGroup
	for range diskUsageWorkers {
		workers.Go(s.worker)
	}
	workers.Wait()
	for i := range results {
		if s.roots[i].err != nil {
			results[i].Bytes, results[i].Err = measureAllocatedWalk(paths[i])
		} else {
			results[i].Bytes = s.roots[i].bytes
		}
	}
	return results
}

func (s *bulkScan) enqueue(job bulkJob) {
	if job.parent != nil {
		job.parent.refs.Add(1)
	}
	s.mu.Lock()
	s.jobs = append(s.jobs, job)
	s.pending++
	s.ready.Signal()
	s.mu.Unlock()
}

func (s *bulkScan) worker() {
	buffer := make([]byte, 16*1024)
	for {
		s.mu.Lock()
		for len(s.jobs) == 0 && s.pending != 0 {
			s.ready.Wait()
		}
		if s.pending == 0 {
			s.mu.Unlock()
			return
		}
		i := len(s.jobs) - 1
		job := s.jobs[i]
		s.jobs[i] = bulkJob{}
		s.jobs = s.jobs[:i]
		err := s.roots[job.root].err
		s.mu.Unlock()
		if err == nil {
			err = s.scanDirectory(job, buffer)
		} else {
			job.parent.release()
		}
		s.mu.Lock()
		if err != nil {
			s.roots[job.root].err = err
		}
		s.pending--
		if s.pending == 0 {
			s.ready.Broadcast()
		}
		s.mu.Unlock()
	}
}

func (s *bulkScan) add(root int, entry allocatedEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &s.roots[root]
	if entry.hardlinks {
		if _, ok := r.seen[entry.id]; ok {
			return nil
		}
		r.seen[entry.id] = struct{}{}
	}
	if entry.bytes > ^uint64(0)-r.bytes {
		return errAllocatedOverflow
	}
	r.bytes += entry.bytes
	return nil
}

func (s *bulkScan) scanDirectory(job bulkJob, buffer []byte) error {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	var fd int
	var err error
	if job.parent != nil {
		fd, err = unix.Openat(job.parent.fd, job.name, flags, 0)
		job.parent.release()
	} else {
		fd, err = unix.Open(job.path, flags, 0)
	}
	if err != nil {
		return err
	}
	var parent *bulkParent
	select {
	case s.slots <- struct{}{}:
		parent = &bulkParent{fd: fd, slots: s.slots}
		parent.refs.Store(1)
		defer parent.release()
	default:
		defer func() { _ = unix.Close(fd) }()
	}
	var total uint64
	for {
		count, err := readBulkAttributes(fd, buffer)
		if err != nil {
			return err
		}
		if count == 0 {
			return s.add(job.root, allocatedEntry{bytes: total})
		}
		remaining := buffer
		for range count {
			entry, rest, err := parseBulkEntry(remaining)
			if err != nil {
				return err
			}
			remaining = rest
			if job.top && (entry.name == ".git" || entry.name == ".bare") {
				continue
			}
			if entry.err != nil {
				return entry.err
			}
			var path string
			if entry.directory || entry.fallback {
				path = filepath.Join(job.path, entry.name)
			}
			if entry.fallback {
				info, err := os.Lstat(path)
				if err != nil {
					return err
				}
				entry.allocated, err = statAllocated(path, info)
				if err != nil {
					return err
				}
				entry.directory = info.IsDir()
			}
			if entry.allocated.hardlinks {
				if err := s.add(job.root, entry.allocated); err != nil {
					return err
				}
			} else {
				if entry.allocated.bytes > ^uint64(0)-total {
					return errAllocatedOverflow
				}
				total += entry.allocated.bytes
			}
			if entry.directory {
				s.enqueue(bulkJob{path: path, name: entry.name, parent: parent, root: job.root})
			}
		}
	}
}
