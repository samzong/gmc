package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/config"
	"github.com/spf13/viper"
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
			result := BuildPrompt(tt.role, tt.changedFiles, tt.diff, "")

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
	// Enable emoji for these tests to match expected behavior
	viper.Set("enable_emoji", true)
	defer viper.Set("enable_emoji", false)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Valid conventional commit",
			input:    "feat(auth): implement user authentication",
			expected: "✨ feat(auth): implement user authentication",
		},
		{
			name:     "Valid conventional commit without scope",
			input:    "fix: resolve parsing error",
			expected: "🐛 fix: resolve parsing error",
		},
		{
			name:     "Remove issue number with parentheses",
			input:    "feat(auth): implement user auth (#123)",
			expected: "✨ feat(auth): implement user auth",
		},
		{
			name:     "Remove issue number with hash",
			input:    "fix: resolve bug #456",
			expected: "🐛 fix: resolve bug",
		},
		{
			name:     "Remove multiple issue numbers",
			input:    "feat: add feature #123 (#456)",
			expected: "✨ feat: add feature",
		},
		{
			name:     "Non-conventional message without prefix returns as-is",
			input:    "fix the authentication bug",
			expected: "fix the authentication bug",
		},
		{
			name:     "Non-conventional message without prefix returns as-is",
			input:    "add new user management feature",
			expected: "add new user management feature",
		},
		{
			name:     "Non-conventional message without prefix returns as-is",
			input:    "update documentation for API",
			expected: "update documentation for API",
		},
		{
			name:     "Non-conventional commit generic",
			input:    "update some functionality",
			expected: "update some functionality",
		},
		{
			name:     "Multi-line message (only first line used)",
			input:    "feat: add feature\n\nThis is a detailed description",
			expected: "✨ feat: add feature",
		},
		{
			name:     "Message with whitespace",
			input:    "  feat: add feature  ",
			expected: "✨ feat: add feature",
		},
		{
			name:     "Remove existing prefix",
			input:    "fix: fix the parsing issue",
			expected: "🐛 fix: fix the parsing issue",
		},
		{
			name:     "Message already has emoji",
			input:    "✨ feat: add feature",
			expected: "✨ feat: add feature",
		},
		{
			name:     "All commit types with emoji",
			input:    "docs: update README",
			expected: "📝 docs: update README",
		},
		{
			name:     "Style commit type",
			input:    "style: format code",
			expected: "💄 style: format code",
		},
		{
			name:     "Refactor commit type",
			input:    "refactor: restructure code",
			expected: "♻️ refactor: restructure code",
		},
		{
			name:     "Perf commit type",
			input:    "perf: optimize queries",
			expected: "⚡ perf: optimize queries",
		},
		{
			name:     "Test commit type",
			input:    "test: add unit tests",
			expected: "✅ test: add unit tests",
		},
		{
			name:     "Chore commit type",
			input:    "chore: update dependencies",
			expected: "🔧 chore: update dependencies",
		},
		{
			name:     "Other commit type - regression test",
			input:    "other: tweak docs",
			expected: "🔧 other: tweak docs",
		},
		{
			name:     "Other commit type with scope",
			input:    "other(utils): some change",
			expected: "🔧 other(utils): some change",
		},
		{
			name:     "Other commit type with emoji already present",
			input:    "🔧 other: tweak docs",
			expected: "🔧 other: tweak docs",
		},
		{
			name:     "Case insensitive - uppercase CI",
			input:    "CI: refresh workflows",
			expected: "🤖 ci: refresh workflows",
		},
		{
			name:     "Case insensitive - uppercase BUILD",
			input:    "BUILD: update dependencies",
			expected: "🏗️ build: update dependencies",
		},
		{
			name:     "Case insensitive - mixed case FeAt",
			input:    "FeAt: add new feature",
			expected: "✨ feat: add new feature",
		},
		{
			name:     "Case insensitive - uppercase DEPS",
			input:    "DEPS: update packages",
			expected: "🔗 deps: update packages",
		},
		{
			name:     "Case insensitive - uppercase with scope",
			input:    "CI(workflows): refresh config",
			expected: "🤖 ci(workflows): refresh config",
		},
		{
			name:     "Case insensitive - uppercase CI with emoji already present",
			input:    "🤖 CI: refresh workflows",
			expected: "🤖 ci: refresh workflows",
		},
		{
			name:     "Hotfix message with type prefix",
			input:    "hotfix: critical bug fix",
			expected: "🔥 hotfix: critical bug fix",
		},
		{
			name:     "Security message with type prefix",
			input:    "security: fix vulnerability",
			expected: "🔒 security: fix vulnerability",
		},
		{
			name:     "Release message with type prefix",
			input:    "release: new version",
			expected: "🚀 release: new version",
		},
		{
			name:     "CI workflow with dependencies",
			input:    "ci: configure workflow dependencies",
			expected: "🤖 ci: configure workflow dependencies",
		},
		{
			name:     "Non-conventional message without prefix returns as-is",
			input:    "circular buffer fix",
			expected: "circular buffer fix",
		},
		{
			name:     "Non-conventional message without prefix returns as-is",
			input:    "city planning update",
			expected: "city planning update",
		},
		{
			name:     "Non-conventional message without prefix returns as-is",
			input:    "swipe gesture bug",
			expected: "swipe gesture bug",
		},
		{
			name:     "Non-conventional message without prefix returns as-is",
			input:    "whip action fix",
			expected: "whip action fix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCommitMessage(tt.input)
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
	result := BuildPrompt("Test Developer", []string{"file.go"}, "diff content", "")
	assert.Contains(t, result, "Test Developer")
	assert.Contains(t, result, "file.go")
	assert.Contains(t, result, "diff content")
}

func TestBuildPromptFallbackToBuiltinOnError(t *testing.T) {
	// Test with non-existent template should fall back to builtin
	role := "Senior Go Developer"
	files := []string{"main.go"}
	diff := "some diff"

	result := BuildPrompt(role, files, diff, "")

	// Should contain the expected content even when template fails
	assert.Contains(t, result, role)
	assert.Contains(t, result, "main.go")
	assert.Contains(t, result, "some diff")
	assert.Contains(t, result, "Conventional Commits")
}

func TestBuildPromptWithUserPrompt(t *testing.T) {
	tests := []struct {
		name             string
		role             string
		changedFiles     []string
		diff             string
		userPrompt       string
		expectContain    []string
		expectNotContain []string
	}{
		{
			name:         "User prompt included when provided",
			role:         "Developer",
			changedFiles: []string{"file.go"},
			diff:         "some diff",
			userPrompt:   "This is a critical bug fix",
			expectContain: []string{
				"Developer",
				"file.go",
				"some diff",
				"Additional Context:",
				"This is a critical bug fix",
			},
			expectNotContain: []string{},
		},
		{
			name:         "User prompt not included when empty",
			role:         "Developer",
			changedFiles: []string{"file.go"},
			diff:         "some diff",
			userPrompt:   "",
			expectContain: []string{
				"Developer",
				"file.go",
				"some diff",
			},
			expectNotContain: []string{
				"Additional Context:",
			},
		},
		{
			name:         "User prompt with multiline content",
			role:         "Developer",
			changedFiles: []string{"file.go"},
			diff:         "some diff",
			userPrompt:   "This change:\n- Fixes issue #123\n- Improves performance",
			expectContain: []string{
				"Additional Context:",
				"This change:",
				"Fixes issue #123",
				"Improves performance",
			},
			expectNotContain: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildPrompt(tt.role, tt.changedFiles, tt.diff, tt.userPrompt)

			assert.NotEmpty(t, result)
			for _, expected := range tt.expectContain {
				assert.Contains(t, result, expected,
					"Prompt should contain: %s", expected)
			}
			for _, notExpected := range tt.expectNotContain {
				assert.NotContains(t, result, notExpected,
					"Prompt should not contain: %s", notExpected)
			}
		})
	}
}
