package task

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	slugSanitizer = regexp.MustCompile(`[^a-z0-9]+`)
	pullNumberRE  = regexp.MustCompile(`(?i)/pull/(\d+)`)
)

const maxTitleLen = 72

// NewTaskID returns a short stable id: t-YYYYMMDD-xxxx (4 random hex digits).
func NewTaskID(now time.Time) string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("t-%s-%s", now.UTC().Format("20060102"), hex.EncodeToString(b[:]))
}

// DeriveTitle builds a human-readable title from the create input.
func DeriveTitle(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "Untitled task"
	}
	if pr := ParsePullNumber(source); pr != "" {
		repo := parseRepoSlug(source)
		if repo != "" {
			return fmt.Sprintf("Review PR #%s (%s)", pr, repo)
		}
		return fmt.Sprintf("Review PR #%s", pr)
	}
	if issue := ParseIssueNumber(source); issue != "" {
		return fmt.Sprintf("Issue #%s", issue)
	}
	if len(source) <= maxTitleLen {
		return source
	}
	return source[:maxTitleLen-3] + "..."
}

// ParsePullNumber extracts a GitHub pull request number from a URL or text.
func ParsePullNumber(source string) string {
	m := pullNumberRE.FindStringSubmatch(source)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ParseIssueNumber extracts a GitHub issue number (not from /pull/ URLs).
func ParseIssueNumber(source string) string {
	if ParsePullNumber(source) != "" {
		return ""
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	if strings.HasPrefix(source, "#") {
		if n, err := strconv.Atoi(strings.TrimPrefix(source, "#")); err == nil && n > 0 {
			return strconv.Itoa(n)
		}
	}
	lower := strings.ToLower(source)
	if idx := strings.Index(lower, "/issues/"); idx >= 0 {
		rest := source[idx+len("/issues/"):]
		if n, _, ok := parseLeadingInt(rest); ok {
			return strconv.Itoa(n)
		}
	}
	fields := strings.FieldsFunc(source, func(r rune) bool {
		return r == '#' || r == ' ' || r == ','
	})
	for i := len(fields) - 1; i >= 0; i-- {
		if n, err := strconv.Atoi(fields[i]); err == nil && n > 0 {
			return strconv.Itoa(n)
		}
	}
	if n, err := strconv.Atoi(source); err == nil && n > 0 {
		return strconv.Itoa(n)
	}
	return ""
}

func parseRepoSlug(source string) string {
	lower := strings.ToLower(source)
	idx := strings.Index(lower, "github.com/")
	if idx < 0 {
		return ""
	}
	rest := source[idx+len("github.com/"):]
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func parseLeadingInt(s string) (int, int, bool) {
	n := 0
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if i == 0 {
		return 0, 0, false
	}
	return n, i, true
}

// DisplayTitle returns the title for UI, deriving from source for older tasks.
func DisplayTitle(rec Record) string {
	if rec.Title != "" {
		return rec.Title
	}
	return DeriveTitle(rec.Source)
}

// NewAttemptID returns the next attempt id for a task (attempt-1, attempt-2, ...).
func NewAttemptID(existing int) string {
	return fmt.Sprintf("attempt-%d", existing+1)
}

// NewRunID returns a unique run id within a task directory.
func NewRunID(now time.Time, seq int) string {
	return fmt.Sprintf("run-%s-%d", now.UTC().Format("150405"), seq)
}

// WorktreeDirName is the on-disk worktree directory name (may start with '.').
func WorktreeDirName(taskID, attemptID string) string {
	base := slugSanitizer.ReplaceAllString(strings.ToLower(taskID), "-")
	base = strings.Trim(base, "-")
	if len(base) > 24 {
		base = base[:24]
	}
	shortAttempt := strings.TrimPrefix(attemptID, "attempt-")
	return fmt.Sprintf(".task-%s-%s", base, shortAttempt)
}

// WorktreeBranchName is a valid git branch for the attempt worktree.
func WorktreeBranchName(taskID, attemptID string) string {
	base := slugSanitizer.ReplaceAllString(strings.ToLower(taskID), "-")
	base = strings.Trim(base, "-")
	if len(base) > 40 {
		base = base[:40]
	}
	shortAttempt := strings.TrimPrefix(attemptID, "attempt-")
	return fmt.Sprintf("_task/%s/%s", base, shortAttempt)
}

// TmuxSessionName builds a tmux session name for a task attempt (coding / running).
func TmuxSessionName(taskID, attemptID string) string {
	return tmuxSessionNameWithSuffix(taskID, attemptID, "")
}

// TmuxStageSessionName builds a separate tmux session for a stage (review, verify, …).
func TmuxStageSessionName(taskID, attemptID, stage string) string {
	suffix := "-" + strings.TrimSpace(stage)
	if stage == "" {
		suffix = ""
	}
	return tmuxSessionNameWithSuffix(taskID, attemptID, suffix)
}

func tmuxSessionNameWithSuffix(taskID, attemptID, suffix string) string {
	name := slugSanitizer.ReplaceAllString(strings.ToLower(taskID+"-"+attemptID), "-")
	name = strings.Trim(name, "-")
	max := 48 - len(suffix)
	if max < 8 {
		max = 8
	}
	if len(name) > max {
		name = name[len(name)-max:]
	}
	return "gmc-" + name + suffix
}
