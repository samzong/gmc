package worktree

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/samzong/gmc/internal/rustcache"
)

var ensureRustCache = sync.OnceValues(rustcache.Ensure)

func (c *Client) prepareRustCache(root string, report *Report) {
	_, projects, err := scanShareProjects(root, nil)
	if err != nil {
		if c.verbose {
			report.Warn(fmt.Sprintf("Rust cache discovery skipped: %v", err))
		}
		return
	}
	sort.Slice(projects, func(i, j int) bool { return len(projects[i].Path) < len(projects[j].Path) })
	var roots []string
	for _, project := range projects {
		if project.Ecosystem != "Rust" || filepath.Base(project.Path) != "Cargo.toml" {
			continue
		}
		covered := false
		for _, parent := range roots {
			if parent == "." || strings.HasPrefix(project.Project, parent+"/") {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		roots = append(roots, project.Project)
		path := filepath.Join(root, filepath.FromSlash(project.Project))
		eligible, reason := rustcache.Eligible(path)
		if !eligible {
			if c.verbose && reason != "" {
				report.Info(fmt.Sprintf("Rust cache skipped (%s): %s", project.Project, reason))
			}
			continue
		}
		helper, err := ensureRustCache()
		if err == nil {
			err = rustcache.Enable(path, helper)
		}
		if c.verbose && err != nil {
			report.Warn(fmt.Sprintf("Rust cache skipped (%s): %v", project.Project, err))
		}
	}
}
