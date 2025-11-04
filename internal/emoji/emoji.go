package emoji

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// emojiMap maps commit types to their corresponding emojis
var emojiMap = map[string]string{
	"feat":     "✨",  // sparkles - new feature
	"fix":      "🐛",  // bug - bug fix
	"docs":     "📝",  // memo - documentation
	"style":    "💄",  // lipstick - code style
	"refactor": "♻️", // recycle - refactoring
	"perf":     "⚡",  // zap - performance
	"test":     "✅",  // white_check_mark - tests
	"chore":    "🔧",  // wrench - maintenance
	"build":    "🏗️", // hammer - build system
	"ci":       "🤖",  // robot - continuous integration
	"revert":   "🔙",  // back - revert a commit
	"release":  "🚀",  // rocket - release a new version
	"hotfix":   "🔥",  // fire - hotfix a critical issue
	"security": "🔒",  // lock - security fix
	"other":    "🔧",  // wrench - other changes
	"wip":      "🚧",  // construction - work in progress
	"deps":     "🔗",  // link - dependency update
}

// Pre-compiled regex pattern for extracting commit type
var commitTypeRegex = regexp.MustCompile(`^([a-zA-Z]+)(?:\([^)]+\))?:`)

// GetEmojiForType returns the emoji for a given commit type.
// Returns empty string if type is not recognized.
func GetEmojiForType(commitType string) string {
	if emoji, ok := emojiMap[strings.ToLower(commitType)]; ok {
		return emoji
	}
	return ""
}

// GetAllCommitTypes returns all supported commit types in alphabetical order.
func GetAllCommitTypes() []string {
	types := make([]string, 0, len(emojiMap))
	for t := range emojiMap {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// GetEmojiDescription returns a formatted string listing all commit types with their emojis.
// Format: "emoji for type, emoji for type, ..."
func GetEmojiDescription() string {
	types := GetAllCommitTypes()
	parts := make([]string, 0, len(types))
	for _, t := range types {
		emoji := emojiMap[t]
		parts = append(parts, fmt.Sprintf("%s for %s", emoji, t))
	}
	return strings.Join(parts, ", ")
}

// GetCommitTypesRegexPattern returns a regex pattern string that matches all commit types.
// Useful for building regex patterns dynamically.
func GetCommitTypesRegexPattern() string {
	types := GetAllCommitTypes()
	// Types are already valid identifiers, so we can use them directly
	return strings.Join(types, "|")
}

// AddEmojiToMessage adds an emoji prefix to a commit message based on its type.
// It parses the commit message format "type(scope): description" and prepends
// the appropriate emoji. If the message already has an emoji, it returns
// the message unchanged. If the type is not recognized, returns the message
// unchanged.
func AddEmojiToMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}

	// Check if message already starts with an emoji (common emoji ranges)
	firstRune := []rune(message)[0]
	if isEmoji(firstRune) {
		// Already has emoji, return as is
		return message
	}

	// Parse commit type from format "type(scope): description" or "type: description"
	commitType := extractCommitType(message)
	if commitType == "" {
		// Could not parse type, return message unchanged
		return message
	}

	emoji := GetEmojiForType(commitType)
	if emoji == "" {
		// No emoji for this type, return message unchanged
		return message
	}

	// Prepend emoji with a space
	return emoji + " " + message
}

// extractCommitType extracts the commit type from a commit message.
// Supports formats: "type:" or "type(scope):"
func extractCommitType(message string) string {
	// Pattern matches: type(scope): or type:
	matches := commitTypeRegex.FindStringSubmatch(message)
	if len(matches) >= 2 {
		return strings.ToLower(matches[1])
	}
	return ""
}

// isEmoji checks if a rune is likely an emoji character.
// This covers most common emoji ranges in Unicode.
func isEmoji(r rune) bool {
	// Check various emoji ranges:
	// U+1F000-U+1F9FF: Miscellaneous Symbols and Pictographs (includes U+1F300-U+1F9FF)
	// U+2600-U+26FF: Miscellaneous Symbols
	// U+2700-U+27BF: Dingbats
	// U+FE00-FE0F: Variation Selectors
	// U+200D: Zero Width Joiner
	// U+203C-U+3299: Various symbols
	return (r >= 0x1F000 && r <= 0x1F9FF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r == 0x200D) ||
		(r >= 0x203C && r <= 0x3299)
}
