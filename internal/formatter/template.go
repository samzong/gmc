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
	initBuiltinTemplates()
}

var builtinTemplates map[string]string

// initBuiltinTemplates initializes builtin templates with dynamic content
func initBuiltinTemplates() {
	// Templates will be generated dynamically based on enable_emoji config
	builtinTemplates = map[string]string{
		"default":  "", // Will be generated dynamically
		"detailed": "", // Will be generated dynamically
	}
}

// buildTemplateContent builds template content based on enable_emoji configuration
func buildTemplateContent(templateName string) string {
	cfg := config.GetConfig()
	enableEmoji := cfg.EnableEmoji

	formatMsg := templateParts.Format
	emojiInstruction := ""
	if enableEmoji {
		formatMsg = `Use the "emoji type(scope): description" syntax`
		emojiInstruction = templateParts.Emoji + "\n"
	}

	switch templateName {
	case "default":
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
	case "detailed":
		emojiSection := ""
		if enableEmoji {
			emojiSection = fmt.Sprintf(
				"1. Lead with an emoji that matches the commit type:\n%s\n\n",
				buildDetailedEmojiList(),
			)
		}
		return fmt.Sprintf(
			`As a seasoned %s

%s

%s

Provide a one-line summary. %s.
%s2. Scope (optional) should identify the impacted component.
3. Description must stay under 150 characters, start with an imperative verb, and explain the change.

%s`,
			templateParts.Header,
			templateParts.Files,
			templateParts.Content,
			formatMsg,
			emojiSection,
			templateParts.NoIssues,
		)
	default:
		return ""
	}
}

// buildDetailedEmojiList builds a formatted list of emoji mappings for the detailed template
func buildDetailedEmojiList() string {
	types := emoji.GetAllCommitTypes()
	lines := make([]string, 0, len(types))
	for _, t := range types {
		emojiChar := emoji.GetEmojiForType(t)
		description := getCommitTypeDescription(t)
		lines = append(lines, fmt.Sprintf("   - %s %s: %s", emojiChar, t, description))
	}
	return strings.Join(lines, "\n")
}

// getCommitTypeDescription returns a human-readable description for a commit type
func getCommitTypeDescription(commitType string) string {
	descriptions := map[string]string{
		"feat":     "new feature",
		"fix":      "bug fix",
		"docs":     "documentation changes",
		"style":    "code style changes (e.g., formatting, missing semicolons, etc.)",
		"refactor": "code changes that neither fix a bug nor add a feature",
		"perf":     "performance improvements",
		"test":     "adding or correcting tests",
		"chore":    "changes to build process or auxiliary tools and libraries",
		"build":    "changes to build system or dependencies",
		"ci":       "changes to CI configuration files and scripts",
		"revert":   "revert a previous commit",
		"release":  "release a new version",
		"hotfix":   "hotfix a critical issue",
		"security": "security fix",
		"other":    "other changes",
		"wip":      "work in progress",
		"deps":     "dependency update",
	}
	if desc, ok := descriptions[commitType]; ok {
		return desc
	}
	return "changes"
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
	if template, ok := builtinTemplates[templateName]; ok {
		// Generate template dynamically based on enable_emoji config
		content := buildTemplateContent(templateName)
		if content != "" {
			return content, nil
		}
		// Fallback to stored template if dynamic generation fails
		if template != "" {
			return template, nil
		}
	}

	if _, err := os.Stat(templateName); err == nil {
		return readTemplateFile(templateName)
	}

	// Check repo-level prompts directory first (./.gmc/prompts/)
	cwd, _ := os.Getwd()
	if cwd != "" {
		repoPromptsPath := filepath.Join(cwd, ".gmc", "prompts", templateName)
		if filepath.Ext(repoPromptsPath) == "" {
			repoPromptsPath += ".yaml"
		}
		if _, err := os.Stat(repoPromptsPath); err == nil {
			return readTemplateFile(repoPromptsPath)
		}
	}

	// Fallback to home prompts directory
	cfg := config.GetConfig()
	if cfg.PromptsDir != "" {
		customPath := filepath.Join(cfg.PromptsDir, templateName)
		if filepath.Ext(customPath) == "" {
			customPath += ".yaml"
		}

		if _, err := os.Stat(customPath); err == nil {
			return readTemplateFile(customPath)
		}
	}

	return "", fmt.Errorf("could not find prompt template: %s", templateName)
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

func GetBuiltinTemplates() map[string]string {
	result := make(map[string]string)
	for name := range builtinTemplates {
		content := buildTemplateContent(name)
		if content != "" {
			result[name] = content
		}
	}
	return result
}

func ListTemplates(dirPath string) ([]string, error) {
	fi, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("directory does not exist: %s", dirPath)
		}
		return nil, err
	}

	if !fi.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", dirPath)
	}

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var templates []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		ext := filepath.Ext(file.Name())
		if ext == ".yaml" || ext == ".yml" {
			filePath := filepath.Join(dirPath, file.Name())
			content, err := os.ReadFile(filePath)
			if err == nil {
				var tpl PromptTemplate
				if err := yaml.Unmarshal(content, &tpl); err == nil && tpl.Template != "" {
					name := file.Name()
					name = name[:len(name)-len(ext)]
					if tpl.Name != "" {
						templates = append(templates, fmt.Sprintf("%s (%s)", name, tpl.Name))
					} else {
						templates = append(templates, name)
					}
				}
			}
		}
	}

	return templates, nil
}
