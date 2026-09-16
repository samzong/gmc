package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/stringsutil"
)

func (c *Client) untrackedFiles(candidatePath string) ([]string, error) {
	data, err := c.gitBytes(candidatePath, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	parts := bytes.Split(data, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		path := filepath.Clean(string(part))
		if path == "." || filepath.IsAbs(path) || strings.HasPrefix(path, ".."+string(filepath.Separator)) || path == ".." {
			return nil, fmt.Errorf("unsafe untracked path: %s", string(part))
		}
		files = append(files, path)
	}
	return files, nil
}

func (c *Client) collectPromotionFileNames(candidatePath, mergeBase string, untracked []string) ([]string, error) {
	specs := [][]string{
		{"diff", "--name-only", "-M", mergeBase + "..HEAD"},
		{"diff", "--name-only", "-M", "--cached"},
		{"diff", "--name-only", "-M"},
	}

	var files []string
	for _, args := range specs {
		data, err := c.gitBytes(candidatePath, args...)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			files = append(files, filepath.Clean(line))
		}
	}
	files = append(files, untracked...)
	return stringsutil.UniqueStrings(files), nil
}

func filterExistingIdenticalFiles(parentRoot, candidatePath string, files []string) ([]string, []string, error) {
	filtered := make([]string, 0, len(files))
	var ignored []string
	for _, rel := range files {
		same, err := sameRegularFileContent(filepath.Join(candidatePath, rel), filepath.Join(parentRoot, rel))
		if err != nil {
			return nil, nil, err
		}
		if same {
			ignored = append(ignored, rel)
			continue
		}
		filtered = append(filtered, rel)
	}
	return filtered, ignored, nil
}

func sameRegularFileContent(a, b string) (bool, error) {
	aInfo, err := os.Lstat(a)
	if err != nil {
		return false, fmt.Errorf("failed to inspect source file %s: %w", a, err)
	}
	if !aInfo.Mode().IsRegular() {
		return false, nil
	}

	bInfo, err := os.Lstat(b)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to inspect destination file %s: %w", b, err)
	}
	if !bInfo.Mode().IsRegular() || aInfo.Size() != bInfo.Size() {
		return false, nil
	}

	aFile, err := os.Open(a)
	if err != nil {
		return false, fmt.Errorf("failed to open source file %s: %w", a, err)
	}
	defer aFile.Close()

	bFile, err := os.Open(b)
	if err != nil {
		return false, fmt.Errorf("failed to open destination file %s: %w", b, err)
	}
	defer bFile.Close()

	aBuf := make([]byte, 32*1024)
	bBuf := make([]byte, 32*1024)
	for {
		aN, aErr := aFile.Read(aBuf)
		bN, bErr := bFile.Read(bBuf)
		if aN != bN || !bytes.Equal(aBuf[:aN], bBuf[:bN]) {
			return false, nil
		}
		if errors.Is(aErr, io.EOF) && errors.Is(bErr, io.EOF) {
			return true, nil
		}
		if aErr != nil && !errors.Is(aErr, io.EOF) {
			return false, fmt.Errorf("failed to read source file %s: %w", a, aErr)
		}
		if bErr != nil && !errors.Is(bErr, io.EOF) {
			return false, fmt.Errorf("failed to read destination file %s: %w", b, bErr)
		}
	}
}

func (c *Client) preflightUntrackedCopies(parentRoot string, untracked []string) error {
	for _, rel := range untracked {
		target := filepath.Join(parentRoot, rel)
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("cannot promote untracked file %s: destination already exists", rel)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect destination for %s: %w", rel, err)
		}
	}
	return nil
}

func copyUntrackedPromotionFiles(candidatePath, targetPath string, untracked []string) error {
	for _, rel := range untracked {
		src := filepath.Join(candidatePath, rel)
		dst := filepath.Join(targetPath, rel)
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("cannot promote untracked file %s: destination already exists", rel)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect destination for %s: %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("failed to create destination directory for %s: %w", rel, err)
		}
		if err := copyPromotionFile(src, dst); err != nil {
			return fmt.Errorf("failed to copy untracked file %s: %w", rel, err)
		}
	}
	return nil
}

func copyPromotionFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported untracked file type: %s", src)
	}
	return copyFile(src, dst)
}
