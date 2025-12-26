package gitutil

import (
	"fmt"
	"strings"

	"github.com/samzong/gmc/internal/gitcmd"
)

// WrapGitError builds an error message that prefers git stderr output when present.
func WrapGitError(action string, result gitcmd.Result, err error) error {
	errMsg := strings.TrimSpace(string(result.Stderr))
	if errMsg != "" {
		return fmt.Errorf("%s: %s: %w", action, errMsg, err)
	}
	return fmt.Errorf("%s: %w", action, err)
}
