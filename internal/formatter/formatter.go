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

var (
	// Pre-compiled regex patterns for performance
	issuePattern        *regexp.Regexp
	conventionalPattern *regexp.Regexp
	prefixPattern       *regexp.Regexp
)

func init() {
	// Compile regex patterns once at package initialization
	issuePattern = regexp.MustCompile(`\s*\(#\d+\)|\s*#\d+`)

	typePattern := emoji.GetCommitTypesRegexPattern()
	conventionalPattern = regexp.MustCompile(`(?i)^(?:[^\s]*\s)?(` + typePattern + `)(\([^\)]+\))?: (.+)`)
	prefixPattern = regexp.MustCompile(`(?i)^(` + typePattern + `):\s*(.+)`)
}

func BuildPrompt(role string, changedFiles []string, diff string, userPrompt string) string {
	// Limit the content size of the diff to avoid exceeding the token limit.
	if len(diff) > diffPromptLimit {
		diff = truncateToValidUTF8(diff, diffPromptLimit) + "...(content is too long, truncated)"
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
		templateContent = buildTemplateContent("default")
	}

	prompt, err := RenderTemplate(templateContent, data)
	if err != nil {
		fmt.Printf("Warning: %v, using simple format\n", err)
		cfg := config.GetConfig()
		formatMsg := `in the format of "type(scope): description"`
		emojiInstruction := ""
		if cfg.EnableEmoji {
			formatMsg = `in the format of "emoji type(scope): description"`
			emojiInstruction = fmt.Sprintf(
				"Add an appropriate emoji at the beginning based on the commit type (%s).\n",
				emoji.GetEmojiDescription(),
			)
		}
		prompt = fmt.Sprintf(`You are a professional %s, please generate a commit message that follows the `+
			`Conventional Commits specification based on the following Git changes:

Changed Files:
%s

Changed Content:
%s

Please generate a commit message %s.
The type should be the most appropriate from the following choices: %s.
%sThe description should be concise (no more than 150 characters) and accurately reflect the changes.
Do not add issue numbers like "#123" or "(#123)" in the commit message, `+
			`this will be handled automatically by the tool.`, role, changedFilesStr, diff,
			formatMsg, strings.Join(emoji.GetAllCommitTypes(), ", "), emojiInstruction)
	}

	// Append user prompt if provided
	if userPrompt != "" {
		prompt += "\n\nAdditional Context:\n" + userPrompt
	}

	return prompt
}

func FormatCommitMessage(message string) string {
	cfg := config.GetConfig()
	message = strings.TrimSpace(message)

	lines := strings.Split(message, "\n")
	if len(lines) > 0 {
		firstLine := lines[0]

		firstLine = issuePattern.ReplaceAllString(firstLine, "")

		// Check if message already follows conventional format (with or without emoji)
		// LLM should return messages in conventional format, so we just need to match and normalize
		matches := conventionalPattern.FindStringSubmatch(firstLine)
		if len(matches) >= 4 {
			// Normalize the commit type to lowercase
			commitType := strings.ToLower(matches[1])
			scope := matches[2]
			description := matches[3]
			// Reconstruct the message with normalized type
			firstLine = commitType + scope + ": " + description
		} else {
			// If not in conventional format, try to normalize type prefix if present
			// Otherwise, return as-is without adding any prefix
			firstLine = normalizeTypePrefix(firstLine)
		}

		// Add emoji if enabled and not already present
		if cfg.EnableEmoji {
			firstLine = emoji.AddEmojiToMessage(firstLine)
		}

		return firstLine
	}

	if cfg.EnableEmoji {
		return emoji.AddEmojiToMessage(message)
	}
	return message
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
