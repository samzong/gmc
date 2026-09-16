package worktree

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func cachedReviewOutput(
	provider string,
	repoURL string,
	author string,
	load func() ([]byte, error),
) ([]byte, error) {
	if out, ok := readReviewCache(provider, repoURL, author); ok {
		return out, nil
	}
	out, err := load()
	if err != nil {
		return nil, err
	}
	writeReviewCache(provider, repoURL, author, out)
	return out, nil
}

func reviewCachePath(provider string, repoURL string, author string) (string, bool) {
	dir, err := reviewCacheDirFunc()
	if err != nil || dir == "" {
		return "", false
	}
	key := strings.Join([]string{reviewCacheVersion, provider, repoURL, author}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, "gmc", "reviews", fmt.Sprintf("%x.json", sum)), true
}

func decodeReviewJSON(out []byte, target any) error {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" {
		return nil
	}
	if err := json.Unmarshal([]byte(trimmed), target); err != nil {
		return fmt.Errorf("failed to parse review lookup output: %w", err)
	}
	return nil
}

func readReviewCache(provider string, repoURL string, author string) ([]byte, bool) {
	path, ok := reviewCachePath(provider, repoURL, author)
	if !ok {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > reviewCacheTTL {
		return nil, false
	}
	out, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return out, true
}

func writeReviewCache(provider string, repoURL string, author string, out []byte) {
	path, ok := reviewCachePath(provider, repoURL, author)
	if !ok {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
