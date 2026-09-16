package formatter

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/emoji"
)

const diffPromptLimit = 4000
const maxPromptFiles = 200

func BuildPromptWithConfig(cfg *config.Config, changedFiles []string, diff, stats, userPrompt string) string {
	if len(diff) > diffPromptLimit {
		if stats == "" {
			diff = truncateToValidUTF8(diff, diffPromptLimit) + "...(content is too long, truncated)"
		} else {
			diff = truncateDiffWithStats(diff, stats, diffPromptLimit)
		}
	}

	changedFilesStr := formatChangedFiles(changedFiles)

	role := ""
	templateName := "default"
	if cfg != nil {
		role = cfg.Role
		if cfg.PromptTemplate != "" {
			templateName = cfg.PromptTemplate
		}
	}

	data := TemplateData{
		Role:  role,
		Files: changedFilesStr,
		Diff:  diff,
	}

	var templateContent string
	if templateName == "" || templateName == config.DefaultPromptTemplate {
		templateContent = buildDefaultTemplateContentWith(cfg)
	} else if content, err := GetPromptTemplate(templateName); err != nil {
		warnf("%v, using default template", err)
		templateContent = buildDefaultTemplateContentWith(cfg)
	} else {
		templateContent = content
	}

	prompt, err := RenderTemplate(templateContent, data)
	if err != nil {
		warnf("%v, using simple format", err)
		prompt = buildSimplePromptWithConfig(cfg, role, changedFilesStr, diff)
	}

	if userPrompt != "" {
		prompt += "\n\nAdditional Context:\n" + userPrompt
	}

	return prompt
}

func formatChangedFiles(changedFiles []string) string {
	if len(changedFiles) <= maxPromptFiles {
		return strings.Join(changedFiles, "\n")
	}

	listed := strings.Join(changedFiles[:maxPromptFiles], "\n")
	return fmt.Sprintf("%s\n... and %d more files", listed, len(changedFiles)-maxPromptFiles)
}

func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}

func truncateToValidUTF8(input string, maxBytes int) string {
	if len(input) <= maxBytes {
		return input
	}

	end := maxBytes
	for end > 0 && !utf8.ValidString(input[:end]) {
		end--
	}

	return input[:end]
}

func buildSimplePromptWithConfig(cfg *config.Config, role, changedFilesStr, diff string) string {
	enableEmoji := cfg != nil && cfg.EnableEmoji

	typeInstruction := `Use the "type(scope): description" syntax`
	if enableEmoji {
		typeInstruction = `Use the "emoji type(scope): description" syntax`
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "%s, summarize the following git changes as a single Conventional Commits line.\n\n", role)
	fmt.Fprintf(&builder, "Files:\n%s\n\n", changedFilesStr)
	fmt.Fprintf(&builder, "Diff:\n%s\n\n", diff)
	fmt.Fprintf(&builder, "%s and pick the most relevant type from: %s.\n",
		typeInstruction, strings.Join(emoji.GetAllCommitTypes(), ", "))
	if enableEmoji {
		fmt.Fprintf(&builder, "Start with an emoji that matches the type (%s).\n", emoji.GetEmojiDescription())
	}
	builder.WriteString("Keep it under 150 characters and skip issue references; gmc adds them automatically.")

	return builder.String()
}
