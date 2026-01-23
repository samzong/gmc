// Package shell provides utilities for shell integration,
// including the Directive File pattern for parent shell manipulation.
package shell

import (
	"fmt"
	"os"
	"strings"
)

// DirectiveFileEnv is the environment variable name for the directive file path.
const DirectiveFileEnv = "GMC_DIRECTIVE_FILE"

// WriteDirective writes a shell command to the directive file.
// If the environment variable is not set, this is a no-op.
func WriteDirective(cmd string) error {
	path := os.Getenv(DirectiveFileEnv)
	if path == "" {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open directive file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(cmd + "\n"); err != nil {
		return fmt.Errorf("failed to write directive: %w", err)
	}

	return nil
}

// ChangeDirectory writes a cd command to the directive file.
func ChangeDirectory(path string) error {
	// Escape single quotes in path
	escaped := strings.ReplaceAll(path, "'", "'\"'\"'")
	return WriteDirective(fmt.Sprintf("cd '%s'", escaped))
}

func GenerateWrapper(shellType string) string {
	switch shellType {
	case "bash", "zsh":
		return posixWrapper
	case "fish":
		return fishWrapper
	default:
		return ""
	}
}

const posixWrapper = `# gmc shell integration
gmc() {
    local directive_file
    directive_file="$(mktemp)"
    
    GMC_DIRECTIVE_FILE="$directive_file" command gmc "$@"
    local exit_code=$?
    
    if [[ -s "$directive_file" ]]; then
        source "$directive_file"
    fi
    
    rm -f "$directive_file"
    return $exit_code
}
`

const fishWrapper = `# gmc shell integration
function gmc
    set -l directive_file (mktemp)
    
    GMC_DIRECTIVE_FILE="$directive_file" command gmc $argv
    set -l exit_code $status
    
    if test -s "$directive_file"
        source "$directive_file"
    end
    
    rm -f "$directive_file"
    return $exit_code
end
`
