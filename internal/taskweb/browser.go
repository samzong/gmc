package taskweb

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func DefaultBrowserOpener(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Run()
	case "linux":
		return exec.Command("xdg-open", url).Run()
	default:
		return nil
	}
}

func SuggestedTerminal() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TERM_PROGRAM"))) {
	case "ghostty":
		return "ghostty"
	case "iterm.app", "iterm":
		return "iterm"
	case "apple_terminal":
		return "terminal"
	default:
		return "ghostty"
	}
}
