package analyzer

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/samzong/gmc/internal/git"
)

// QualityEvaluatorImpl implements the QualityEvaluator interface
type QualityEvaluatorImpl struct{}

// NewQualityEvaluator creates a new quality evaluator
func NewQualityEvaluator() QualityEvaluator {
	return &QualityEvaluatorImpl{}
}

// EvaluateCommit evaluates the quality of a single commit
func (q *QualityEvaluatorImpl) EvaluateCommit(commit git.CommitInfo) QualityMetrics {
	conventionalScore := q.evaluateConventionalCommits(commit.Message)
	lengthScore := q.evaluateMessageLength(commit.Message)
	clarityScore := q.evaluateMessageClarity(commit.Message)

	overallScore := conventionalScore + lengthScore + clarityScore

	return QualityMetrics{
		ConventionalScore: conventionalScore,
		LengthScore:       lengthScore,
		ClarityScore:      clarityScore,
		OverallScore:      overallScore,
	}
}

// EvaluateBatch evaluates multiple commits
func (q *QualityEvaluatorImpl) EvaluateBatch(commits []git.CommitInfo) []QualityMetrics {
	metrics := make([]QualityMetrics, len(commits))
	for i, commit := range commits {
		metrics[i] = q.EvaluateCommit(commit)
	}
	return metrics
}

// evaluateConventionalCommits checks if the commit follows Conventional Commits format (0-40 points)
func (q *QualityEvaluatorImpl) evaluateConventionalCommits(message string) float64 {
	// Conventional Commits pattern: type(scope): description
	conventionalPattern := regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|chore|build|ci)(\([^)]+\))?: .+`)

	if conventionalPattern.MatchString(message) {
		return 40.0
	}

	// Partial credit for having a type prefix
	typePattern := regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|chore|build|ci):`)
	if typePattern.MatchString(message) {
		return 25.0
	}

	// Check if it starts with a verb (some credit for good structure)
	words := strings.Fields(message)
	if len(words) > 0 && q.isVerb(words[0]) {
		return 15.0
	}

	return 0.0
}

// evaluateMessageLength evaluates the message length (0-30 points)
func (q *QualityEvaluatorImpl) evaluateMessageLength(message string) float64 {
	// Get the first line (subject line)
	lines := strings.Split(message, "\n")
	subject := strings.TrimSpace(lines[0])
	length := len(subject)

	// Optimal length: 50-72 characters
	if length >= 50 && length <= 72 {
		return 30.0
	}

	// Good length: 30-50 or 72-100 characters
	if (length >= 30 && length < 50) || (length > 72 && length <= 100) {
		return 20.0
	}

	// Acceptable length: 20-30 or 100-120 characters
	if (length >= 20 && length < 30) || (length > 100 && length <= 120) {
		return 10.0
	}

	// Too short or too long
	return 0.0
}

// evaluateMessageClarity evaluates the clarity and quality of the message (0-30 points)
func (q *QualityEvaluatorImpl) evaluateMessageClarity(message string) float64 {
	score := 0.0

	// Get the first line (subject line)
	lines := strings.Split(message, "\n")
	subject := strings.TrimSpace(lines[0])

	// Check if it starts with a verb (imperative mood)
	words := strings.Fields(subject)
	if len(words) > 0 && q.isVerb(words[0]) {
		score += 10.0
	}

	// Check for specific, descriptive words (not vague)
	if !q.containsVagueWords(subject) {
		score += 10.0
	}

	// Check for proper capitalization (first letter should be lowercase for conventional commits)
	if len(subject) > 0 && unicode.IsLower(rune(subject[0])) {
		score += 5.0
	}

	// Check that it doesn't end with a period
	if !strings.HasSuffix(subject, ".") {
		score += 5.0
	}

	return score
}

// isVerb checks if a word is likely a verb (simple heuristic)
func (q *QualityEvaluatorImpl) isVerb(word string) bool {
	word = strings.ToLower(word)

	// Common commit verbs
	verbs := []string{
		"add", "remove", "fix", "update", "implement", "create", "delete",
		"refactor", "improve", "optimize", "enhance", "modify", "change",
		"introduce", "support", "handle", "resolve", "correct", "adjust",
		"migrate", "upgrade", "downgrade", "merge", "revert", "bump",
		"configure", "setup", "install", "uninstall", "enable", "disable",
	}

	for _, verb := range verbs {
		if word == verb {
			return true
		}
	}

	return false
}

// containsVagueWords checks if the message contains vague or non-descriptive words
func (q *QualityEvaluatorImpl) containsVagueWords(message string) bool {
	message = strings.ToLower(message)

	vagueWords := []string{
		"stuff", "things", "misc", "various", "some", "minor", "small",
		"quick", "temp", "temporary", "wip", "work in progress", "todo",
		"fixme", "hack", "dirty", "cleanup", "misc changes",
	}

	for _, vague := range vagueWords {
		if strings.Contains(message, vague) {
			return true
		}
	}

	return false
}

// GetQualityLevel returns a human-readable quality level
func GetQualityLevel(score float64) string {
	switch {
	case score >= 80:
		return "Excellent"
	case score >= 60:
		return "Good"
	default:
		return "Needs Improvement"
	}
}
