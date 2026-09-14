package formatter

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/emoji"
)

const diffPromptLimit = 4000

const maxPromptFiles = 200

var (
	issuePattern        *regexp.Regexp
	conventionalPattern *regexp.Regexp
	prefixPattern       *regexp.Regexp
	typePrefixPattern   *regexp.Regexp
	preamblePattern     *regexp.Regexp
	wrapperLabelPattern *regexp.Regexp
)

func init() {
	issuePattern = regexp.MustCompile(`\s*\(#\d+\)|\s*#\d+`)

	typePattern := emoji.GetCommitTypesRegexPattern()
	conventionalPattern = regexp.MustCompile(`(?i)^(?:[^\s]*\s)?(` + typePattern + `)(\([^\)]+\))?: (.+)`)
	prefixPattern = regexp.MustCompile(`(?i)^(` + typePattern + `):\s*(.+)`)
	typePrefixPattern = regexp.MustCompile(`(?i)^(` + typePattern + `)(\([^\)]+\))?:`)

	preamblePattern = regexp.MustCompile(
		`(?i)^(?:sure|ok(?:ay)?|certainly|here(?:'s| is| are)?|this is|the commit message|commit message|message|output|result|suggestion)\b[^:\n]*:`)
	wrapperLabelPattern = regexp.MustCompile(
		`(?i)^(?:here(?:'s| is)?\s+(?:your\s+)?)?(?:(?:the|proposed|suggested|my)\s+)?` +
			`(?:commit\s+message|commit|message|output|result|suggestion)s?$`)
}

func BuildPrompt(role string, changedFiles []string, diff string, userPrompt string) string {
	cfg := config.MustGetConfig()
	if role != "" {
		cfgCopy := *cfg
		cfgCopy.Role = role
		cfg = &cfgCopy
	}
	return BuildPromptWithConfig(cfg, changedFiles, diff, "", userPrompt)
}

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

func FormatCommitMessage(message string) string {
	return FormatCommitMessageWithConfig(config.MustGetConfig(), message)
}

func FormatCommitMessageWithConfig(cfg *config.Config, message string) string {
	message = SanitizeCommitMessage(message)
	if message == "" {
		return ""
	}

	message = issuePattern.ReplaceAllString(message, "")
	message = normalizeEmojiMissingType(message)

	if matches := conventionalPattern.FindStringSubmatch(message); len(matches) >= 4 {
		commitType := strings.ToLower(matches[1])
		scope := matches[2]
		description := matches[3]
		message = commitType + scope + ": " + description
	} else {
		message = normalizeTypePrefix(message)
	}

	if cfg != nil && cfg.EnableEmoji {
		message = emoji.AddEmojiToMessage(message)
	}
	return message
}

func SanitizeCommitMessage(message string) string {
	lines := strings.Split(strings.TrimSpace(message), "\n")

	inFence := false
	fenceClosed := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if fenceClosed {
			continue
		}

		content, isFence := stripFence(line)
		if isFence {
			if content != "" {
				if candidate, ok := unwrapWrapper(content); ok {
					return candidate
				}
				continue
			}
			if inFence {
				inFence = false
				fenceClosed = true
			} else {
				inFence = true
			}
			continue
		}

		if inFence {
			line = trimFenceSuffix(line)
			if line == "" {
				continue
			}
			if candidate, ok := unwrapWrapper(line); ok {
				return candidate
			}
			continue
		}

		if candidate, ok := unwrapWrapper(line); ok {
			return candidate
		}
	}

	return ""
}

func unwrapWrapper(line string) (string, bool) {
	line = trimDecoration(line)
	if line == "" || wrapperLabelPattern.MatchString(line) {
		return "", false
	}

	rest, isPreamble := trimPreamble(line)
	if !isPreamble {
		return line, true
	}

	rest = trimDecoration(rest)
	if rest == "" || wrapperLabelPattern.MatchString(rest) {
		return "", false
	}
	return rest, true
}

func stripFence(line string) (string, bool) {
	for _, marker := range []string{"```", "~~~"} {
		if !strings.HasPrefix(line, marker) {
			continue
		}

		rest := line[len(marker):]
		if end := strings.Index(rest, marker); end >= 0 {
			return strings.TrimSpace(rest[:end]), true
		}
		return "", true
	}
	return line, false
}

func trimFenceSuffix(line string) string {
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasSuffix(line, marker) {
			return strings.TrimSpace(strings.TrimSuffix(line, marker))
		}
	}
	return line
}

func trimDecoration(line string) string {
	line = strings.TrimSpace(line)

	decorated := false
	for {
		trimmed := strings.TrimLeft(line, "#>-*_ \t")
		if trimmed == line {
			break
		}
		decorated = true
		line = trimmed
	}
	if decorated {
		line = strings.TrimRight(line, "*_")
	}

	return strings.TrimSpace(line)
}

func trimPreamble(line string) (string, bool) {
	loc := preamblePattern.FindStringIndex(line)
	if loc == nil {
		return line, false
	}
	return strings.TrimSpace(line[loc[1]:]), true
}

func normalizeEmojiMissingType(message string) string {
	commitType, rest := emoji.InferTypeFromEmojiPrefix(message)
	if commitType == "" || rest == "" {
		return message
	}

	if typePrefixPattern.MatchString(rest) {
		return message
	}

	if strings.HasPrefix(rest, "(") || strings.HasPrefix(rest, ":") {
		return commitType + rest
	}

	return commitType + ": " + rest
}

// normalizeTypePrefix normalizes the type prefix if present, otherwise returns the message as-is.
func normalizeTypePrefix(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}

	// Check if message starts with a known type prefix (case-insensitive)
	matches := prefixPattern.FindStringSubmatch(message)
	if len(matches) >= 3 {
		// Normalize type and return
		commitType := strings.ToLower(matches[1])
		description := strings.TrimSpace(matches[2])
		return commitType + ": " + description
	}

	// If no type prefix found, return as-is
	return message
}

func truncateToValidUTF8(input string, maxBytes int) string {
	if len(input) <= maxBytes {
		return input
	}

	end := maxBytes
	for end > 0 && !utf8.ValidString(input[:end]) {
		end--
	}

	if end == 0 {
		return ""
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
