package formatter

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/samzong/gmc/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeCommitMessage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain subject is untouched", input: "feat: add thing", want: "feat: add thing"},
		{name: "surrounding whitespace is trimmed", input: "  feat: add thing  ", want: "feat: add thing"},
		{name: "fenced answer keeps only the message", input: "```\nfeat: add thing\n```", want: "feat: add thing"},
		{name: "fenced answer with a language tag", input: "```text\nfeat: add thing\n```", want: "feat: add thing"},
		{
			name:  "prose after the closing fence is dropped",
			input: "```\nfeat: add thing\n```\n\nThis message follows Conventional Commits.",
			want:  "feat: add thing",
		},
		{
			name:  "preamble line is dropped",
			input: "Here is the commit message:\nfeat: add thing",
			want:  "feat: add thing",
		},
		{name: "bold preamble is dropped", input: "**Commit message:**\nfeat: add thing", want: "feat: add thing"},
		{name: "heading preamble is dropped", input: "### Commit message:\nfeat: add thing", want: "feat: add thing"},
		{name: "bullet preamble is dropped", input: "- Commit message:\nfeat: add thing", want: "feat: add thing"},
		{
			name:  "heading preamble followed by a fenced block",
			input: "### Commit Message\n\n```\nfeat: add thing\n```",
			want:  "feat: add thing",
		},
		{
			name:  "sentence label without a colon",
			input: "### Here is the commit message\nfeat: add thing",
			want:  "feat: add thing",
		},
		{
			name:  "possessive sentence label",
			input: "Here's your commit message\nfeat: add thing",
			want:  "feat: add thing",
		},
		{
			name:  "wrapper inside a fenced block",
			input: "```\nHere is the commit message:\nfeat: add thing\n```",
			want:  "feat: add thing",
		},
		{
			name:  "chatter inside a one line fence",
			input: "```Sure, here is the commit message: feat: add thing```",
			want:  "feat: add thing",
		},
		{name: "closing fence sharing the message line", input: "```\nfeat: add thing```", want: "feat: add thing"},
		{name: "unclosed fence carrying content", input: "```feat: add thing", want: ""},
		{name: "list bullet before the subject", input: "- feat: add thing", want: "feat: add thing"},
		{
			name:  "fence carrying an info string",
			input: "```json title=x\nfeat: add thing\n```",
			want:  "feat: add thing",
		},
		{name: "fence opened and closed on one line", input: "```feat: add thing```", want: "feat: add thing"},
		{name: "tilde fence", input: "~~~\nfeat: add thing\n~~~", want: "feat: add thing"},
		{name: "subject ending in an underscore survives", input: "fix: handle foo_", want: "fix: handle foo_"},
		{
			name:  "emphasis inside the subject survives",
			input: "feat: add **bold** support",
			want:  "feat: add **bold** support",
		},
		{
			name:  "preamble with a blank line after it",
			input: "Here is the commit message:\n\nfeat: add thing",
			want:  "feat: add thing",
		},
		{
			name:  "chatty preamble with the message on the same line",
			input: "Sure! Commit message: feat: add thing",
			want:  "feat: add thing",
		},
		{
			name:  "preamble then a fenced block",
			input: "Here is the commit message:\n```\nfeat: add thing\n```",
			want:  "feat: add thing",
		},
		{
			name:  "body is dropped because gmc commits one line",
			input: "feat: add thing\n\nA longer explanation of the change.",
			want:  "feat: add thing",
		},
		{name: "empty input stays empty", input: "", want: ""},
		{name: "fence only", input: "```", want: ""},
		{name: "empty fenced block", input: "```\n```", want: ""},
		{name: "preamble only", input: "Here is the commit message:", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SanitizeCommitMessage(tt.input))
		})
	}
}

func TestFormatCommitMessageWithConfigUnwrapsModelWrappers(t *testing.T) {
	cfg := &config.Config{}

	assert.Equal(t, "feat: add thing", FormatCommitMessageWithConfig(cfg, "```\nfeat: add thing\n```"))
	assert.Equal(t, "feat: add thing", FormatCommitMessageWithConfig(cfg, "Here is the commit message:\nfeat: add thing"))
	assert.Equal(t, "feat: add thing", FormatCommitMessageWithConfig(cfg, "```text\nfeat: add thing\n```"))
	assert.Empty(t, FormatCommitMessageWithConfig(cfg, "```"))
	assert.Empty(t, FormatCommitMessageWithConfig(cfg, ""))
}

func TestBuildPromptWithConfigKeepsDiffVerbatim(t *testing.T) {
	diff := "diff --git a/x.txt b/x.txt\n@@ -0,0 +1,3 @@\n+-- gmc diff stats --\n+second line\n+third line\n"

	prompt := BuildPromptWithConfig(&config.Config{Role: "Dev"}, []string{"x.txt"}, diff, "3\t0\tx.txt", "")

	assert.Contains(t, prompt, "+-- gmc diff stats --")
	assert.Contains(t, prompt, "+third line", "diff content after the separator-looking line must survive")
}

func TestBuildPromptWithConfigCapsChangedFiles(t *testing.T) {
	files := make([]string, 300)
	for i := range files {
		files[i] = fmt.Sprintf("file-%d.go", i)
	}

	prompt := BuildPromptWithConfig(&config.Config{Role: "Dev"}, files, "diff", "", "")

	assert.Contains(t, prompt, "file-0.go")
	assert.Contains(t, prompt, "file-199.go")
	assert.NotContains(t, prompt, "file-200.go")
	assert.Contains(t, prompt, "and 100 more files")
}

func TestBuildPromptWithConfigRejectsTemplateWithoutBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: broken\n"), 0o600))

	_, err := GetPromptTemplate(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no template body")

	prompt := BuildPromptWithConfig(
		&config.Config{Role: "Developer", PromptTemplate: path},
		[]string{"a.go"}, "diff content", "", "")

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "Developer")
	assert.Contains(t, prompt, "diff content")
}

func TestGetPromptTemplateRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blank.yaml")
	require.NoError(t, os.WriteFile(path, []byte("   \n"), 0o600))

	_, err := GetPromptTemplate(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestSummarizeFileLabelsNewFiles(t *testing.T) {
	assert.Equal(t, "new.txt (added) (+2/-0)", summarizeFile(DiffFile{Path: "new.txt", IsNew: true, Added: 2}))
	assert.Equal(t, "old.txt (mode changed)", summarizeFile(DiffFile{Path: "old.txt", HasModeChange: true}))
	assert.Equal(t, "a.txt (+1/-1)", summarizeFile(DiffFile{Path: "a.txt", Added: 1, Deleted: 1}))
}

func TestApplyHeaderLineDistinguishesCreationFromChmod(t *testing.T) {
	var created DiffFile
	applyHeaderLine(&created, "new file mode 100644")
	assert.True(t, created.IsNew)
	assert.False(t, created.HasModeChange)

	var chmod DiffFile
	applyHeaderLine(&chmod, "old mode 100644")
	applyHeaderLine(&chmod, "new mode 100755")
	assert.True(t, chmod.HasModeChange)
	assert.False(t, chmod.IsNew)

	var deleted DiffFile
	applyHeaderLine(&deleted, "deleted file mode 100644")
	assert.False(t, deleted.HasModeChange)
	assert.False(t, deleted.IsNew)
}

func TestSanitizeCommitMessageKeepsEmojiAndScope(t *testing.T) {
	input := "```\n✨ feat(auth): add login\n```"
	assert.Equal(t, "✨ feat(auth): add login", SanitizeCommitMessage(input))
}
