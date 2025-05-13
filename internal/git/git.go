package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func GetDiff() (string, error) {
	cmd := exec.Command("git", "diff")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Failed to run git diff: %w", err)
	}

	unstaged := out.String()
	out.Reset()

	cmd = exec.Command("git", "diff", "--cached")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Failed to run git diff --cached: %w", err)
	}

	staged := out.String()

	diff := unstaged + staged
	return diff, nil
}

func GetStagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--cached")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Failed to run git diff --cached: %w", err)
	}

	return out.String(), nil
}

func ParseChangedFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Failed to run git diff --name-only: %w", err)
	}

	unstaged := strings.Split(strings.TrimSpace(out.String()), "\n")
	out.Reset()

	cmd = exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Failed to run git diff --cached --name-only: %w", err)
	}

	staged := strings.Split(strings.TrimSpace(out.String()), "\n")

	fileMap := make(map[string]bool)
	for _, file := range unstaged {
		if file != "" {
			fileMap[file] = true
		}
	}

	for _, file := range staged {
		if file != "" {
			fileMap[file] = true
		}
	}

	var changedFiles []string
	for file := range fileMap {
		changedFiles = append(changedFiles, file)
	}

	return changedFiles, nil
}

func ParseStagedFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Failed to run git diff --cached --name-only: %w", err)
	}

	stagedFiles := strings.Split(strings.TrimSpace(out.String()), "\n")

	var result []string
	for _, file := range stagedFiles {
		if file != "" {
			result = append(result, file)
		}
	}

	return result, nil
}

func AddAll() error {
	cmd := exec.Command("git", "add", ".")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Failed to run git add .: %w", err)
	}
	return nil
}

func Commit(message string, args ...string) error {
	commitArgs := append([]string{"commit", "-m", message}, args...)
	cmd := exec.Command("git", commitArgs...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Failed to run git commit: %w", err)
	}
	return nil
}
