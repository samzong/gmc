package worktree

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type DiffStat struct {
	Base       string
	Files      int
	Insertions int
	Deletions  int
}

func (s DiffStat) HasChanges() bool {
	return s.Files > 0
}

func (c *Client) ResolveDiffBase(override string) (string, error) {
	if err := c.ensureInit(); err != nil {
		return "", fmt.Errorf("failed to find worktree root: %w", err)
	}
	return c.resolveSyncBaseBranch(c.repoDir, override)
}

func (c *Client) WorktreeDiffStat(path, base string) (DiffStat, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return DiffStat{}, errors.New("diff base cannot be empty")
	}

	result, err := c.runner.Run("-C", path, "merge-base", base, "HEAD")
	if err != nil {
		return DiffStat{}, fmt.Errorf("failed to find merge base with %s: %w", base, err)
	}
	mergeBase := result.StdoutString(true)
	if mergeBase == "" {
		return DiffStat{}, fmt.Errorf("failed to find merge base with %s", base)
	}

	result, err = c.runner.Run("-C", path, "diff", "--numstat", "-z", mergeBase)
	if err != nil {
		return DiffStat{}, fmt.Errorf("failed to collect diff stat against %s: %w", mergeBase, err)
	}

	stat := parseDiffNumstat(result.Stdout)
	stat.Base = base
	return stat, nil
}

func parseDiffNumstat(output []byte) DiffStat {
	var stat DiffStat
	if len(output) == 0 {
		return stat
	}

	fields := strings.Split(string(output), "\x00")
	for i := 0; i < len(fields); {
		field := fields[i]
		if field == "" {
			i++
			continue
		}

		parts := strings.SplitN(field, "\t", 3)
		if len(parts) < 3 {
			i++
			continue
		}

		stat.Files++
		stat.Insertions += parseNumstatCount(parts[0])
		stat.Deletions += parseNumstatCount(parts[1])

		// With -z, renamed/copied paths are emitted as:
		// "<adds>\t<dels>\t\0<old>\0<new>\0".
		if parts[2] == "" {
			i += 3
			continue
		}
		i++
	}

	return stat
}

func parseNumstatCount(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}
