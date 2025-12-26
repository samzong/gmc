//go:build tools

package tools

// Tool dependencies for documentation generation.
// This file exists to ensure go mod tidy keeps these dependencies.
import (
	_ "github.com/spf13/cobra/doc"
)
