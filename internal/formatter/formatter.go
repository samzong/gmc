package formatter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/samzong/gma/internal/config"
)

func BuildPrompt(role string, changedFiles []string, diff string) string {
	// Limit the content size of the diff to avoid exceeding the token limit.
	if len(diff) > 4000 {
		diff = diff[:4000] + "...(content is too long, truncated)"
	}

	changedFilesStr := strings.Join(changedFiles, "\n")

	cfg := config.GetConfig()
	templateName := cfg.PromptTemplate
	if templateName == "" {
		templateName = "default"
	}

	data := TemplateData{
		Role:  role,
		Files: changedFilesStr,
		Diff:  diff,
	}

	templateContent, err := GetPromptTemplate(templateName)
	if err != nil {
		fmt.Printf("Warning: %v, using default template\n", err)
		templateContent = builtinTemplates["default"]
	}

	prompt, err := RenderTemplate(templateContent, data)
	if err != nil {
		fmt.Printf("Warning: %v, using simple format\n", err)
		return fmt.Sprintf(`You are a professional %s, please generate a commit message that follows the Conventional Commits specification based on the following Git changes:

Changed Files:
%s

Changed Content:
%s

Please generate a commit message in the format of "type(scope): description".
The type should be the most appropriate from the following choices: feat, fix, docs, style, refactor, perf, test, chore.
The description should be concise (no more than 150 characters) and accurately reflect the changes.
Do not add issue numbers like "#123" or "(#123)" in the commit message, this will be handled automatically by the tool.`, role, changedFilesStr, diff)
	}

	return prompt
}

func FormatCommitMessage(message string) string {
	message = strings.TrimSpace(message)

	lines := strings.Split(message, "\n")
	if len(lines) > 0 {
		firstLine := lines[0]

		issuePattern := regexp.MustCompile(`\s*\(#\d+\)|\s*#\d+`)
		firstLine = issuePattern.ReplaceAllString(firstLine, "")

		conventionalPattern := regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|chore)(\([^\)]+\))?: .+`)
		if !conventionalPattern.MatchString(firstLine) {
			return formatToConventional(firstLine)
		}

		return firstLine
	}

	return message
}

func formatToConventional(message string) string {
	message = strings.TrimSpace(message)

	var commitType string

	lowerMsg := strings.ToLower(message)
	if strings.Contains(lowerMsg, "fix") || strings.Contains(lowerMsg, "bug") {
		commitType = "fix"
	} else if strings.Contains(lowerMsg, "add") || strings.Contains(lowerMsg, "feature") {
		commitType = "feat"
	} else if strings.Contains(lowerMsg, "doc") || strings.Contains(lowerMsg, "document") {
		commitType = "docs"
	} else if strings.Contains(lowerMsg, "style") || strings.Contains(lowerMsg, "style") {
		commitType = "style"
	} else if strings.Contains(lowerMsg, "refactor") || strings.Contains(lowerMsg, "refactor") {
		commitType = "refactor"
	} else if strings.Contains(lowerMsg, "perf") || strings.Contains(lowerMsg, "performance") {
		commitType = "perf"
	} else if strings.Contains(lowerMsg, "test") || strings.Contains(lowerMsg, "test") {
		commitType = "test"
	} else {
		commitType = "chore"
	}

	cleanMessage := message
	prefixes := []string{"fix:", "bug:", "feat:", "feature:", "docs:", "style:", "refactor:", "perf:", "test:", "chore:"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerMsg, prefix) {
			cleanMessage = message[len(prefix):]
			break
		}
	}

	cleanMessage = strings.TrimSpace(cleanMessage)

	return fmt.Sprintf("%s: %s", commitType, cleanMessage)
}
