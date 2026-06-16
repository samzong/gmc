package task

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const gmcTmuxSocket = "gmc-task"

var tmuxSessionStarter = StartTmuxSession

type TmuxProfile struct {
	Session string
	Socket  string
}

func StartTmuxSession(session, workdir string, command []string) (TmuxProfile, error) {
	if !tmuxAvailable() {
		return TmuxProfile{}, errors.New("tmux not found in PATH")
	}
	if len(command) == 0 {
		return TmuxProfile{}, errors.New("empty command")
	}
	profile := TmuxProfile{Session: session, Socket: gmcTmuxSocket}
	if tmuxHasSession(profile) {
		_ = KillTmuxSession(profile)
	}
	args := tmuxBaseArgs(profile)
	args = append(args, "new-session", "-d", "-s", session, "-c", workdir, shellJoin(command))
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return TmuxProfile{}, fmt.Errorf("tmux new-session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return profile, nil
}

func AttachTmuxSession(profile TmuxProfile) error {
	if !tmuxAvailable() {
		return errors.New("tmux not found in PATH")
	}
	if !tmuxHasSession(profile) {
		return fmt.Errorf("tmux session %q not found", profile.Session)
	}
	args := append(tmuxBaseArgs(profile), "attach-session", "-t", profile.Session)
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func SendTmuxPrompt(profile TmuxProfile, prompt string) error {
	if !tmuxAvailable() {
		return errors.New("tmux not found in PATH")
	}
	if !tmuxHasSession(profile) {
		return fmt.Errorf("tmux session %q not found", profile.Session)
	}
	buffer := "gmc-task-prompt"
	loadArgs := append(tmuxBaseArgs(profile), "load-buffer", "-b", buffer, "-")
	loadCmd := exec.Command("tmux", loadArgs...)
	loadCmd.Stdin = strings.NewReader(prompt)
	if out, err := loadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux load-buffer: %w: %s", err, strings.TrimSpace(string(out)))
	}
	pasteArgs := append(tmuxBaseArgs(profile), "paste-buffer", "-d", "-b", buffer, "-t", profile.Session)
	if out, err := exec.Command("tmux", pasteArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux paste-buffer: %w: %s", err, strings.TrimSpace(string(out)))
	}
	enterArgs := append(tmuxBaseArgs(profile), "send-keys", "-t", profile.Session, "Enter")
	if out, err := exec.Command("tmux", enterArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func KillTmuxSession(profile TmuxProfile) error {
	if !tmuxAvailable() || !tmuxHasSession(profile) {
		return nil
	}
	args := append(tmuxBaseArgs(profile), "kill-session", "-t", profile.Session)
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux kill-session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func tmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func tmuxHasSession(profile TmuxProfile) bool {
	args := append(tmuxBaseArgs(profile), "has-session", "-t", profile.Session)
	return exec.Command("tmux", args...).Run() == nil
}

func tmuxBaseArgs(profile TmuxProfile) []string {
	if profile.Socket == "" {
		return nil
	}
	return []string{"-L", profile.Socket}
}

func shellJoin(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, arg := range argv {
		if arg == "" {
			parts = append(parts, "''")
			continue
		}
		if strings.ContainsAny(arg, " \t\"'$\\") {
			parts = append(parts, fmt.Sprintf("%q", arg))
			continue
		}
		parts = append(parts, arg)
	}
	return strings.Join(parts, " ")
}
