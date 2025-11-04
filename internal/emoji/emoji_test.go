package emoji

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEmojiForType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "feat type",
			input:    "feat",
			expected: "✨",
		},
		{
			name:     "fix type",
			input:    "fix",
			expected: "🐛",
		},
		{
			name:     "docs type",
			input:    "docs",
			expected: "📝",
		},
		{
			name:     "style type",
			input:    "style",
			expected: "💄",
		},
		{
			name:     "refactor type",
			input:    "refactor",
			expected: "♻️",
		},
		{
			name:     "perf type",
			input:    "perf",
			expected: "⚡",
		},
		{
			name:     "test type",
			input:    "test",
			expected: "✅",
		},
		{
			name:     "chore type",
			input:    "chore",
			expected: "🔧",
		},
		{
			name:     "case insensitive - uppercase",
			input:    "FEAT",
			expected: "✨",
		},
		{
			name:     "case insensitive - mixed",
			input:    "FiX",
			expected: "🐛",
		},
		{
			name:     "unknown type",
			input:    "unknown",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetEmojiForType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAddEmojiToMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "feat with scope",
			input:    "feat(auth): add user authentication",
			expected: "✨ feat(auth): add user authentication",
		},
		{
			name:     "fix without scope",
			input:    "fix: resolve parsing error",
			expected: "🐛 fix: resolve parsing error",
		},
		{
			name:     "docs type",
			input:    "docs: update README",
			expected: "📝 docs: update README",
		},
		{
			name:     "style type",
			input:    "style: format code",
			expected: "💄 style: format code",
		},
		{
			name:     "refactor type",
			input:    "refactor(api): restructure handlers",
			expected: "♻️ refactor(api): restructure handlers",
		},
		{
			name:     "perf type",
			input:    "perf: optimize database queries",
			expected: "⚡ perf: optimize database queries",
		},
		{
			name:     "test type",
			input:    "test: add unit tests",
			expected: "✅ test: add unit tests",
		},
		{
			name:     "chore type",
			input:    "chore: update dependencies",
			expected: "🔧 chore: update dependencies",
		},
		{
			name:     "message already has emoji",
			input:    "✨ feat: already has emoji",
			expected: "✨ feat: already has emoji",
		},
		{
			name:     "message starts with different emoji",
			input:    "🐛 fix: bug fix with emoji",
			expected: "🐛 fix: bug fix with emoji",
		},
		{
			name:     "non-conventional format",
			input:    "add new feature",
			expected: "add new feature",
		},
		{
			name:     "invalid format",
			input:    "not a commit message",
			expected: "not a commit message",
		},
		{
			name:     "unknown commit type",
			input:    "unknown: some message",
			expected: "unknown: some message",
		},
		{
			name:     "empty message",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "message with leading whitespace",
			input:    "  feat: add feature",
			expected: "✨ feat: add feature",
		},
		{
			name:     "message with trailing whitespace",
			input:    "fix: resolve bug   ",
			expected: "🐛 fix: resolve bug",
		},
		{
			name:     "multi-line message",
			input:    "feat: add feature\n\nDetailed description",
			expected: "✨ feat: add feature\n\nDetailed description",
		},
		{
			name:     "message with issue number",
			input:    "feat: add feature (#123)",
			expected: "✨ feat: add feature (#123)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AddEmojiToMessage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCommitType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "type with scope",
			input:    "feat(auth): add authentication",
			expected: "feat",
		},
		{
			name:     "type without scope",
			input:    "fix: resolve bug",
			expected: "fix",
		},
		{
			name:     "uppercase type",
			input:    "FEAT: add feature",
			expected: "feat",
		},
		{
			name:     "mixed case type",
			input:    "FiX: fix bug",
			expected: "fix",
		},
		{
			name:     "invalid format",
			input:    "not a commit message",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "type without colon",
			input:    "feat add feature",
			expected: "",
		},
		{
			name:     "complex scope",
			input:    "refactor(api/handlers): restructure",
			expected: "refactor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCommitType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsEmoji(t *testing.T) {
	tests := []struct {
		name     string
		rune     rune
		expected bool
	}{
		{
			name:     "sparkles emoji",
			rune:     '✨',
			expected: true,
		},
		{
			name:     "bug emoji",
			rune:     '🐛',
			expected: true,
		},
		{
			name:     "memo emoji",
			rune:     '📝',
			expected: true,
		},
		{
			name:     "regular letter",
			rune:     'f',
			expected: false,
		},
		{
			name:     "number",
			rune:     '1',
			expected: false,
		},
		{
			name:     "space",
			rune:     ' ',
			expected: false,
		},
		{
			name:     "colon",
			rune:     ':',
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEmoji(tt.rune)
			assert.Equal(t, tt.expected, result)
		})
	}
}
