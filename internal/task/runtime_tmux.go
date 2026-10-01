package task

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const gmcTmuxSocket = "gmc-task"

var tmuxSessionStarter = StartTmuxSession

var tmuxSessionAlive = tmuxHasSession

var tmuxReachable = tmuxAvailable

var tmuxSessionAttached = func(profile TmuxProfile) bool {
	args := append(tmuxBaseArgs(profile),
		"list-sessions", "-F", "#{session_name}\t#{session_attached}")
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		name, attached, ok := strings.Cut(line, "\t")
		if !ok || name != profile.Session {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(attached))
		return err == nil && n > 0
	}
	return false
}

var tmuxListSessions = func(socket string) ([]string, error) {
	if !tmuxAvailable() {
		return nil, nil
	}
	args := append(tmuxBaseArgs(TmuxProfile{Socket: socket}), "list-sessions", "-F", "#{session_name}")
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		return nil, nil
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

var killTmuxSession = KillTmuxSession

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
	args := append(tmuxBaseArgs(profile), "attach-session", "-t", tmuxTarget(profile.Session))
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func KillTmuxSession(profile TmuxProfile) error {
	if !tmuxAvailable() || !tmuxHasSession(profile) {
		return nil
	}
	args := append(tmuxBaseArgs(profile), "kill-session", "-t", tmuxTarget(profile.Session))
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
	args := append(tmuxBaseArgs(profile), "has-session", "-t", tmuxTarget(profile.Session))
	return exec.Command("tmux", args...).Run() == nil
}

func tmuxTarget(session string) string {
	return "=" + session
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
