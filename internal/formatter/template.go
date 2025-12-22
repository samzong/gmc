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

// buildDefaultTemplateContent builds the default template content based on enable_emoji configuration
func buildDefaultTemplateContent() string {
	cfg := config.GetConfig()
	enableEmoji := cfg.EnableEmoji

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

// readTemplateFile reads and parses a template file.
// Returns the template content if successful, or an error if the file cannot be read.
// If YAML parsing fails, returns the raw content as plain text.
func readTemplateFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read template file %s: %w", filePath, err)
	}

	var tpl PromptTemplate
	if err := yaml.Unmarshal(content, &tpl); err != nil {
		// If YAML parsing fails, treat as plain text template
		return string(content), nil //nolint:nilerr // Intentional fallback to plain text
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
