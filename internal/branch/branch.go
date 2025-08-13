package branch

import (
	"fmt"
	"regexp"
	"strings"
)

// Pre-compiled regex patterns for efficiency
var (
	specialCharsRegex    = regexp.MustCompile(`[^a-zA-Z0-9\s\-]+`)
	multipleSpacesRegex  = regexp.MustCompile(`\s+`)
	multipleHyphensRegex = regexp.MustCompile(`-+`)
)

// Keywords for branch prefix detection
var prefixKeywords = map[string][]string{
	"feature": {"add", "create", "implement", "new", "support", "feature"},
	"fix":     {"fix", "resolve", "correct", "repair", "patch", "bug"},
	"docs":    {"document", "readme", "guide", "manual", "docs"},
}

// GenerateName creates a branch name from the description with appropriate prefix
func GenerateName(description string) string {
	if description == "" {
		return ""
	}

	// Determine prefix based on keywords
	prefix := detectPrefix(description)

	// Sanitize and format the description
	cleaned := sanitizeDescription(description)
	if cleaned == "" {
		return ""
	}

	// Limit length to 45 characters to leave room for prefix
	cleaned = limitLength(cleaned, 45)

	return fmt.Sprintf("%s/%s", prefix, cleaned)
}

// detectPrefix determines the appropriate prefix based on keywords in the description
func detectPrefix(description string) string {
	lowerDesc := strings.ToLower(description)

	for prefix, keywords := range prefixKeywords {
		if containsAnyKeyword(lowerDesc, keywords) {
			return prefix
		}
	}

	return "chore" // default prefix
}

// containsAnyKeyword checks if the text contains any of the specified keywords
func containsAnyKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// sanitizeDescription cleans and formats the description for use in branch names
func sanitizeDescription(description string) string {
	// Remove special characters except letters, numbers, spaces, and hyphens
	cleaned := specialCharsRegex.ReplaceAllString(description, "")

	// Convert to lowercase and trim spaces
	cleaned = strings.ToLower(strings.TrimSpace(cleaned))

	// Replace multiple spaces with single hyphen
	cleaned = multipleSpacesRegex.ReplaceAllString(cleaned, "-")

	// Remove multiple consecutive hyphens
	cleaned = multipleHyphensRegex.ReplaceAllString(cleaned, "-")

	// Remove leading and trailing hyphens
	return strings.Trim(cleaned, "-")
}

// limitLength truncates the string to maxLength while preserving word boundaries
func limitLength(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}

	truncated := text[:maxLength]
	return strings.Trim(truncated, "-")
}
