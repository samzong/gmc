package worktree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func (c *Client) syncSharedResourcesToPath(targetRoot string, runHooks bool) (Report, error) {
	return c.syncWorktreeResources(targetRoot, runHooks, false)
}

func (c *Client) prepareNewWorktree(targetRoot string) (Report, error) {
	return c.syncWorktreeResources(targetRoot, true, true)
}

func (c *Client) syncWorktreeResources(targetRoot string, runHooks, prepare bool) (Report, error) {
	var report Report

	cfg, err := c.LoadEffectiveSharedConfig()
	if err != nil {
		return report, err
	}

	if !prepare && len(cfg.Resources) == 0 && (!runHooks || len(cfg.Hooks) == 0) {
		return report, nil
	}

	if err := c.ensureInit(); err != nil {
		return report, err
	}

	resources, err := c.expandSharedResources(cfg.Resources)
	if err != nil {
		return report, err
	}
	for _, res := range resources {
		resourceReport, err := c.syncOneResource(c.worktreeRoot, targetRoot, res)
		report.Merge(resourceReport)
		if err != nil {
			return report, err
		}
	}

	if prepare {
		c.prepareRustCache(targetRoot, &report)
	}
	if runHooks {
		if err := c.runHooks(targetRoot, cfg.Hooks, &report); err != nil {
			return report, err
		}
	}

	return report, nil
}

func (c *Client) syncOneResource(repoRoot, targetRoot string, res SharedResource) (Report, error) {
	var report Report

	if res.Path == "" {
		return report, errors.New("shared resource missing 'path' field")
	}
	if res.Strategy == "" {
		return report, fmt.Errorf("shared resource '%s' missing 'strategy' field", res.Path)
	}

	srcPath, targetPath, skip, err := c.resolveSharedPaths(repoRoot, targetRoot, res)
	if err != nil {
		return report, err
	}
	if skip {
		return report, nil
	}

	dstPath := filepath.Join(targetRoot, targetPath)

	info, err := os.Stat(srcPath)
	if os.IsNotExist(err) {
		if c.verbose {
			report.Warn("Shared resource source not found: " + srcPath)
		}
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("cannot inspect shared source %s: %w", srcPath, err)
	}

	if filepath.Clean(srcPath) == filepath.Clean(dstPath) {
		return report, nil
	}
	if dstInfo, statErr := os.Lstat(dstPath); statErr == nil {
		if res.Strategy == StrategyCopy && dstInfo.Mode().Type() == info.Mode().Type() {
			return report, nil
		}
		if dstInfo.Mode()&os.ModeSymlink != 0 {
			destination, dstErr := filepath.EvalSymlinks(dstPath)
			source, srcErr := filepath.EvalSymlinks(srcPath)
			if dstErr == nil && srcErr == nil && destination == source && res.Strategy == StrategySymlink {
				return report, nil
			}
		}
		report.Warn(fmt.Sprintf("Kept existing resource: %s (not replaced; requested %s from %s)",
			dstPath, res.Strategy, srcPath))
		return report, nil
	} else if !os.IsNotExist(statErr) {
		return report, fmt.Errorf("cannot inspect shared destination %s: %w", dstPath, statErr)
	}
	if err := ensureUntrackedResource(targetRoot, targetPath); err != nil {
		return report, err
	}
	if err := ensureUntrackedResource(filepath.Dir(srcPath), filepath.Base(srcPath)); err != nil {
		return report, err
	}
	if err := ensureSharedDestination(targetRoot, targetPath); err != nil {
		return report, err
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return report, fmt.Errorf("failed to create parent directory for %s: %w", dstPath, err)
	}

	report.Info(fmt.Sprintf("Syncing shared resource: %s -> %s (%s)", res.Path, targetPath, res.Strategy))

	switch res.Strategy {
	case StrategySymlink:
		relSrc, err := filepath.Rel(filepath.Dir(dstPath), srcPath)
		if err != nil {
			return report, fmt.Errorf("failed to calculate relative path: %w", err)
		}
		if err := os.Symlink(relSrc, dstPath); err != nil {
			return report, fmt.Errorf("failed to symlink %s: %w", res.Path, err)
		}
	case StrategyCopy:
		if info.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return report, fmt.Errorf("failed to copy directory %s: %w", res.Path, err)
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return report, fmt.Errorf("failed to copy file %s: %w", res.Path, err)
			}
		}
	default:
		return report, fmt.Errorf("unknown strategy '%s' for resource '%s' (valid: copy, link)", res.Strategy, res.Path)
	}
	return report, nil
}

func (c *Client) SyncAllSharedResources() (Report, error) {
	var report Report

	worktrees, err := c.ListCached()
	if err != nil {
		return report, err
	}

	if err := c.ensureInit(); err != nil {
		return report, err
	}
	isBare := c.repoDir != c.worktreeRoot
	var targets []Info
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" {
			continue
		}
		if isBare && isExternalPath(c.worktreeRoot, wt.Path) {
			continue
		}
		targets = append(targets, wt)
	}

	if len(targets) == 0 {
		report.Info("No worktrees to sync.")
		return report, nil
	}

	report.Info(fmt.Sprintf("Syncing resources to %d worktrees...", len(targets)))
	var failures []error
	for _, wt := range targets {
		resourceReport, err := c.syncSharedResourcesToPath(wt.Path, false)
		report.Merge(resourceReport)
		if err != nil {
			report.Warn(fmt.Sprintf("Warning: failed to sync %s: %v", filepath.Base(wt.Path), err))
			failures = append(failures, fmt.Errorf("sync %s: %w", filepath.Base(wt.Path), err))
		}
	}

	return report, errors.Join(failures...)
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	sourceInfo, err := os.Stat(src)
	if err == nil {
		_ = os.Chmod(dst, sourceInfo.Mode())
	}

	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

func (c *Client) runHooks(worktreeRoot string, hooks []Hook, report *Report) error {
	for _, hook := range hooks {
		if hook.Disabled || hook.Cmd == "" {
			continue
		}

		label := hook.Cmd
		if hook.Desc != "" {
			label = hook.Desc
		}
		report.Info(fmt.Sprintf("Running hook: %s", label))

		cmd := exec.Command("sh", "-c", hook.Cmd)
		cmd.Dir = worktreeRoot
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook failed '%s': %w", hook.Cmd, err)
		}
	}

	return nil
}
