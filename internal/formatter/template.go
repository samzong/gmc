package formatter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/emoji"
	"gopkg.in/yaml.v3"
)

type PromptTemplate struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Template    string `yaml:"template"`
}

type TemplateData struct {
	Role  string
	Files string
	Diff  string
}

// Common template parts that are shared between templates
var templateParts = struct {
	Header   string
	Files    string
	Content  string
	Format   string
	NoIssues string
	Emoji    string
}{
	Header:   "{{.Role}}, craft a Conventional Commits-style summary for the changes below.",
	Files:    "Files touched:\n{{.Files}}",
	Content:  "Diff excerpt:\n{{.Diff}}",
	Format:   "Use the \"type(scope): description\" syntax",
	NoIssues: "Skip issue references; gmc appends them automatically.",
	Emoji:    "", // Will be initialized by initTemplateParts()
}

// initTemplateParts initializes template parts with dynamic content from emoji module
func initTemplateParts() {
	templateParts.Emoji = fmt.Sprintf(
		"Lead with an emoji that matches the commit type (%s).",
		emoji.GetEmojiDescription(),
	)
}

func init() {
	initTemplateParts()
}

func buildDefaultTemplateContent() string {
	return buildDefaultTemplateContentWith(config.MustGetConfig())
}

func buildDefaultTemplateContentWith(cfg *config.Config) string {
	enableEmoji := cfg != nil && cfg.EnableEmoji

	formatMsg := templateParts.Format
	emojiInstruction := ""
	if enableEmoji {
		formatMsg = `Use the "emoji type(scope): description" syntax`
		emojiInstruction = templateParts.Emoji + "\n"
	}

	return fmt.Sprintf(
		`%s

%s

%s

Reply with one line. %s.
Select the most fitting type from: %s.
%sKeep the description under 150 characters and describe the behavior change.
%s`,
		templateParts.Header,
		templateParts.Files,
		templateParts.Content,
		formatMsg,
		strings.Join(emoji.GetAllCommitTypes(), ", "),
		emojiInstruction,
		templateParts.NoIssues,
	)
}

func readTemplateFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read template file %s: %w", filePath, err)
	}

	if strings.TrimSpace(string(content)) == "" {
		return "", fmt.Errorf("prompt template file is empty: %s", filePath)
	}

	var tpl PromptTemplate
	if err := yaml.Unmarshal(content, &tpl); err != nil {
		return string(content), nil //nolint:nilerr // Intentional fallback to plain text
	}

	if strings.TrimSpace(tpl.Template) == "" {
		return "", fmt.Errorf("prompt template file has no template body: %s", filePath)
	}

	return tpl.Template, nil
}

func GetPromptTemplate(templateName string) (string, error) {
	if templateName == "" || templateName == config.DefaultPromptTemplate {
		content := buildDefaultTemplateContent()
		if content != "" {
			return content, nil
		}
	}

	// Expand ~ to home directory
	if strings.HasPrefix(templateName, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			templateName = filepath.Join(home, templateName[2:])
		}
	}

	info, err := os.Stat(templateName)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("prompt template file not found: %s", templateName)
		}
		return "", fmt.Errorf("unable to stat prompt template file %s: %w", templateName, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("prompt template path is a directory: %s", templateName)
	}

	return readTemplateFile(templateName)
}

func RenderTemplate(templateContent string, data TemplateData) (string, error) {
	tmpl, err := template.New("prompt").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("template parsing error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template rendering error: %w", err)
	}

	return buf.String(), nil
}
