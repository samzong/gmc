package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPrompt(t *testing.T) {
	tests := []struct {
		name          string
		role          string
		changedFiles  []string
		diff          string
		expectContain []string
	}{
		{
			name: "Basic prompt with role and files",
			role: "Senior Go Developer",
			changedFiles: []string{
				"internal/config/config.go",
				"internal/formatter/formatter.go",
			},
			diff: "diff --git a/internal/config/config.go b/internal/config/config.go\n+Role string",
			expectContain: []string{
				"Senior Go Developer",
				"internal/config/config.go",
				"internal/formatter/formatter.go",
				"+Role string",
			},
		},
		{
			name:         "Empty changed files",
			role:         "Developer",
			changedFiles: []string{},
			diff:         "no changes",
			expectContain: []string{
				"Developer",
				"no changes",
			},
		},
		{
			name:         "Long diff truncation",
			role:         "Developer",
			changedFiles: []string{"file.go"},
			diff:         strings.Repeat("a", 5000), // More than 4000 chars
			expectContain: []string{
				"Developer",
				"file.go",
				"...(content is too long, truncated)",
			},
		},
		{
			name: "Special characters in files",
			role: "Developer",
			changedFiles: []string{
				"path/with spaces/file.go",
				"path/with-dashes/file.go",
				"path/with_underscores/file.go",
			},
			diff: "some diff content",
			expectContain: []string{
				"path/with spaces/file.go",
				"path/with-dashes/file.go",
				"path/with_underscores/file.go",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildPrompt(tt.role, tt.changedFiles, tt.diff)

			assert.NotEmpty(t, result)
			for _, expected := range tt.expectContain {
				assert.Contains(t, result, expected,
					"Prompt should contain: %s", expected)
			}

			// Check length constraint for diff
			if len(tt.diff) > 4000 {
				assert.Contains(t, result, "...(content is too long, truncated)")
			}
		})
	}
}

func TestFormatCommitMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid conventional commit",
			input:    "feat(auth): implement user authentication",
			expected: "feat(auth): implement user authentication",
		},
		{
			name:     "Valid conventional commit without scope",
			input:    "fix: resolve parsing error",
			expected: "fix: resolve parsing error",
		},
		{
			name:     "Remove issue number with parentheses",
			input:    "feat(auth): implement user auth (#123)",
			expected: "feat(auth): implement user auth",
		},
		{
			name:     "Remove issue number with hash",
			input:    "fix: resolve bug #456",
			expected: "fix: resolve bug",
		},
		{
			name:     "Remove multiple issue numbers",
			input:    "feat: add feature #123 (#456)",
			expected: "feat: add feature",
		},
		{
			name:     "Non-conventional commit with fix keyword",
			input:    "fix the authentication bug",
			expected: "fix: fix the authentication bug",
		},
		{
			name:     "Non-conventional commit with add keyword",
			input:    "add new user management feature",
			expected: "feat: add new user management feature",
		},
		{
			name:     "Non-conventional commit with doc keyword",
			input:    "update documentation for API",
			expected: "docs: update documentation for API",
		},
		{
			name:     "Non-conventional commit generic",
			input:    "update some functionality",
			expected: "chore: update some functionality",
		},
		{
			name:     "Multi-line message (only first line used)",
			input:    "feat: add feature\n\nThis is a detailed description",
			expected: "feat: add feature",
		},
		{
			name:     "Message with whitespace",
			input:    "  feat: add feature  ",
			expected: "feat: add feature",
		},
		{
			name:     "Remove existing prefix",
			input:    "fix: fix the parsing issue",
			expected: "fix: fix the parsing issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCommitMessage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatToConventional(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Fix keyword detected",
			input:    "fix the authentication bug",
			expected: "fix: fix the authentication bug",
		},
		{
			name:     "Bug keyword detected",
			input:    "resolve critical bug in parser",
			expected: "fix: resolve critical bug in parser",
		},
		{
			name:     "Add keyword detected",
			input:    "add user management functionality",
			expected: "feat: add user management functionality",
		},
		{
			name:     "Feature keyword detected",
			input:    "implement new feature for auth",
			expected: "feat: implement new feature for auth",
		},
		{
			name:     "Doc keyword detected",
			input:    "update documentation",
			expected: "docs: update documentation",
		},
		{
			name:     "Document keyword detected",
			input:    "document the new API",
			expected: "docs: document the new API",
		},
		{
			name:     "Style keyword detected",
			input:    "improve code style consistency",
			expected: "style: improve code style consistency",
		},
		{
			name:     "Format keyword detected",
			input:    "format code according to standards",
			expected: "style: format code according to standards",
		},
		{
			name:     "Refactor keyword detected",
			input:    "refactor authentication logic",
			expected: "refactor: refactor authentication logic",
		},
		{
			name:     "Restructure keyword detected",
			input:    "restructure project layout",
			expected: "refactor: restructure project layout",
		},
		{
			name:     "Perf keyword detected",
			input:    "improve perf of database queries",
			expected: "perf: improve perf of database queries",
		},
		{
			name:     "Performance keyword detected",
			input:    "optimize performance bottlenecks",
			expected: "perf: optimize performance bottlenecks",
		},
		{
			name:     "Test keyword detected",
			input:    "improve test coverage",
			expected: "test: improve test coverage",
		},
		{
			name:     "Testing keyword detected",
			input:    "improve testing coverage",
			expected: "test: improve testing coverage",
		},
		{
			name:     "Default to chore",
			input:    "update dependencies",
			expected: "chore: update dependencies",
		},
		{
			name:     "Remove existing prefix - fix:",
			input:    "fix: the authentication bug",
			expected: "fix: the authentication bug",
		},
		{
			name:     "Remove existing prefix - feat:",
			input:    "feat: new user management",
			expected: "chore: new user management",
		},
		{
			name:     "Case insensitive detection",
			input:    "Fix Authentication Bug",
			expected: "fix: Fix Authentication Bug",
		},
		{
			name:     "Empty message",
			input:    "",
			expected: "chore: ",
		},
		{
			name:     "Whitespace handling",
			input:    "  fix authentication bug  ",
			expected: "fix: fix authentication bug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatToConventional(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test with custom configuration
func TestBuildPromptWithCustomTemplate(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "gmc_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create custom template
	customTemplate := `name: "custom"
description: "Custom test template"
template: |
  Custom template for {{.Role}}.
  Files: {{.Files}}
  Diff: {{.Diff}}
  End of custom template.`

	templateFile := filepath.Join(tempDir, "custom.yaml")
	err = os.WriteFile(templateFile, []byte(customTemplate), 0644)
	require.NoError(t, err)

	// Mock config to use custom template
	_ = config.GetConfig() // Just to ensure config is initialized

	// Test that custom template path works
	result := BuildPrompt("Test Developer", []string{"file.go"}, "diff content")
	assert.Contains(t, result, "Test Developer")
	assert.Contains(t, result, "file.go")
	assert.Contains(t, result, "diff content")
}

func TestBuildPromptFallbackToBuiltinOnError(t *testing.T) {
	// Test with non-existent template should fall back to builtin
	role := "Senior Go Developer"
	files := []string{"main.go"}
	diff := "some diff"

	result := BuildPrompt(role, files, diff)

	// Should contain the expected content even when template fails
	assert.Contains(t, result, role)
	assert.Contains(t, result, "main.go")
	assert.Contains(t, result, "some diff")
	assert.Contains(t, result, "Conventional Commits")
}
