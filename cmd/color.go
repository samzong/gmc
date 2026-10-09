package cmd

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/samzong/gmc/internal/worktree"
)

const (
	sgrBold    = "1"
	sgrDim     = "2"
	sgrRed     = "31"
	sgrGreen   = "32"
	sgrMagenta = "35"
	sgrCyan    = "36"
)

var diffStatPattern = regexp.MustCompile(`^(\d+ files? \()(\+\d+) (-\d+)\)$`)

func colorEnabled(w io.Writer) bool {
	return colorAllowed(terminalLinksEnabled(w))
}

func colorAllowed(terminal bool) bool {
	return terminal && os.Getenv("NO_COLOR") == ""
}

func paint(text, code string, enabled bool) string {
	if !enabled || code == "" || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func colorStatus(status string, enabled bool) string {
	if !enabled {
		return status
	}
	parts := strings.Split(status, ", ")
	for i, part := range parts {
		parts[i] = colorStatusPart(part)
	}
	return strings.Join(parts, ", ")
}

func colorStatusPart(part string) string {
	switch part {
	case "clean":
		return paint(part, sgrGreen, true)
	case "agent":
		return paint(part, sgrCyan, true)
	case "bare":
		return paint(part, sgrDim, true)
	case "locked":
		return paint(part, sgrMagenta, true)
	case "prunable":
		return paint(part, sgrRed, true)
	}
	if match := diffStatPattern.FindStringSubmatch(part); match != nil {
		return match[1] + paint(match[2], sgrGreen, true) + " " + paint(match[3], sgrRed, true) + ")"
	}
	return part
}

func colorReview(reviews map[string]worktree.ReviewInfo, branch, display string, enabled bool) string {
	review, ok := reviews[branch]
	if !ok {
		return paint(display, sgrDim, enabled)
	}
	switch strings.ToLower(review.State) {
	case "open":
		return paint(display, sgrGreen, enabled)
	case "merged":
		return paint(display, sgrMagenta, enabled)
	case "closed":
		return paint(display, sgrRed, enabled)
	}
	return display
}

func colorSize(text string, enabled bool) string {
	if text == "-" {
		return paint(text, sgrDim, enabled)
	}
	return text
}

func currentWorktreePath(worktrees []worktree.Info) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	current := ""
	for _, wt := range worktrees {
		rel, err := filepath.Rel(wt.Path, cwd)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(wt.Path) > len(current) {
			current = wt.Path
		}
	}
	return current
}
