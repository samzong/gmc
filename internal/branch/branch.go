package branch

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	specialCharsRegex    = regexp.MustCompile(`[^a-zA-Z0-9\s\-]+`)
	multipleSpacesRegex  = regexp.MustCompile(`\s+`)
	multipleHyphensRegex = regexp.MustCompile(`-+`)
)

var prefixKeywords = map[string][]string{
	"feature": {"add", "create", "implement", "new", "support", "feature"},
	"fix":     {"fix", "resolve", "correct", "repair", "patch", "bug"},
	"docs":    {"document", "readme", "guide", "manual", "docs"},
}

func GenerateName(description string) string {
	if description == "" {
		return ""
	}

	prefix := detectPrefix(description)
	cleaned := sanitizeDescription(description)
	if cleaned == "" {
		return ""
	}
	cleaned = limitLength(cleaned, 45)

	return fmt.Sprintf("%s/%s", prefix, cleaned)
}

func detectPrefix(description string) string {
	lowerDesc := strings.ToLower(description)

	for prefix, keywords := range prefixKeywords {
		if containsAnyKeyword(lowerDesc, keywords) {
			return prefix
		}
	}

	return "chore" // default prefix
}

func containsAnyKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func sanitizeDescription(description string) string {
	cleaned := specialCharsRegex.ReplaceAllString(description, "")
	cleaned = strings.ToLower(strings.TrimSpace(cleaned))
	cleaned = multipleSpacesRegex.ReplaceAllString(cleaned, "-")
	cleaned = multipleHyphensRegex.ReplaceAllString(cleaned, "-")
	return strings.Trim(cleaned, "-")
}

func limitLength(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}

	truncated := text[:maxLength]
	return strings.Trim(truncated, "-")
}
