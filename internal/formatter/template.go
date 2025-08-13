package formatter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/samzong/gmc/internal/config"
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
}{
	Header:   "{{.Role}}, please generate a commit message that follows the Conventional Commits specification",
	Files:    "Changed Files:\n{{.Files}}",
	Content:  "Changed Content:\n{{.Diff}}",
	Format:   "in the format of \"type(scope): description\"",
	NoIssues: "Do not include issue numbers, the system will handle them automatically.",
}

var builtinTemplates = map[string]string{
	"default": fmt.Sprintf(`You are a professional %s based on the following Git changes:

%s

%s

Please generate a commit message %s.
The type should be the most appropriate from the following choices: feat, fix, docs, style, refactor, perf, test, chore.
The description should be concise (no more than 150 characters) and accurately reflect the changes.
%s`, templateParts.Header, templateParts.Files, templateParts.Content, templateParts.Format, templateParts.NoIssues),

	"detailed": fmt.Sprintf(`As a seasoned %s:

%s

%s

Please provide a commit message %s, where:
1. The type must be the most appropriate from the following choices:
   - feat: new feature
   - fix: bug fix
   - docs: documentation changes
   - style: code style changes (e.g., formatting, missing semicolons, etc.)
   - refactor: code changes that neither fix a bug nor add a feature
   - perf: performance improvements
   - test: adding or correcting tests
   - chore: changes to build process or auxiliary tools and libraries

2. The scope (optional): should clearly identify the component or module that has been changed
3. The description: must be concise (no more than 150 characters) and accurately reflect the changes, `+
		`using an imperative sentence starting with a verb

%s`, templateParts.Header, templateParts.Files, templateParts.Content, templateParts.Format, templateParts.NoIssues),
}

func GetPromptTemplate(templateName string) (string, error) {
	if template, ok := builtinTemplates[templateName]; ok {
		return template, nil
	}

	if _, err := os.Stat(templateName); err == nil {
		content, err := os.ReadFile(templateName)
		if err != nil {
			return "", fmt.Errorf("unable to read template file %s: %w", templateName, err)
		}

		var tpl PromptTemplate
		if err := yaml.Unmarshal(content, &tpl); err != nil {
			// If YAML parsing fails, treat as plain text template
			return string(content), nil //nolint:nilerr // Intentional fallback to plain text
		}

		return tpl.Template, nil
	}

	cfg := config.GetConfig()
	if cfg.PromptsDir != "" {
		customPath := filepath.Join(cfg.PromptsDir, templateName)
		if filepath.Ext(customPath) == "" {
			customPath += ".yaml"
		}

		if _, err := os.Stat(customPath); err == nil {
			content, err := os.ReadFile(customPath)
			if err != nil {
				return "", fmt.Errorf("unable to read template file %s: %w", customPath, err)
			}

			var tpl PromptTemplate
			if err := yaml.Unmarshal(content, &tpl); err != nil {
				// If YAML parsing fails, treat as plain text template
				return string(content), nil //nolint:nilerr // Intentional fallback to plain text
			}

			return tpl.Template, nil
		}
	}

	return "", fmt.Errorf("could not find cue template: %s", templateName)
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
	return builtinTemplates
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
