package formatter

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/emoji"
)

const diffPromptLimit = 4000

// DiffStatsSeparator separates diff content from optional stats block.
const DiffStatsSeparator = "-- gmc diff stats --"

var (
	// Pre-compiled regex patterns for performance
	issuePattern        *regexp.Regexp
	conventionalPattern *regexp.Regexp
	prefixPattern       *regexp.Regexp
	typePrefixPattern   *regexp.Regexp
)

func init() {
	// Compile regex patterns once at package initialization
	issuePattern = regexp.MustCompile(`\s*\(#\d+\)|\s*#\d+`)

	typePattern := emoji.GetCommitTypesRegexPattern()
	conventionalPattern = regexp.MustCompile(`(?i)^(?:[^\s]*\s)?(` + typePattern + `)(\([^\)]+\))?: (.+)`)
	prefixPattern = regexp.MustCompile(`(?i)^(` + typePattern + `):\s*(.+)`)
	typePrefixPattern = regexp.MustCompile(`(?i)^(` + typePattern + `)(\([^\)]+\))?:`)
}

func BuildPrompt(role string, changedFiles []string, diff string, userPrompt string) string {
	cfg := config.MustGetConfig()
	if role != "" {
		cfgCopy := *cfg
		cfgCopy.Role = role
		cfg = &cfgCopy
	}
	return BuildPromptWithConfig(cfg, changedFiles, diff, userPrompt)
}

func BuildPromptWithConfig(cfg *config.Config, changedFiles []string, diff string, userPrompt string) string {
	stats := ""
	if parts := strings.SplitN(diff, DiffStatsSeparator, 2); len(parts) == 2 {
		diff = strings.TrimRight(parts[0], "\n")
		stats = strings.TrimSpace(parts[1])
	}

	if len(diff) > diffPromptLimit {
		if stats == "" {
			diff = truncateToValidUTF8(diff, diffPromptLimit) + "...(content is too long, truncated)"
		} else {
			diff = truncateDiffWithStats(diff, stats, diffPromptLimit)
		}
	}

	changedFilesStr := strings.Join(changedFiles, "\n")

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

	templateContent, err := GetPromptTemplate(templateName)
	if err != nil {
		fmt.Printf("Warning: %v, using default template\n", err)
		templateContent = buildDefaultTemplateContent()
	}

	prompt, err := RenderTemplate(templateContent, data)
	if err != nil {
		fmt.Printf("Warning: %v, using simple format\n", err)
		prompt = buildSimplePromptWithConfig(cfg, role, changedFilesStr, diff)
	}

	if userPrompt != "" {
		prompt += "\n\nAdditional Context:\n" + userPrompt
	}

	return prompt
}

func FormatCommitMessage(message string) string {
	return FormatCommitMessageWithConfig(config.MustGetConfig(), message)
}

func FormatCommitMessageWithConfig(cfg *config.Config, message string) string {
	message = strings.TrimSpace(message)

	lines := strings.Split(message, "\n")
	if len(lines) > 0 {
		firstLine := lines[0]

		firstLine = issuePattern.ReplaceAllString(firstLine, "")
		firstLine = normalizeEmojiMissingType(firstLine)

		matches := conventionalPattern.FindStringSubmatch(firstLine)
		if len(matches) >= 4 {
			commitType := strings.ToLower(matches[1])
			scope := matches[2]
			description := matches[3]
			firstLine = commitType + scope + ": " + description
		} else {
			firstLine = normalizeTypePrefix(firstLine)
		}

		if cfg != nil && cfg.EnableEmoji {
			firstLine = emoji.AddEmojiToMessage(firstLine)
		}

		return firstLine
	}

	if cfg != nil && cfg.EnableEmoji {
		return emoji.AddEmojiToMessage(message)
	}
	return message
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
