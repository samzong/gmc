package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBuiltinTemplates(t *testing.T) {
	templates := GetBuiltinTemplates()

	assert.NotEmpty(t, templates)
	assert.Contains(t, templates, "default")
	assert.Contains(t, templates, "detailed")

	// Check that templates contain expected content
	defaultTemplate := templates["default"]
	assert.Contains(t, defaultTemplate, "{{.Role}}")
	assert.Contains(t, defaultTemplate, "{{.Files}}")
	assert.Contains(t, defaultTemplate, "{{.Diff}}")

	detailedTemplate := templates["detailed"]
	assert.Contains(t, detailedTemplate, "{{.Role}}")
	assert.Contains(t, detailedTemplate, "{{.Files}}")
	assert.Contains(t, detailedTemplate, "{{.Diff}}")
	assert.Contains(t, detailedTemplate, "feat: new feature")
}

func TestGetPromptTemplate_Builtin(t *testing.T) {
	tests := []struct {
		name         string
		templateName string
		expectError  bool
	}{
		{
			name:         "Default builtin template",
			templateName: "default",
			expectError:  false,
		},
		{
			name:         "Detailed builtin template",
			templateName: "detailed",
			expectError:  false,
		},
		{
			name:         "Non-existent builtin template",
			templateName: "nonexistent",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetPromptTemplate(tt.templateName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				assert.Contains(t, result, "{{.Role}}")
				assert.Contains(t, result, "{{.Files}}")
				assert.Contains(t, result, "{{.Diff}}")
			}
		})
	}
}

func TestGetPromptTemplate_File(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_template_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name         string
		fileContent  string
		expectError  bool
		expectResult string
	}{
		{
			name: "Valid YAML template",
			fileContent: `name: "test"
description: "Test template"
template: |
  Test template for {{.Role}}.
  Files: {{.Files}}
  Changes: {{.Diff}}`,
			expectError:  false,
			expectResult: "Test template for {{.Role}}.\nFiles: {{.Files}}\nChanges: {{.Diff}}",
		},
		{
			name:         "Plain text template (YAML parse fails)",
			fileContent:  "Simple text template with {{.Role}} and {{.Files}}",
			expectError:  false,
			expectResult: "Simple text template with {{.Role}} and {{.Files}}",
		},
		{
			name: "Invalid YAML but valid template",
			fileContent: `invalid yaml: [
but still {{.Role}} template content`,
			expectError: false,
			expectResult: `invalid yaml: [
but still {{.Role}} template content`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			templateFile := filepath.Join(tempDir, "test_template.yaml")
			err := os.WriteFile(templateFile, []byte(tt.fileContent), 0644)
			require.NoError(t, err)

			result, err := GetPromptTemplate(templateFile)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectResult, result)
			}

			// Clean up
			os.Remove(templateFile)
		})
	}
}

func TestGetPromptTemplate_CustomPromptsDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_prompts_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create custom template
	templateContent := `name: "custom_dir_test"
description: "Template from custom prompts dir"
template: "Custom dir template: {{.Role}}"`

	templateFile := filepath.Join(tempDir, "custom.yaml")
	err = os.WriteFile(templateFile, []byte(templateContent), 0644)
	require.NoError(t, err)

	// This test requires config mock, but for now just test the error path
	result, err := GetPromptTemplate("custom")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find cue template")
	assert.Empty(t, result)
}

func TestGetPromptTemplate_FileReadError(t *testing.T) {
	// Test with non-existent file
	result, err := GetPromptTemplate("/non/existent/path/template.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find cue template")
	assert.Empty(t, result)

	// Test file that exists but can't be read (permissions)
	tempDir, err := os.MkdirTemp("", "gmc_read_error_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	restrictedFile := filepath.Join(tempDir, "restricted.yaml")
	err = os.WriteFile(restrictedFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Change permissions to make it unreadable (this might not work on all systems)
	err = os.Chmod(restrictedFile, 0000)
	if err == nil {
		defer func() { _ = os.Chmod(restrictedFile, 0644) }() // Restore for cleanup
		result, err := GetPromptTemplate(restrictedFile)
		if err != nil {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "unable to read template file")
			assert.Empty(t, result)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name            string
		templateContent string
		data            TemplateData
		expectError     bool
		expectResult    string
	}{
		{
			name:            "Valid template with all fields",
			templateContent: "Role: {{.Role}}, Files: {{.Files}}, Diff: {{.Diff}}",
			data: TemplateData{
				Role:  "Senior Go Developer",
				Files: "main.go\nconfig.go",
				Diff:  "+added line\n-removed line",
			},
			expectError:  false,
			expectResult: "Role: Senior Go Developer, Files: main.go\nconfig.go, Diff: +added line\n-removed line",
		},
		{
			name:            "Template with missing field",
			templateContent: "Role: {{.Role}}, Unknown: {{.Unknown}}",
			data: TemplateData{
				Role: "Developer",
			},
			expectError: true,
		},
		{
			name:            "Empty template",
			templateContent: "",
			data:            TemplateData{},
			expectError:     false,
			expectResult:    "",
		},
		{
			name:            "Template with conditionals",
			templateContent: "{{if .Role}}Role: {{.Role}}{{end}}{{if .Files}} Files: {{.Files}}{{end}}",
			data: TemplateData{
				Role: "Developer",
			},
			expectError:  false,
			expectResult: "Role: Developer",
		},
		{
			name:            "Invalid template syntax",
			templateContent: "{{.Role", // Missing closing brace
			data:            TemplateData{},
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.templateContent, tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectResult, result)
			}
		})
	}
}

func TestListTemplates(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_list_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test template files
	templates := map[string]string{
		"template1.yaml": `name: "template1"
description: "First template"
template: "Template 1 content {{.Role}}"`,
		"template2.yml": `name: "template2"
description: "Second template"  
template: "Template 2 content {{.Files}}"`,
		"invalid.yaml":    `invalid yaml content [`,
		"nottemplate.txt": "This is not a template file",
		"empty.yaml": `name: ""
template: ""`, // Empty template should be ignored
	}

	for filename, content := range templates {
		err := os.WriteFile(filepath.Join(tempDir, filename), []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create a subdirectory that should be ignored
	subDir := filepath.Join(tempDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	result, err := ListTemplates(tempDir)
	require.NoError(t, err)

	// Should find valid templates
	assert.Contains(t, result, "template1 (template1)")
	assert.Contains(t, result, "template2 (template2)")

	// Should not contain invalid or non-template files
	assert.NotContains(t, result, "invalid")
	assert.NotContains(t, result, "nottemplate")
	assert.NotContains(t, result, "empty")
	assert.NotContains(t, result, "subdir")
}

func TestListTemplates_NonExistentDirectory(t *testing.T) {
	result, err := ListTemplates("/non/existent/directory")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "directory does not exist")
	assert.Nil(t, result)
}

func TestListTemplates_FileInsteadOfDirectory(t *testing.T) {
	tempFile, err := os.CreateTemp("", "notdir")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	result, err := ListTemplates(tempFile.Name())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path is not a directory")
	assert.Nil(t, result)
}

func TestTemplateData(t *testing.T) {
	data := TemplateData{
		Role:  "Senior Go Developer",
		Files: "main.go\nconfig.go\ntest.go",
		Diff:  "diff --git a/main.go b/main.go\n+added content",
	}

	assert.Equal(t, "Senior Go Developer", data.Role)
	assert.Equal(t, "main.go\nconfig.go\ntest.go", data.Files)
	assert.Contains(t, data.Diff, "+added content")
}

func TestPromptTemplate(t *testing.T) {
	template := PromptTemplate{
		Name:        "test_template",
		Description: "A test template for unit testing",
		Template:    "Test content with {{.Role}}",
	}

	assert.Equal(t, "test_template", template.Name)
	assert.Equal(t, "A test template for unit testing", template.Description)
	assert.Contains(t, template.Template, "{{.Role}}")
}

// Integration test for the complete template workflow
func TestTemplateWorkflow_EndToEnd(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gmc_workflow_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a custom template
	templateContent := `name: "workflow_test"
description: "End-to-end workflow test template"
template: |
  === Commit Message Generation ===
  Developer Role: {{.Role}}
  
  Modified Files:
  {{.Files}}
  
  Code Changes:
  {{.Diff}}
  
  Please generate an appropriate commit message.`

	templateFile := filepath.Join(tempDir, "workflow_test.yaml")
	err = os.WriteFile(templateFile, []byte(templateContent), 0644)
	require.NoError(t, err)

	// Test getting the template
	retrievedTemplate, err := GetPromptTemplate(templateFile)
	require.NoError(t, err)
	assert.Contains(t, retrievedTemplate, "=== Commit Message Generation ===")

	// Test rendering with data
	data := TemplateData{
		Role:  "Senior Go Developer",
		Files: "internal/formatter/formatter.go\ninternal/formatter/template.go",
		Diff:  "diff --git a/internal/formatter/formatter.go b/internal/formatter/formatter.go\n+func NewFunction() {}",
	}

	renderedResult, err := RenderTemplate(retrievedTemplate, data)
	require.NoError(t, err)

	// Verify the rendered content
	assert.Contains(t, renderedResult, "=== Commit Message Generation ===")
	assert.Contains(t, renderedResult, "Senior Go Developer")
	assert.Contains(t, renderedResult, "internal/formatter/formatter.go")
	assert.Contains(t, renderedResult, "internal/formatter/template.go")
	assert.Contains(t, renderedResult, "+func NewFunction() {}")

	// Test listing templates in the directory
	templates, err := ListTemplates(tempDir)
	require.NoError(t, err)
	assert.Contains(t, templates, "workflow_test (workflow_test)")
}
