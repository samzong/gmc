package formatter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func templateFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestGetPromptTemplate_Builtin(t *testing.T) {
	for _, name := range []string{"", "default"} {
		t.Run(name, func(t *testing.T) {
			result, err := GetPromptTemplate(name)
			require.NoError(t, err)
			assert.NotEmpty(t, result)
			assert.Contains(t, result, "{{.Role}}")
			assert.Contains(t, result, "{{.Files}}")
			assert.Contains(t, result, "{{.Diff}}")
		})
	}
}

func TestGetPromptTemplate_File(t *testing.T) {
	path := templateFile(t, `name: "test"
description: "Test template"
template: |
  Test template for {{.Role}}.
  Files: {{.Files}}
  Changes: {{.Diff}}`)
	result, err := GetPromptTemplate(path)
	require.NoError(t, err)
	assert.Equal(t, "Test template for {{.Role}}.\nFiles: {{.Files}}\nChanges: {{.Diff}}", result)
	for _, content := range []string{
		"Simple text template with {{.Role}} and {{.Files}}",
		"invalid yaml: [\nbut still {{.Role}} template content",
	} {
		t.Run(content, func(t *testing.T) {
			result, err := GetPromptTemplate(templateFile(t, content))
			require.NoError(t, err)
			assert.Equal(t, content, result)
		})
	}
}

func TestGetPromptTemplate_FileReadError(t *testing.T) {
	for _, path := range []string{"nonexistent", "/non/existent/path/template.yaml"} {
		result, err := GetPromptTemplate(path)
		require.ErrorContains(t, err, "prompt template file not found")
		assert.Empty(t, result)
	}
	restrictedFile := templateFile(t, "test content")
	if err := os.Chmod(restrictedFile, 0); err != nil {
		t.Skipf("cannot restrict template permissions: %v", err)
	}
	t.Cleanup(func() { require.NoError(t, os.Chmod(restrictedFile, 0o644)) })
	result, err := GetPromptTemplate(restrictedFile)
	if err == nil {
		t.Skip("current user can read permission-restricted files")
	}
	require.ErrorContains(t, err, "unable to read template file")
	assert.Empty(t, result)
}

func TestRenderTemplate(t *testing.T) {
	for _, tt := range []struct {
		template string
		data     TemplateData
		expected string
	}{
		{"Role: {{.Role}}, Files: {{.Files}}, Diff: {{.Diff}}",
			TemplateData{Role: "Senior Go Developer", Files: "main.go\nconfig.go", Diff: "+added line\n-removed line"},
			"Role: Senior Go Developer, Files: main.go\nconfig.go, Diff: +added line\n-removed line"},
		{"", TemplateData{}, ""},
		{"{{if .Role}}Role: {{.Role}}{{end}}{{if .Files}} Files: {{.Files}}{{end}}",
			TemplateData{Role: "Developer"}, "Role: Developer"},
	} {
		t.Run(tt.template, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.data)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
	for _, template := range []string{"Role: {{.Role}}, Unknown: {{.Unknown}}", "{{.Role"} {
		t.Run(template, func(t *testing.T) {
			_, err := RenderTemplate(template, TemplateData{Role: "Developer"})
			require.Error(t, err)
		})
	}
}

func TestTemplateWorkflow_EndToEnd(t *testing.T) {
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

	retrievedTemplate, err := GetPromptTemplate(templateFile(t, templateContent))
	require.NoError(t, err)
	assert.Contains(t, retrievedTemplate, "=== Commit Message Generation ===")

	data := TemplateData{
		Role:  "Senior Go Developer",
		Files: "internal/formatter/formatter.go\ninternal/formatter/template.go",
		Diff:  "diff --git a/internal/formatter/formatter.go b/internal/formatter/formatter.go\n+func NewFunction() {}",
	}

	renderedResult, err := RenderTemplate(retrievedTemplate, data)
	require.NoError(t, err)

	assert.Contains(t, renderedResult, "=== Commit Message Generation ===")
	assert.Contains(t, renderedResult, "Senior Go Developer")
	assert.Contains(t, renderedResult, "internal/formatter/formatter.go")
	assert.Contains(t, renderedResult, "internal/formatter/template.go")
	assert.Contains(t, renderedResult, "+func NewFunction() {}")
}
