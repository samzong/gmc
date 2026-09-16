package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/gitutil"
	"github.com/samzong/gmc/internal/stringsutil"
)

func (c *Client) ResolveFiles(paths []string) ([]string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, err
	}

	var resolvedFiles []string
	for _, path := range paths {
		cleanPath := filepath.Clean(path)

		info, err := os.Stat(cleanPath)
		if err != nil {
			if os.IsNotExist(err) {
				inIndex, indexErr := c.isPathInStagedDiff(cleanPath)
				if indexErr != nil {
					return nil, fmt.Errorf("failed to resolve path %s: %w", path, indexErr)
				}
				if inIndex {
					resolvedFiles = append(resolvedFiles, cleanPath)
					continue
				}
				return nil, fmt.Errorf("file or directory does not exist: %s", path)
			}
			return nil, fmt.Errorf("failed to check path: %s: %w", path, err)
		}

		if info.IsDir() {
			dirFiles, err := c.getGitTrackedFilesInDir(cleanPath)
			if err != nil {
				return nil, fmt.Errorf("failed to get files in directory %s: %w", path, err)
			}
			resolvedFiles = append(resolvedFiles, dirFiles...)
		} else {
			resolvedFiles = append(resolvedFiles, cleanPath)
		}
	}

	return stringsutil.UniqueStrings(resolvedFiles), nil
}

func (c *Client) isPathInStagedDiff(path string) (bool, error) {
	gitPath := filepath.ToSlash(path)

	result, err := c.runner.RunLogged("diff", "--cached", "--name-only", "--", gitPath)
	if err != nil {
		c.logVerboseOutput("Git stderr:", result.Stderr)
		return false, fmt.Errorf("failed to inspect staged diff: %w", err)
	}

	output := result.StdoutString(true)
	if output == "" {
		return false, nil
	}

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == gitPath {
			return true, nil
		}
	}

	return false, nil
}

func (c *Client) getGitTrackedFilesInDir(dir string) ([]string, error) {
	runResult, err := c.runner.RunLogged("ls-files", "--cached", "--others", "--exclude-standard", dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list git files in directory: %w", err)
	}

	output := runResult.StdoutString(true)
	return stringsutil.SplitNonEmpty(output, "\n"), nil
}

func (c *Client) CheckFileStatus(files []string) ([]string, []string, []string, error) {
	if err := c.CheckGitRepository(); err != nil {
		return nil, nil, nil, err
	}

	var staged, modified, untracked []string

	for _, file := range files {
		isStaged, err := c.hasOutput("diff", "--cached", "--name-only", file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to check staged status for %s: %w", file, err)
		}

		isModified, err := c.hasOutput("diff", "--name-only", file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to check modified status for %s: %w", file, err)
		}

		isTracked, err := c.hasOutput("ls-files", file)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to check tracked status for %s: %w", file, err)
		}

		switch {
		case isStaged:
			staged = append(staged, file)
		case isModified:
			modified = append(modified, file)
		case !isTracked:
			untracked = append(untracked, file)
		}
	}

	return staged, modified, untracked, nil
}

func (c *Client) hasOutput(args ...string) (bool, error) {
	result, err := c.runner.Run(args...)
	return result.StdoutString(true) != "", err
}

func (c *Client) StageFiles(files []string) error {
	if err := c.CheckGitRepository(); err != nil {
		return err
	}

	for _, file := range files {
		result, err := c.runner.RunLogged("add", file)
		if err != nil {
			return gitutil.WrapGitError("failed to stage file "+file, result, err)
		}
	}

	return nil
}

func (c *Client) GetFilesDiff(files []string) (string, error) {
	if len(files) == 0 {
		return "", c.CheckGitRepository()
	}
	return c.checkedOutput("failed to get diff for files", append([]string{"diff", "--cached", "--"}, files...)...)
}
