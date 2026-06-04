package task

import (
	"errors"
	"fmt"
	"strings"
)

// ResolveTaskID maps a user reference to a full task id (exact or unique prefix).
func (s *Store) ResolveTaskID(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("task id is required")
	}
	ids, err := s.ListTaskIDs()
	if err != nil {
		return "", err
	}
	var matches []string
	for _, id := range ids {
		if id == ref {
			return id, nil
		}
		if strings.HasPrefix(id, ref) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: %s", ErrNotFound, ref)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous task id %q (matches: %s)", ref, strings.Join(matches, ", "))
	}
}
