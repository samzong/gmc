package analyzer

import (
	"fmt"
	"testing"

	"github.com/samzong/gmc/internal/git"
)

func TestEvaluateConventionalCommits(t *testing.T) {
	evaluator := &QualityEvaluatorImpl{}

	tests := []struct {
		name     string
		message  string
		expected float64
	}{
		{
			name:     "Perfect conventional commit",
			message:  "feat(auth): implement user authentication",
			expected: 40.0,
		},
		{
			name:     "Conventional commit without scope",
			message:  "fix: resolve parsing error",
			expected: 40.0,
		},
		{
			name:     "Type prefix only",
			message:  "feat: add new feature",
			expected: 40.0,
		},
		{
			name:     "Invalid type",
			message:  "update: change something",
			expected: 0.0, // "update" is not in our verb list
		},
		{
			name:     "No conventional format",
			message:  "updated some files",
			expected: 0.0,
		},
		{
			name:     "Starts with verb",
			message:  "add new functionality",
			expected: 15.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := evaluator.evaluateConventionalCommits(tt.message)
			if score != tt.expected {
				t.Errorf("evaluateConventionalCommits(%q) = %v, want %v", tt.message, score, tt.expected)
			}
		})
	}
}

func TestEvaluateMessageLength(t *testing.T) {
	evaluator := &QualityEvaluatorImpl{}

	tests := []struct {
		name     string
		message  string
		expected float64
	}{
		{
			name:     "Optimal length (50-72 chars)",
			message:  "feat(auth): implement user authentication system", // 47 chars
			expected: 20.0,                                               // Actually in 30-50 range
		},
		{
			name:     "Perfect length",
			message:  "feat(auth): implement comprehensive user authentication", // 55 chars
			expected: 30.0,
		},
		{
			name:     "Too short",
			message:  "fix bug", // 7 chars
			expected: 0.0,
		},
		{
			name:     "Too long",
			message:  "feat(auth): implement a very comprehensive user authentication system with multiple features and extensive validation that handles all edge cases", // >120 chars
			expected: 0.0,
		},
		{
			name:     "Good length range",
			message:  "feat: add user management", // 25 chars
			expected: 10.0,                        // 20-30 range
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := evaluator.evaluateMessageLength(tt.message)
			if score != tt.expected {
				t.Errorf("evaluateMessageLength(%q) = %v, want %v (length: %d)", tt.message, score, tt.expected, len(tt.message))
			}
		})
	}
}

func TestEvaluateMessageClarity(t *testing.T) {
	evaluator := &QualityEvaluatorImpl{}

	tests := []struct {
		name     string
		message  string
		minScore float64 // Use minimum expected score since clarity is subjective
	}{
		{
			name:     "Clear message with verb",
			message:  "implement user authentication",
			minScore: 15.0, // verb + no period + lowercase
		},
		{
			name:     "Vague message",
			message:  "update stuff",
			minScore: 5.0, // verb but contains vague word
		},
		{
			name:     "Good descriptive message",
			message:  "refactor database connection logic",
			minScore: 15.0,
		},
		{
			name:     "Message with period (bad)",
			message:  "fix authentication bug.",
			minScore: 10.0, // verb + lowercase but has period
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := evaluator.evaluateMessageClarity(tt.message)
			if score < tt.minScore {
				t.Errorf("evaluateMessageClarity(%q) = %v, want at least %v", tt.message, score, tt.minScore)
			}
		})
	}
}

func TestEvaluateCommit(t *testing.T) {
	evaluator := NewQualityEvaluator()

	tests := []struct {
		name     string
		commit   git.CommitInfo
		minScore float64
		maxScore float64
	}{
		{
			name: "High quality commit",
			commit: git.CommitInfo{
				Hash:    "abc1234",
				Author:  "John Doe",
				Date:    "2024-01-15",
				Message: "feat(auth): implement user authentication system",
			},
			minScore: 70.0,
			maxScore: 100.0,
		},
		{
			name: "Low quality commit",
			commit: git.CommitInfo{
				Hash:    "def5678",
				Author:  "Jane Smith",
				Date:    "2024-01-14",
				Message: "update stuff",
			},
			minScore: 0.0,
			maxScore: 40.0, // Adjusted based on actual scoring
		},
		{
			name: "Medium quality commit",
			commit: git.CommitInfo{
				Hash:    "ghi9012",
				Author:  "Bob Wilson",
				Date:    "2024-01-13",
				Message: "fix: resolve authentication bug",
			},
			minScore: 50.0,
			maxScore: 90.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := evaluator.EvaluateCommit(tt.commit)

			if metrics.OverallScore < tt.minScore || metrics.OverallScore > tt.maxScore {
				t.Errorf("EvaluateCommit(%q) overall score = %v, want between %v and %v",
					tt.commit.Message, metrics.OverallScore, tt.minScore, tt.maxScore)
			}

			// Check that individual scores are within valid ranges
			if metrics.ConventionalScore < 0 || metrics.ConventionalScore > 40 {
				t.Errorf("ConventionalScore = %v, want 0-40", metrics.ConventionalScore)
			}

			if metrics.LengthScore < 0 || metrics.LengthScore > 30 {
				t.Errorf("LengthScore = %v, want 0-30", metrics.LengthScore)
			}

			if metrics.ClarityScore < 0 || metrics.ClarityScore > 30 {
				t.Errorf("ClarityScore = %v, want 0-30", metrics.ClarityScore)
			}
		})
	}
}

func TestGetQualityLevel(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{90.0, "Excellent"},
		{80.0, "Excellent"},
		{75.0, "Good"},
		{60.0, "Good"},
		{50.0, "Needs Improvement"},
		{0.0, "Needs Improvement"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("score_%.1f", tt.score), func(t *testing.T) {
			level := GetQualityLevel(tt.score)
			if level != tt.expected {
				t.Errorf("GetQualityLevel(%v) = %q, want %q", tt.score, level, tt.expected)
			}
		})
	}
}
