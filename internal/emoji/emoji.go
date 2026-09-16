package emoji

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var typeEmoji = map[string]string{
	"feat":      "✨",
	"fix":       "🐛",
	"docs":      "📝",
	"style":     "💄",
	"refactor":  "♻️",
	"perf":      "⚡️",
	"test":      "✅",
	"build":     "👷",
	"ci":        "💚",
	"chore":     "🔧",
	"revert":    "⏪️",
	"deps":      "⬆️",
	"security":  "🔒️",
	"hotfix":    "🚑️",
	"release":   "🚀",
	"wip":       "🚧",
	"init":      "🎉",
	"breaking":  "💥",
	"config":    "🔧",
	"i18n":      "🌐",
	"typo":      "✏️",
	"merge":     "🔀",
	"move":      "🚚",
	"remove":    "🔥",
	"add":       "➕",
	"upgrade":   "⬆️",
	"downgrade": "⬇️",
	"other":     "🔧",
}

var commitTypeRegex = regexp.MustCompile(`^([a-zA-Z]+)(?:\([^)]+\))?:`)

func GetEmojiForType(commitType string) string {
	return typeEmoji[strings.ToLower(commitType)]
}

func GetAllCommitTypes() []string {
	return slices.Sorted(maps.Keys(typeEmoji))
}

func GetEmojiDescription() string {
	types := GetAllCommitTypes()
	parts := make([]string, 0, len(types))
	for _, t := range types {
		parts = append(parts, typeEmoji[t]+" for "+t)
	}
	return strings.Join(parts, ", ")
}

func InferTypeFromEmojiPrefix(message string) (string, string) {
	message = strings.TrimSpace(message)
	for commitType, emoji := range typeEmoji {
		if after, found := strings.CutPrefix(message, emoji); found {
			return commitType, strings.TrimSpace(after)
		}
	}
	return "", ""
}

func AddEmojiToMessage(message string) string {
	message = strings.TrimSpace(message)
	first, _ := utf8.DecodeRuneInString(message)
	if isEmoji(first) {
		return message
	}
	if emoji := GetEmojiForType(extractCommitType(message)); emoji != "" {
		return emoji + " " + message
	}
	return message
}

func extractCommitType(message string) string {
	matches := commitTypeRegex.FindStringSubmatch(message)
	if len(matches) >= 2 {
		return strings.ToLower(matches[1])
	}
	return ""
}

func isEmoji(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1F9FF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r == 0x200D) ||
		(r >= 0x203C && r <= 0x3299)
}
