package taskweb

import (
	"fmt"
	"strings"
)

func composeShellCommand(workdir, command string) string {
	workdir = strings.TrimSpace(workdir)
	command = strings.TrimSpace(command)
	if workdir == "" {
		return command
	}
	if command == "" {
		return "cd " + shellQuote(workdir)
	}
	return "cd " + shellQuote(workdir) + " && " + command
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func attachCLICommand(gmcBinary, taskID string) string {
	return strings.Join([]string{
		shellQuote(strings.TrimSpace(gmcBinary)),
		"task",
		"attach",
		shellQuote(strings.TrimSpace(taskID)),
	}, " ")
}

func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func ghosttyScript(workdir, command string) string {
	var b strings.Builder
	b.WriteString("tell application \"Ghostty\"\nactivate\n")
	if wd := strings.TrimSpace(workdir); wd != "" {
		b.WriteString("set cfg to new surface configuration\n")
		b.WriteString("set initial working directory of cfg to ")
		b.WriteString(appleScriptString(wd))
		b.WriteString("\n")
		b.WriteString("set win to new window with configuration cfg\n")
	} else {
		b.WriteString("set win to new window\n")
	}
	b.WriteString("set t to focused terminal of selected tab of win\n")
	b.WriteString("input text ")
	b.WriteString(appleScriptString(strings.TrimSpace(command)))
	b.WriteString(" & return to t\nend tell")
	return b.String()
}

func itermScript(shellCmd string) string {
	return fmt.Sprintf(`tell application "iTerm"
	activate
	set newWindow to (create window with default profile)
	tell current session of newWindow
		write text %s
	end tell
end tell`, appleScriptString(shellCmd))
}

func terminalAppScript(shellCmd string) string {
	return fmt.Sprintf(`tell application "Terminal"
	do script %s
	activate
end tell`, appleScriptString(shellCmd))
}
