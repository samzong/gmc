package task

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var slugSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func NewTaskID(now time.Time) string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "t-" + now.UTC().Format("20060102-150405")
	}
	return "t-" + now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

func NewAttemptID() string {
	return "attempt-1"
}

func DisplayTitle(rec Record) string {
	if strings.TrimSpace(rec.Title) != "" {
		return rec.Title
	}
	return DeriveTitle(rec.Source, rec.SourceFile, rec.Issue)
}

func DeriveTitle(source, sourceFile, issue string) string {
	if issue != "" {
		return "Issue #" + issue
	}
	if sourceFile != "" {
		return filepath.Base(sourceFile)
	}
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 80 {
			return strings.TrimSpace(line[:80])
		}
		return line
	}
	return "Untitled task"
}

func ParseIssueNumber(source string) string {
	s := strings.TrimSpace(source)
	s = strings.TrimPrefix(s, "#")
	if matched, _ := regexp.MatchString(`^\d+$`, s); matched {
		return s
	}
	if i := strings.LastIndex(s, "/issues/"); i >= 0 {
		rest := s[i+len("/issues/"):]
		for j, r := range rest {
			if r < '0' || r > '9' {
				return rest[:j]
			}
		}
		return rest
	}
	return ""
}

func WorktreeDirName(taskID, attemptID string) string {
	base := slugSanitizer.ReplaceAllString(strings.ToLower(taskID), "-")
	base = strings.Trim(base, "-")
	if len(base) > 24 {
		base = base[:24]
	}
	shortAttempt := strings.TrimPrefix(attemptID, "attempt-")
	return fmt.Sprintf("task-%s-%s", base, shortAttempt)
}

func WorktreeBranchName(taskID, attemptID string) string {
	base := slugSanitizer.ReplaceAllString(strings.ToLower(taskID), "-")
	base = strings.Trim(base, "-")
	if len(base) > 40 {
		base = base[:40]
	}
	shortAttempt := strings.TrimPrefix(attemptID, "attempt-")
	return fmt.Sprintf("_task/%s/%s", base, shortAttempt)
}

func TmuxSessionName(taskID, attemptID string, suffixes ...string) string {
	name := slugSanitizer.ReplaceAllString(strings.ToLower(taskID+"-"+attemptID), "-")
	name = strings.Trim(name, "-")
	if len(suffixes) == 0 {
		if len(name) > 48 {
			name = name[:48]
		}
		return "gmc-" + name
	}
	suffix := slugSanitizer.ReplaceAllString(strings.ToLower(strings.Join(suffixes, "-")), "-")
	suffix = strings.Trim(suffix, "-")
	if len(suffix) > 24 {
		suffix = suffix[:24]
	}
	if len(name) > 40 {
		name = name[:40]
	}
	if suffix == "" {
		return "gmc-" + name
	}
	return "gmc-" + name + "-" + suffix
}
