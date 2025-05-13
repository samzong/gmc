package formatter

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"

	"github.com/samzong/gma/internal/config"
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

var builtinTemplates = map[string]string{
	"default": `You are a professional {{.Role}}, please generate a commit message that follows the Conventional Commits specification based on the following Git changes:

Changed Files:
{{.Files}}

Changed Content:
{{.Diff}}

Please generate a commit message in the format of "type(scope): description".
The type should be the most appropriate from the following choices: feat, fix, docs, style, refactor, perf, test, chore.
The description should be concise (no more than 150 characters) and accurately reflect the changes.
Do not add issue numbers like "#123" or "(#123)" in the commit message, this will be handled automatically by the tool.`,

	"detailed": `As a seasoned {{.Role}}, please carefully analyze the following Git changes and generate a commit message that follows the Conventional Commits specification:

Changed Files:
{{.Files}}

Changed Content:
{{.Diff}}

Please provide a commit message in the format of "type(scope): description", where:
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
3. The description: must be concise (no more than 150 characters) and accurately reflect the changes, using an imperative sentence starting with a verb

Do not include issue numbers, the system will handle them automatically.`,
}

func GetPromptTemplate(templateName string) (string, error) {
	if template, ok := builtinTemplates[templateName]; ok {
		return template, nil
	}

	if _, err := os.Stat(templateName); err == nil {
		content, err := ioutil.ReadFile(templateName)
		if err != nil {
			return "", fmt.Errorf("Unable to read template file %s: %w", templateName, err)
		}

		var tpl PromptTemplate
		if err := yaml.Unmarshal(content, &tpl); err != nil {
			return string(content), nil
		}

		return tpl.Template, nil
	}

	cfg := config.GetConfig()
	if cfg.CustomPromptsDir != "" {
		customPath := filepath.Join(cfg.CustomPromptsDir, templateName)
		if filepath.Ext(customPath) == "" {
			customPath += ".yaml"
		}

		if _, err := os.Stat(customPath); err == nil {
			content, err := ioutil.ReadFile(customPath)
			if err != nil {
				return "", fmt.Errorf("Unable to read custom template file %s: %w", customPath, err)
			}

			var tpl PromptTemplate
			if err := yaml.Unmarshal(content, &tpl); err != nil {
				return string(content), nil
			}

			return tpl.Template, nil
		}
	}

	return "", fmt.Errorf("Could not find cue template: %s", templateName)
}

func RenderTemplate(templateContent string, data TemplateData) (string, error) {
	tmpl, err := template.New("prompt").Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("Template parsing error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("Template rendering error: %w", err)
	}

	return buf.String(), nil
}

func GetBuiltinTemplates() map[string]string {
	return builtinTemplates
}

func ListCustomTemplates(dirPath string) ([]string, error) {
	fi, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Directory does not exist: %s", dirPath)
		}
		return nil, err
	}
	
	if !fi.IsDir() {
		return nil, fmt.Errorf("Path is not a directory: %s", dirPath)
	}
	
	files, err := ioutil.ReadDir(dirPath)
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
			content, err := ioutil.ReadFile(filePath)
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