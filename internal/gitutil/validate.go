package gitutil

import (
	"errors"
	"fmt"
	"strings"
)

// ValidateBranchName validates a git branch name for common illegal patterns.
func ValidateBranchName(name string) error {
	if name == "" {
		return errors.New("branch name cannot be empty")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("branch name cannot start with '-': %s", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("branch name cannot contain '..': %s", name)
	}
	for _, ch := range []string{" ", "~", "^", ":", "?", "*", "["} {
		if strings.Contains(name, ch) {
			return fmt.Errorf("branch name contains invalid character %q: %s", ch, name)
		}
	}
	return nil
}
