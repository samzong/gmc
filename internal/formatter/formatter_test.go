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
	for _, tt := range []struct {
		name, role, diff, excerpt string
		files                     []string
	}{
		{"basic", "Senior Go Developer", "diff --git a/config.go b/config.go\n+Role string", "+Role string",
			[]string{"internal/config/config.go", "internal/formatter/formatter.go"}},
		{"empty files", "Developer", "no changes", "no changes", nil},
		{"long diff", "Developer", "diff --git a/file.go b/file.go\n" + strings.Repeat("a", 5000), "diff --git",
			[]string{"file.go"}},
		{"special paths", "Developer", "some diff content", "some diff content",
			[]string{"path/with spaces/file.go", "path/with-dashes/file.go", "path/with_underscores/file.go"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildPromptWithConfig(&config.Config{Role: tt.role}, tt.files, tt.diff, "", "")
			assert.NotEmpty(t, result)
			assert.Contains(t, result, tt.role)
			assert.Contains(t, result, tt.excerpt)
			for _, file := range tt.files {
				assert.Contains(t, result, file)
			}
		})
	}
}

func TestFormatCommitMessage(t *testing.T) {
	for _, tt := range []struct{ input, expected string }{
		{"feat(auth): implement user authentication", "✨ feat(auth): implement user authentication"},
		{"fix: resolve parsing error", "🐛 fix: resolve parsing error"},
		{"feat(auth): implement user auth (#123)", "✨ feat(auth): implement user auth"},
		{"fix: resolve bug #456", "🐛 fix: resolve bug"},
		{"feat: add feature #123 (#456)", "✨ feat: add feature"},
		{"✨(formatter): add diff truncator", "✨ feat(formatter): add diff truncator"},
		{"fix the authentication bug", "fix the authentication bug"},
		{"add new user management feature", "add new user management feature"},
		{"update documentation for API", "update documentation for API"},
		{"update some functionality", "update some functionality"},
		{"feat: add feature\n\nThis is a detailed description", "✨ feat: add feature"},
		{"  feat: add feature  ", "✨ feat: add feature"},
		{"fix: fix the parsing issue", "🐛 fix: fix the parsing issue"},
		{"✨ feat: add feature", "✨ feat: add feature"},
		{"docs: update README", "📝 docs: update README"},
		{"style: format code", "💄 style: format code"},
		{"refactor: restructure code", "♻️ refactor: restructure code"},
		{"perf: optimize queries", "⚡️ perf: optimize queries"},
		{"test: add unit tests", "✅ test: add unit tests"},
		{"chore: update dependencies", "🔧 chore: update dependencies"},
		{"other: tweak docs", "🔧 other: tweak docs"},
		{"other(utils): some change", "🔧 other(utils): some change"},
		{"🔧 other: tweak docs", "🔧 other: tweak docs"},
		{"CI: refresh workflows", "💚 ci: refresh workflows"},
		{"BUILD: update dependencies", "👷 build: update dependencies"},
		{"FeAt: add new feature", "✨ feat: add new feature"},
		{"DEPS: update packages", "⬆️ deps: update packages"},
		{"CI(workflows): refresh config", "💚 ci(workflows): refresh config"},
		{"💚 CI: refresh workflows", "💚 ci: refresh workflows"},
		{"hotfix: critical bug fix", "🚑️ hotfix: critical bug fix"},
		{"security: fix vulnerability", "🔒️ security: fix vulnerability"},
		{"release: new version", "🚀 release: new version"},
		{"ci: configure workflow dependencies", "💚 ci: configure workflow dependencies"},
		{"circular buffer fix", "circular buffer fix"},
		{"city planning update", "city planning update"},
		{"swipe gesture bug", "swipe gesture bug"},
		{"whip action fix", "whip action fix"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatCommitMessageWithConfig(&config.Config{EnableEmoji: true}, tt.input))
		})
	}
}

func TestBuildPromptWithCustomTemplate(t *testing.T) {
	tempDir := t.TempDir()

	customTemplate := `name: "custom"
description: "Custom test template"
template: |
  Custom template for {{.Role}}.
  Files: {{.Files}}
  Diff: {{.Diff}}
  End of custom template.`

	templateFile := filepath.Join(tempDir, "custom.yaml")
	err := os.WriteFile(templateFile, []byte(customTemplate), 0644)
	require.NoError(t, err)

	result := BuildPromptWithConfig(&config.Config{Role: "Test Developer", PromptTemplate: templateFile},
		[]string{"file.go"}, "diff content", "", "")
	assert.Contains(t, result, "Custom template for Test Developer.")
	assert.Contains(t, result, "Test Developer")
	assert.Contains(t, result, "file.go")
	assert.Contains(t, result, "diff content")
}

func TestBuildPromptFallbackToBuiltinOnError(t *testing.T) {
	role := "Senior Go Developer"
	files := []string{"main.go"}
	diff := "some diff"

	result := BuildPromptWithConfig(&config.Config{Role: role, PromptTemplate: "missing.yaml"}, files, diff, "", "")

	assert.Contains(t, result, role)
	assert.Contains(t, result, "main.go")
	assert.Contains(t, result, "some diff")
	assert.Contains(t, result, "Conventional Commits")
}

func TestBuildPromptWithUserPrompt(t *testing.T) {
	for _, context := range []string{
		"This is a critical bug fix", "", "This change:\n- Fixes issue #123\n- Improves performance",
	} {
		t.Run(context, func(t *testing.T) {
			result := BuildPromptWithConfig(&config.Config{Role: "Developer"}, []string{"file.go"}, "some diff", "", context)
			for _, expected := range []string{"Developer", "file.go", "some diff"} {
				assert.Contains(t, result, expected)
			}
			if context == "" {
				assert.NotContains(t, result, "Additional Context:")
			} else {
				assert.Contains(t, result, "Additional Context:\n"+context)
			}
		})
	}
}
