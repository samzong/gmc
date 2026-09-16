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

type TemplateData struct {
	Role  string
	Files string
	Diff  string
}

func buildDefaultTemplateContentWith(cfg *config.Config) string {
	formatMsg := `Use the "type(scope): description" syntax`
	emojiInstruction := ""
	if cfg != nil && cfg.EnableEmoji {
		formatMsg = `Use the "emoji type(scope): description" syntax`
		emojiInstruction = fmt.Sprintf("Lead with an emoji that matches the commit type (%s).\n", emoji.GetEmojiDescription())
	}
	return fmt.Sprintf(`{{.Role}}, craft a Conventional Commits-style summary for the changes below.

Files touched:
{{.Files}}

Diff excerpt:
{{.Diff}}

Reply with one line. %s.
Select the most fitting type from: %s.
%sKeep the description under 150 characters and describe the behavior change.
Skip issue references; gmc appends them automatically.`,
		formatMsg, strings.Join(emoji.GetAllCommitTypes(), ", "), emojiInstruction)
}

func readTemplateFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read template file %s: %w", filePath, err)
	}

	if strings.TrimSpace(string(content)) == "" {
		return "", fmt.Errorf("prompt template file is empty: %s", filePath)
	}

	var tpl struct {
		Template string `yaml:"template"`
	}
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
		return buildDefaultTemplateContentWith(config.MustGetConfig()), nil
	}

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
