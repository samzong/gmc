package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
)

type CommitInfo struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
	Body    string `json:"body"`
}

func (c *Client) GetLatestTag() (string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return "", err
	}

	result, err := c.runner.RunLogged("tag", "--sort=-creatordate")
	if err != nil {
		return "", fmt.Errorf("failed to list tags: %w", err)
	}

	tag, _, _ := strings.Cut(result.StdoutString(true), "\n")
	return strings.TrimSpace(tag), nil
}

func (c *Client) GetCommitsSinceTag(tag string) ([]CommitInfo, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, err
	}

	format := "%H%x1f%an%x1f%ad%x1f%s%x1f%b%x1e"
	args := []string{"log", "--pretty=format:" + format, "--date=short"}

	if tag != "" {
		exists, err := c.tagExists(tag)
		if err != nil {
			return nil, err
		}
		if exists {
			args = append(args, tag+"..HEAD")
		}
	}

	result, err := c.runner.RunLogged(args...)
	if err != nil {
		if len(result.Stderr) > 0 {
			return nil, fmt.Errorf("failed to run git log: %s", strings.TrimSpace(string(result.Stderr)))
		}
		return nil, fmt.Errorf("failed to run git log: %w", err)
	}

	data := result.Stdout
	records := bytes.Split(data, []byte{0x1e})
	commits := make([]CommitInfo, 0, len(records))

	for _, record := range records {
		record = bytes.TrimSpace(record)
		if len(record) == 0 {
			continue
		}

		fields := bytes.Split(record, []byte{0x1f})
		if len(fields) < 5 {
			continue
		}

		commit := CommitInfo{
			Hash:    strings.TrimSpace(string(fields[0])),
			Author:  strings.TrimSpace(string(fields[1])),
			Date:    strings.TrimSpace(string(fields[2])),
			Message: strings.TrimSpace(string(fields[3])),
			Body:    strings.TrimSpace(string(fields[4])),
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

func (c *Client) CreateAnnotatedTag(tag string, message string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}

	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("tag name cannot be empty")
	}

	if message == "" {
		message = "Release " + tag
	}

	if c.runner.Verbose {
		fmt.Fprintf(os.Stderr, "Running: git tag -a %s -m %q\n", tag, message)
	}

	result, err := c.runner.Run("tag", "-a", tag, "-m", message)
	if err != nil {
		return gitutil.WrapGitError(fmt.Sprintf("failed to create tag '%s'", tag), result, err)
	}

	return nil
}

func (c *Client) tagExists(tag string) (bool, error) {
	ref := "refs/tags/" + tag
	_, err := c.runner.Run("rev-parse", "--verify", ref)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify tag %s: %w", tag, err)
	}

	return true, nil
}
