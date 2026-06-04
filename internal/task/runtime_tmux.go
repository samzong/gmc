package task

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	_ "embed" // required for tmuxGmcConf
)

//go:embed tmux-gmc.conf
var tmuxGmcConf []byte

const gmcTmuxSocket = "gmc"

var (
	tmuxConfigOnce sync.Once
	tmuxConfigPath string
	tmuxConfigErr  error
)

// TmuxProfile selects how to talk to a tmux server.
type TmuxProfile struct {
	Session string
	Socket  string // empty = default tmux server (legacy)
	Config  string // optional -f path (used with gmc socket)
}

func gmcTmuxConfigFile() (string, error) {
	tmuxConfigOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), "gmc-tmux")
		tmuxConfigPath = filepath.Join(dir, "tmux.conf")
		tmuxConfigErr = os.MkdirAll(dir, 0o755)
		if tmuxConfigErr == nil {
			tmuxConfigErr = os.WriteFile(tmuxConfigPath, tmuxGmcConf, 0o644)
		}
	})
	return tmuxConfigPath, tmuxConfigErr
}

func tmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func tmuxBaseArgs(profile TmuxProfile) ([]string, error) {
	args := []string{}
	if profile.Socket != "" {
		args = append(args, "-L", profile.Socket)
	}
	if profile.Config != "" {
		args = append(args, "-f", profile.Config)
	}
	return args, nil
}

func tmuxHasSession(profile TmuxProfile) bool {
	if profile.Session == "" {
		return false
	}
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return false
	}
	args := append(base, "has-session", "-t", profile.Session)
	return exec.Command("tmux", args...).Run() == nil
}

// TmuxProfileForAttempt builds attach/start profile from ledger data.
func TmuxProfileForAttempt(attempt AttemptRecord) (TmuxProfile, error) {
	if attempt.TmuxSession == "" {
		return TmuxProfile{}, errors.New("no tmux session on attempt")
	}
	cfg, err := gmcTmuxConfigFile()
	if err != nil {
		return TmuxProfile{}, err
	}
	if attempt.TmuxSocket != "" {
		return TmuxProfile{
			Session: attempt.TmuxSession,
			Socket:  attempt.TmuxSocket,
			Config:  cfg,
		}, nil
	}
	// Legacy: default tmux server, but use gmc minimal -f so a broken ~/.tmux.conf is not sourced.
	return TmuxProfile{Session: attempt.TmuxSession, Config: cfg}, nil
}

// StartTmuxSession starts a detached session on the gmc tmux server with a minimal config.
func StartTmuxSession(session, workdir string, command []string) (TmuxProfile, error) {
	if !tmuxAvailable() {
		return TmuxProfile{}, errors.New("tmux not found in PATH")
	}
	if len(command) == 0 {
		return TmuxProfile{}, errors.New("empty command")
	}
	cfg, err := gmcTmuxConfigFile()
	if err != nil {
		return TmuxProfile{}, err
	}
	profile := TmuxProfile{Session: session, Socket: gmcTmuxSocket, Config: cfg}
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return TmuxProfile{}, err
	}
	shellCmd := shellJoin(command)
	args := append(base, "new-session", "-d", "-s", session, "-c", workdir, shellCmd)
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return TmuxProfile{}, fmt.Errorf("tmux new-session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return profile, nil
}

// TmuxProfileForReview builds profile for the review-stage tmux session.
func TmuxProfileForReview(attempt AttemptRecord) (TmuxProfile, error) {
	if attempt.ReviewTmuxSession == "" {
		return TmuxProfile{}, errors.New("no review tmux session on attempt")
	}
	cfg, err := gmcTmuxConfigFile()
	if err != nil {
		return TmuxProfile{}, err
	}
	socket := attempt.ReviewTmuxSocket
	if socket == "" {
		socket = gmcTmuxSocket
	}
	return TmuxProfile{Session: attempt.ReviewTmuxSession, Socket: socket, Config: cfg}, nil
}

// RunTmuxToCompletion starts a detached gmc tmux session, waits until it exits, returns exit code.
func RunTmuxToCompletion(session, workdir string, command []string, logPath, exitPath string) (int, error) {
	if !tmuxAvailable() {
		return -1, errors.New("tmux not found in PATH")
	}
	if len(command) == 0 {
		return -1, errors.New("empty command")
	}
	cfg, err := gmcTmuxConfigFile()
	if err != nil {
		return -1, err
	}
	profile := TmuxProfile{Session: session, Socket: gmcTmuxSocket, Config: cfg}
	if tmuxHasSession(profile) {
		_ = KillTmuxSession(profile)
	}
	inner := shellJoin(command)
	script := fmt.Sprintf("%s > %q 2>&1; ec=$?; echo $ec > %q; exit $ec", inner, logPath, exitPath)
	if _, err := StartTmuxSession(session, workdir, []string{"bash", "-lc", script}); err != nil {
		return -1, err
	}
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return -1, err
	}
	waitArgs := append(base, "wait-session", "-t", session)
	if err := exec.Command("tmux", waitArgs...).Run(); err != nil {
		return -1, fmt.Errorf("tmux wait-session: %w", err)
	}
	data, err := os.ReadFile(exitPath)
	if err != nil {
		return -1, fmt.Errorf("read exit status: %w", err)
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return -1, fmt.Errorf("parse exit status: %w", err)
	}
	return code, nil
}

// KillTmuxSession stops a tmux session.
func KillTmuxSession(profile TmuxProfile) error {
	if !tmuxAvailable() {
		return errors.New("tmux not found in PATH")
	}
	if !tmuxHasSession(profile) {
		return nil
	}
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return err
	}
	args := append(base, "kill-session", "-t", profile.Session)
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux kill-session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// WatchHints is shown before a read-only tmux view (stderr).
type WatchHints struct {
	Title          string
	RuntimeMessage string
}

// WatchTmuxSession shows session output without modifying gmc task state.
// If lines > 0, prints a snapshot and returns; otherwise attaches read-only until the client exits.
func WatchTmuxSession(profile TmuxProfile, lines int, hints WatchHints) error {
	if !tmuxAvailable() {
		return errors.New("tmux not found in PATH")
	}
	if !tmuxHasSession(profile) {
		if profile.Socket != "" {
			legacy := TmuxProfile{Session: profile.Session}
			if tmuxHasSession(legacy) {
				profile = legacy
			} else {
				return fmt.Errorf("tmux session %q not found", profile.Session)
			}
		} else {
			return fmt.Errorf("tmux session %q not found", profile.Session)
		}
	}
	if lines > 0 {
		return captureTmuxPane(profile, lines)
	}
	printWatchHints(os.Stderr, hints)
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return err
	}
	args := append(base, "attach-session", "-r", "-t", profile.Session)
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func captureTmuxPane(profile TmuxProfile, lines int) error {
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return err
	}
	args := append(base, "capture-pane", "-t", profile.Session, "-p")
	if lines > 0 {
		start := -lines
		if start < 0 {
			args = append(args, "-S", strconv.Itoa(start))
		}
	}
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("tmux capture-pane: %w", err)
	}
	_, werr := os.Stdout.Write(out)
	return werr
}

func printWatchHints(w *os.File, hints WatchHints) {
	if w == nil {
		return
	}
	if hints.Title != "" {
		fmt.Fprintf(w, "gmc task watch: %s (read-only, does not change task state)\n", hints.Title)
	} else {
		fmt.Fprintln(w, "gmc task watch (read-only, does not change task state)")
	}
	if hints.RuntimeMessage != "" {
		fmt.Fprintln(w, hints.RuntimeMessage)
	}
	fmt.Fprintln(w, "Leave with Ctrl+b d. Use 'gmc task attach' when you need to intervene.")
}

// AttachTmuxSession attaches the current terminal to the session described by profile.
func AttachTmuxSession(profile TmuxProfile, hints AttachHints) error {
	if !tmuxAvailable() {
		return errors.New("tmux not found in PATH")
	}
	printAttachHints(os.Stderr, hints)
	if !tmuxHasSession(profile) {
		// Fallback: session may predate gmc-isolated tmux server.
		if profile.Socket != "" {
			legacy := TmuxProfile{Session: profile.Session}
			if tmuxHasSession(legacy) {
				profile = legacy
			} else {
				return fmt.Errorf("tmux session %q not found (try: tmux -L %s list-sessions)", profile.Session, gmcTmuxSocket)
			}
		} else {
			return fmt.Errorf("tmux session %q not found", profile.Session)
		}
	}
	base, err := tmuxBaseArgs(profile)
	if err != nil {
		return err
	}
	args := append(base, "attach-session", "-t", profile.Session)
	cmd := exec.Command("tmux", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printAttachHints(w *os.File, hints AttachHints) {
	if w == nil {
		return
	}
	if hints.Title != "" {
		fmt.Fprintf(w, "gmc task attach: %s (marks needs-human)\n", hints.Title)
	}
	if hints.RuntimeMessage != "" {
		fmt.Fprintln(w, hints.RuntimeMessage)
	}
	if hints.ContextPath != "" {
		fmt.Fprintf(w, "Task brief: %s\n", hints.ContextPath)
	}
	if hints.Prompt != "" {
		fmt.Fprintf(w, "Initial prompt (on start): %s\n", hints.Prompt)
	}
	switch hints.RuntimeStatus {
	case RuntimeAwaitingInput:
		fmt.Fprintln(w, "Agent may be waiting for input; attach only if you want to respond.")
	case RuntimeIdle:
		fmt.Fprintln(w, "Shell prompt in tmux; agent likely exited.")
	default:
		fmt.Fprintln(w, "Interactive agent: gmc cannot detect task completion from tmux alone.")
	}
}

func shellJoin(argv []string) string {
	var parts []string
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
