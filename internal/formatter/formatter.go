package formatter

import (
	"regexp"
	"strings"

	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/emoji"
)

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

	typePattern := strings.Join(emoji.GetAllCommitTypes(), "|")
	conventionalPattern = regexp.MustCompile(`(?i)^(?:[^\s]*\s)?(` + typePattern + `)(\([^\)]+\))?: (.+)`)
	prefixPattern = regexp.MustCompile(`(?i)^(` + typePattern + `):\s*(.+)`)
	typePrefixPattern = regexp.MustCompile(`(?i)^(` + typePattern + `)(\([^\)]+\))?:`)

	preamblePattern = regexp.MustCompile(
		`(?i)^(?:sure|ok(?:ay)?|certainly|here(?:'s| is| are)?|this is|the commit message|` +
			`commit message|message|output|result|suggestion)\b[^:\n]*:`)
	wrapperLabelPattern = regexp.MustCompile(
		`(?i)^(?:here(?:'s| is)?\s+(?:your\s+)?)?(?:(?:the|proposed|suggested|my)\s+)?` +
			`(?:commit\s+message|commit|message|output|result|suggestion)s?$`)
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
	inFence := false
	for _, raw := range strings.Split(strings.TrimSpace(message), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		content, isFence := stripFence(line)
		switch {
		case isFence && content == "":
			if inFence {
				return ""
			}
			inFence = true
			continue
		case isFence:
			line = content
		case inFence:
			line = trimFenceSuffix(line)
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
	trimmed := strings.TrimLeft(line, "#>-*_ \t")
	if trimmed != line {
		trimmed = strings.TrimRight(trimmed, "*_")
	}
	return strings.TrimSpace(trimmed)
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

func normalizeTypePrefix(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}

	matches := prefixPattern.FindStringSubmatch(message)
	if len(matches) >= 3 {
		commitType := strings.ToLower(matches[1])
		description := strings.TrimSpace(matches[2])
		return commitType + ": " + description
	}

	return message
}
