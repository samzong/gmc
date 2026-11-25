package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
)

type SemVer struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
}

var semverPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)` +
	`(?:-((?:alpha|beta|rc)\.(?:0|[1-9][0-9]*)))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

func ParseSemVer(tag string) (SemVer, error) {
	parts := semverPattern.FindStringSubmatch(strings.TrimSpace(tag))
	if parts == nil {
		return SemVer{}, fmt.Errorf("invalid semantic version: %s", tag)
	}
	var values [3]int
	for i := range values {
		value, err := strconv.Atoi(parts[i+1])
		if err != nil {
			return SemVer{}, fmt.Errorf("invalid semantic version: %s: %w", tag, err)
		}
		values[i] = value
	}
	return SemVer{Major: values[0], Minor: values[1], Patch: values[2], Prerelease: parts[4], Build: parts[5]}, nil
}

func (v SemVer) String() string {
	value := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		value += "-" + v.Prerelease
	}
	if v.Build != "" {
		value += "+" + v.Build
	}
	return value
}

func (v SemVer) Release() SemVer {
	v.Prerelease = ""
	v.Build = ""
	return v
}

func (v SemVer) Equal(other SemVer) bool {
	return !v.LessThan(other) && !other.LessThan(v)
}

func (v SemVer) LessThan(other SemVer) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	if v.Patch != other.Patch {
		return v.Patch < other.Patch
	}
	if v.Prerelease == other.Prerelease || other.Prerelease == "" {
		return v.Prerelease != other.Prerelease
	}
	if v.Prerelease == "" {
		return false
	}
	stage, number, _ := strings.Cut(v.Prerelease, ".")
	otherStage, otherNumber, _ := strings.Cut(other.Prerelease, ".")
	if stage != otherStage {
		return stage < otherStage
	}
	if len(number) != len(otherNumber) {
		return len(number) < len(otherNumber)
	}
	return number < otherNumber
}

type RuleStats struct {
	Breaking []string
	Features []string
	Patches  []string
	Others   []string
}

type RuleResult struct {
	NextVersion SemVer
	Reason      string
}

var commitTypePattern = regexp.MustCompile(`^(?P<type>[a-z]+)(?:\([^)]+\))?(?P<breaking>!)?:`)

func SuggestWithRules(base SemVer, commits []gitutil.CommitInfo) RuleResult {
	stats := RuleStats{}

	for _, commit := range commits {
		message := strings.TrimSpace(commit.Message)
		if message == "" {
			continue
		}

		commitType, breaking := parseCommitType(message)
		if commitType == "" {
			commitType = inferCommitType(message)
		}

		if !breaking && containsBreakingChange(message, commit.Body) {
			breaking = true
		}

		switch {
		case breaking:
			stats.Breaking = append(stats.Breaking, message)
		case commitType == "feat":
			stats.Features = append(stats.Features, message)
		case isPatchType(commitType):
			stats.Patches = append(stats.Patches, message)
		default:
			stats.Others = append(stats.Others, message)
		}
	}

	result := RuleResult{
		NextVersion: base,
		Reason:      "Only documentation, style, test, or chore changes detected",
	}

	for _, change := range []struct {
		messages []string
		next     SemVer
		label    string
	}{
		{stats.Breaking, SemVer{Major: base.Major + 1}, "breaking change"},
		{stats.Features, SemVer{Major: base.Major, Minor: base.Minor + 1}, "feature"},
		{stats.Patches, SemVer{Major: base.Major, Minor: base.Minor, Patch: base.Patch + 1}, "fix/refactor"},
	} {
		if len(change.messages) > 0 {
			result.NextVersion = change.next
			result.Reason = fmt.Sprintf("Detected %d %s commit(s) since %s, e.g. %s",
				len(change.messages), change.label, base.String(), describeMessages(change.messages))
			return result
		}
	}
	if len(stats.Others) == 0 {
		result.Reason = "No commits found since last release"
	}

	return result
}

func parseCommitType(message string) (string, bool) {
	matches := commitTypePattern.FindStringSubmatch(message)
	if len(matches) == 0 {
		return "", false
	}

	commitType := strings.ToLower(matches[1])
	breaking := matches[2] == "!"
	return commitType, breaking
}

func inferCommitType(message string) string {
	lower := strings.ToLower(message)

	switch {
	case strings.Contains(lower, "break"):
		return "feat"
	case strings.Contains(lower, "feat"), strings.Contains(lower, "feature"), strings.Contains(lower, "add"):
		return "feat"
	case strings.Contains(lower, "fix"), strings.Contains(lower, "bug"), strings.Contains(lower, "patch"):
		return "fix"
	case strings.Contains(lower, "refactor"):
		return "refactor"
	case strings.Contains(lower, "perf"), strings.Contains(lower, "optimiz"):
		return "perf"
	case strings.Contains(lower, "test"):
		return "test"
	case strings.Contains(lower, "doc"):
		return "docs"
	case strings.Contains(lower, "build"), strings.Contains(lower, "ci"):
		return "build"
	default:
		return "chore"
	}
}

func isPatchType(commitType string) bool {
	switch commitType {
	case "fix", "perf", "refactor", "build", "ci", "revert", "hotfix":
		return true
	default:
		return false
	}
}

func containsBreakingChange(message, body string) bool {
	lowerMessage := strings.ToLower(message)
	lowerBody := strings.ToLower(body)
	return strings.Contains(lowerMessage, "breaking change") || strings.Contains(lowerBody, "breaking change")
}

func describeMessages(messages []string) string {
	const maxExamples = 2
	examples := messages[:min(len(messages), maxExamples)]

	quoted := make([]string, 0, len(examples))
	for _, msg := range examples {
		quoted = append(quoted, fmt.Sprintf("%q", msg))
	}

	if len(messages) > maxExamples {
		return fmt.Sprintf("%s, and %d more", strings.Join(quoted, ", "), len(messages)-maxExamples)
	}

	return strings.Join(quoted, ", ")
}
