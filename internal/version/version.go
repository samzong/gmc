package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/samzong/gmc/internal/git"
)

type BumpType string

const (
	BumpNone  BumpType = "none"
	BumpPatch BumpType = "patch"
	BumpMinor BumpType = "minor"
	BumpMajor BumpType = "major"
)

type SemVer struct {
	Major int
	Minor int
	Patch int
}

func ParseSemVer(tag string) (SemVer, error) {
	if strings.TrimSpace(tag) == "" {
		return SemVer{}, nil
	}

	trimmed := strings.TrimSpace(tag)
	trimmed = strings.TrimPrefix(trimmed, "v")

	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return SemVer{}, fmt.Errorf("invalid semantic version: %s", tag)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid major version in %s: %w", tag, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid minor version in %s: %w", tag, err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return SemVer{}, fmt.Errorf("invalid patch version in %s: %w", tag, err)
	}

	if major < 0 || minor < 0 || patch < 0 {
		return SemVer{}, fmt.Errorf("semantic version components must be non-negative: %s", tag)
	}

	return SemVer{Major: major, Minor: minor, Patch: patch}, nil
}

func (v SemVer) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v SemVer) Equal(other SemVer) bool {
	return v.Major == other.Major && v.Minor == other.Minor && v.Patch == other.Patch
}

func (v SemVer) LessThan(other SemVer) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

func (v SemVer) GreaterThan(other SemVer) bool {
	return other.LessThan(v)
}

func (v SemVer) NextMajor() SemVer {
	return SemVer{Major: v.Major + 1, Minor: 0, Patch: 0}
}

func (v SemVer) NextMinor() SemVer {
	return SemVer{Major: v.Major, Minor: v.Minor + 1, Patch: 0}
}

func (v SemVer) NextPatch() SemVer {
	return SemVer{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
}

type RuleStats struct {
	Breaking []string
	Features []string
	Patches  []string
	Others   []string
}

type RuleResult struct {
	BaseVersion SemVer
	NextVersion SemVer
	BumpType    BumpType
	Reason      string
	Stats       RuleStats
}

var commitTypePattern = regexp.MustCompile(`^(?P<type>[a-z]+)(?:\([^)]+\))?(?P<breaking>!)?:`)

func SuggestWithRules(base SemVer, commits []git.CommitInfo) RuleResult {
	stats := RuleStats{}

	if len(commits) == 0 {
		return RuleResult{
			BaseVersion: base,
			NextVersion: base,
			BumpType:    BumpNone,
			Reason:      "No commits found since last release",
			Stats:       stats,
		}
	}

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
		BaseVersion: base,
		NextVersion: base,
		BumpType:    BumpNone,
		Stats:       stats,
		Reason:      "Only documentation, style, test, or chore changes detected",
	}

	switch {
	case len(stats.Breaking) > 0:
		result.BumpType = BumpMajor
		result.NextVersion = base.NextMajor()
		result.Reason = fmt.Sprintf(
			"Detected %d breaking change commit(s) since %s, e.g. %s",
			len(stats.Breaking),
			base.String(),
			describeMessages(stats.Breaking),
		)
	case len(stats.Features) > 0:
		result.BumpType = BumpMinor
		result.NextVersion = base.NextMinor()
		result.Reason = fmt.Sprintf(
			"Detected %d feature commit(s) since %s, e.g. %s",
			len(stats.Features),
			base.String(),
			describeMessages(stats.Features),
		)
	case len(stats.Patches) > 0:
		result.BumpType = BumpPatch
		result.NextVersion = base.NextPatch()
		result.Reason = fmt.Sprintf(
			"Detected %d fix/refactor commit(s) since %s, e.g. %s",
			len(stats.Patches),
			base.String(),
			describeMessages(stats.Patches),
		)
	default:
		if len(stats.Others) == 0 {
			result.Reason = "No commits found since last release"
		}
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
	if len(messages) == 0 {
		return ""
	}

	maxExamples := 2
	examples := messages
	if len(messages) > maxExamples {
		examples = messages[:maxExamples]
	}

	quoted := make([]string, 0, len(examples))
	for _, msg := range examples {
		trimmed := strings.TrimSpace(msg)
		if trimmed == "" {
			continue
		}
		quoted = append(quoted, fmt.Sprintf("%q", trimmed))
	}

	if len(quoted) == 0 {
		return "recent commits"
	}

	if len(messages) > maxExamples {
		return fmt.Sprintf("%s, and %d more", strings.Join(quoted, ", "), len(messages)-maxExamples)
	}

	return strings.Join(quoted, ", ")
}
