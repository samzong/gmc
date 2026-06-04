package task

import "strings"

// ParseSessionTarget maps CLI --session to a tmux target.
func ParseSessionTarget(s string) SessionTarget {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "review":
		return SessionReview
	default:
		return SessionCoding
	}
}
