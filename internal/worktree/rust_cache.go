package worktree

import (
	"fmt"
	"sync"

	"github.com/samzong/gmc/internal/rustcache"
)

var ensureRustCache = sync.OnceValues(rustcache.Ensure)

func (c *Client) prepareRustCache(root string, report *Report) {
	eligible, reason := rustcache.Eligible(root)
	if !eligible {
		if c.verbose && reason != "" {
			report.Info("Rust cache skipped: " + reason)
		}
		return
	}
	helper, err := ensureRustCache()
	if err == nil {
		err = rustcache.Enable(root, helper)
	}
	if c.verbose && err != nil {
		report.Warn(fmt.Sprintf("Rust cache skipped: %v", err))
	}
}
