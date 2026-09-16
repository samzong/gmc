package branch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateName(t *testing.T) {
	tests := []struct {
		name        string
		description string
		expected    string
	}{
		{name: "empty description", description: "", expected: ""},
		{name: "feature keyword", description: "add user authentication", expected: "feature/add-user-authentication"},
		{name: "fix keyword", description: "fix critical security bug", expected: "fix/fix-critical-security-bug"},
		{name: "docs keyword", description: "update documentation", expected: "docs/update-documentation"},
		{name: "default chore prefix", description: "update dependencies", expected: "chore/update-dependencies"},
		{
			name:        "special characters removal",
			description: "add user@email.com validation!",
			expected:    "feature/add-useremailcom-validation",
		},
		{name: "multiple spaces", description: "add   multiple    spaces", expected: "feature/add-multiple-spaces"},
		{
			name:        "long description truncation",
			description: "this is a very long description that should be truncated properly",
			expected:    "chore/this-is-a-very-long-description-that-should-b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GenerateName(tt.description))
		})
	}
}

func TestDetectPrefix(t *testing.T) {
	tests := []struct {
		description string
		expected    string
	}{
		{"add new feature", "feature"},
		{"create user interface", "feature"},
		{"implement oauth", "feature"},
		{"fix bug in login", "fix"},
		{"resolve merge conflict", "fix"},
		{"patch security issue", "fix"},
		{"document api endpoints", "docs"},
		{"update readme file", "docs"},
		{"refactor code structure", "chore"},
		{"update dependencies", "chore"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			assert.Equal(t, tt.expected, detectPrefix(tt.description))
		})
	}
}

func TestSanitizeDescription(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Simple Description", "simple-description"},
		{"Add user@email.com validation!", "add-useremailcom-validation"},
		{"  Multiple   Spaces  ", "multiple-spaces"},
		{"Special!@#$%^&*()Characters", "specialcharacters"},
		{"---Leading-and-trailing---", "leading-and-trailing"},
		{"multiple----hyphens", "multiple-hyphens"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeDescription(tt.input))
		})
	}
}

func TestLimitLength(t *testing.T) {
	tests := []struct {
		input     string
		maxLength int
		expected  string
	}{
		{"short", 10, "short"},
		{"this-is-a-very-long-description-that-exceeds-the-limit", 20, "this-is-a-very-long"},
		{"exactly-twenty-chars", 20, "exactly-twenty-chars"},
		{"trailing-hyphen-", 10, "trailing-h"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := limitLength(tt.input, tt.maxLength)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), tt.maxLength)
		})
	}
}

func BenchmarkGenerateName(b *testing.B) {
	description := "add user authentication with oauth2 support"

	b.ResetTimer()
	for range b.N {
		GenerateName(description)
	}
}

func BenchmarkSanitizeDescription(b *testing.B) {
	description := "add user@email.com validation with special!@#$%^&*() characters"

	b.ResetTimer()
	for range b.N {
		sanitizeDescription(description)
	}
}
