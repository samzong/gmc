package stringsutil

import "strings"

// SplitNonEmpty splits s by sep and returns only non-empty parts.
func SplitNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ShortHash truncates a hex hash to n characters. Returns fallback if hash is empty.
func ShortHash(hash string, n int, fallback string) string {
	if hash == "" {
		return fallback
	}
	if len(hash) > n {
		return hash[:n]
	}
	return hash
}

// UniqueStrings returns a new slice with duplicates removed, preserving first-seen order.
func UniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
