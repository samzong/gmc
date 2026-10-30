package worktree

import (
	"strings"
	"sync"
)

type ListDetail struct {
	Status   string
	DiffStat *DiffStat
}

func (c *Client) ListDetails(worktrees []Info, diffBase string) ([]ListDetail, error) {
	details := make([]ListDetail, len(worktrees))
	errs := make([]error, len(worktrees))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(8, len(worktrees)) {
		workers.Go(func() {
			for i := range jobs {
				wt := worktrees[i]
				details[i].Status = wt.Status
				if wt.Status == "" {
					details[i].Status = c.GetWorktreeStatus(wt.Path)
				}
				base, err := c.ResolveDiffBaseForWorktree(wt.Path, diffBase)
				if err == nil {
					var stat DiffStat
					stat, err = c.WorktreeDiffStat(wt.Path, base)
					if err == nil {
						details[i].DiffStat = &stat
					}
				}
				if strings.TrimSpace(diffBase) != "" {
					errs[i] = err
				}
			}
		})
	}
	for i := range worktrees {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return details, nil
}
