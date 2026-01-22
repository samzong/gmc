package workflow

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/samzong/gmc/internal/config"
	"github.com/samzong/gmc/internal/formatter"
)

type Action int

const (
	ActionCommit Action = iota
	ActionCancel
	ActionRegenerate
)

type Prompter interface {
	GetConfirmation(message string, autoYes bool) (Action, string, error)
}

type InteractivePrompter struct {
	ErrWriter io.Writer
	Stdin     io.Reader
	Cfg       *config.Config
}

func (p *InteractivePrompter) GetConfirmation(message string, autoYes bool) (Action, string, error) {
	if autoYes {
		fmt.Fprintln(p.ErrWriter, "Auto-confirming commit message (-y flag is set)")
		return ActionCommit, "", nil
	}

	stdin := p.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	if f, ok := stdin.(*os.File); ok {
		if !isatty.IsTerminal(f.Fd()) && !isatty.IsCygwinTerminal(f.Fd()) {
			return ActionCancel, "", errors.New("stdin is not a terminal, use --yes to skip interactive confirmation")
		}
	}

	fmt.Fprint(p.ErrWriter,
		"\nDo you want to proceed with this commit message? [y/n/r/e] (y/n/r=regenerate/e=edit): ")
	reader := bufio.NewReader(stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return ActionCancel, "", fmt.Errorf("failed to read user input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	switch response {
	case "n":
		return ActionCancel, "", nil
	case "r":
		return ActionRegenerate, "", nil
	case "e":
		editedMessage, err := p.openEditor(message)
		return ActionCommit, editedMessage, err
	case "y", "":
		if response == "" {
			fmt.Fprintln(p.ErrWriter, "Using default option (yes)")
		}
		return ActionCommit, "", nil
	default:
		fmt.Fprintln(p.ErrWriter, "Invalid input. Commit cancelled")
		return ActionCancel, "", nil
	}
}

func (p *InteractivePrompter) openEditor(message string) (string, error) {
	fmt.Fprintln(p.ErrWriter, "Opening editor to modify commit message...")

	tmpFile, err := os.CreateTemp("", "gmc-commit-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}

	tmpFileName := tmpFile.Name()
	defer os.Remove(tmpFileName)

	if _, err := tmpFile.WriteString(message); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write to temporary file: %w", err)
	}
	tmpFile.Close()

	editor := getEditor()
	cmd := exec.Command(editor, tmpFileName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to open editor: %w", err)
	}

	editedBytes, err := os.ReadFile(tmpFileName)
	if err != nil {
		return "", fmt.Errorf("failed to read edited message: %w", err)
	}

	editedMessage := strings.TrimSpace(string(editedBytes))
	if editedMessage != "" {
		formattedMessage := formatter.FormatCommitMessageWithConfig(p.Cfg, editedMessage)
		fmt.Fprintln(p.ErrWriter, "Using edited message:")
		fmt.Fprintln(p.ErrWriter, formattedMessage)
		return formattedMessage, nil
	}

	fmt.Fprintln(p.ErrWriter, "Empty message provided, using original message")
	return "", nil
}

func getEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	return "vi"
}
