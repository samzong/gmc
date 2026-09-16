package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/samzong/gmc/internal/stringsutil"
	"github.com/samzong/gmc/internal/worktree"
)

type WorktreeJSON struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	Branch         string `json:"branch"`
	Commit         string `json:"commit"`
	Status         string `json:"status"`
	DiffBase       string `json:"diff_base,omitempty"`
	ChangedFiles   *int   `json:"changed_files,omitempty"`
	Insertions     *int   `json:"insertions,omitempty"`
	Deletions      *int   `json:"deletions,omitempty"`
	ReviewProvider string `json:"review_provider,omitempty"`
	ReviewNumber   int    `json:"review_number,omitempty"`
	ReviewState    string `json:"review_state,omitempty"`
	ReviewURL      string `json:"review_url,omitempty"`
}

func filterBareWorktrees(worktrees []worktree.Info) []worktree.Info {
	var filtered []worktree.Info
	for _, wt := range worktrees {
		if wt.IsBare || filepath.Base(wt.Path) == ".bare" {
			continue
		}
		filtered = append(filtered, wt)
	}
	return filtered
}

func runWorktreeList(wtClient *worktree.Client, showCurrent bool) error {
	worktrees, err := wtClient.List()
	if err != nil {
		return err
	}
	worktrees = filterBareWorktrees(worktrees)
	reviews := loadWorktreeReviews(wtClient, worktrees)
	diffStats, err := loadWorktreeDiffStats(wtClient, worktrees)
	if err != nil {
		return err
	}
	if outputFormat() == "json" {
		if err := printJSON(outWriter(), buildWorktreeJSON(wtClient, worktrees, reviews.Reviews, diffStats)); err != nil {
			return err
		}
		printReviewWarning(errWriter(), reviews)
		return nil
	}
	if showCurrent {
		fmt.Fprintln(outWriter(), "Current Worktrees:")
	} else if len(worktrees) == 0 {
		fmt.Fprintln(outWriter(), "No worktrees found.")
		return nil
	}
	printWorktreeTable(wtClient, worktrees, reviews.Reviews, diffStats)
	if cwd, err := os.Getwd(); showCurrent && err == nil {
		for _, wt := range worktrees {
			if strings.HasPrefix(cwd, wt.Path) {
				fmt.Fprintf(outWriter(), "\nYou are here: ./%s (branch: %s)\n", filepath.Base(wt.Path), wt.Branch)
				break
			}
		}
	}
	printReviewWarning(outWriter(), reviews)
	return nil
}

func getDisplayRoot(wtClient *worktree.Client) string {
	root, err := wtClient.GetWorktreeRoot()
	if err != nil || root == "" {
		return ""
	}
	bareDir := filepath.Join(root, ".bare")
	if info, err := os.Stat(bareDir); err == nil && info.IsDir() {
		return root
	}
	return filepath.Dir(root)
}

func isExternalWorktree(displayRoot, wtPath string) bool {
	if displayRoot == "" {
		return false
	}
	rel, err := filepath.Rel(displayRoot, wtPath)
	if err != nil {
		return true
	}
	return strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".."
}

func isAgentWorktree(wtPath string) bool {
	normalized := filepath.ToSlash(wtPath)
	return strings.Contains(normalized, "/.claude/worktrees/") ||
		strings.Contains(normalized, "/.codex/worktrees/")
}

func abbrevPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

func displayWorktreeName(displayRoot string, wtPath string) string {
	if displayRoot == "" {
		return filepath.Base(wtPath)
	}
	rel, err := filepath.Rel(displayRoot, wtPath)
	if err != nil || rel == "." || rel == "" {
		return filepath.Base(wtPath)
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return abbrevPath(wtPath)
	}
	if isAgentWorktree(wtPath) {
		return abbrevPath(wtPath)
	}
	return rel
}

func resolveWorktreeStatus(wtClient *worktree.Client, root string, wt worktree.Info) string {
	switch {
	case wt.IsBare:
		return "bare"
	case isExternalWorktree(root, wt.Path), isAgentWorktree(wt.Path):
		return "agent"
	default:
		return wtClient.GetWorktreeStatus(wt.Path)
	}
}

func loadWorktreeDiffStats(wtClient *worktree.Client, worktrees []worktree.Info) (map[string]worktree.DiffStat, error) {
	lookup := make(map[string]worktree.DiffStat)

	override := strings.TrimSpace(wtDiffBase)

	for _, wt := range worktrees {
		base, baseErr := wtClient.ResolveDiffBaseForWorktree(wt.Path, wtDiffBase)
		if baseErr != nil {
			if override != "" {
				return lookup, baseErr
			}
			continue
		}

		stat, statErr := wtClient.WorktreeDiffStat(wt.Path, base)
		if statErr != nil {
			if override != "" {
				return lookup, statErr
			}
			continue
		}
		lookup[wt.Path] = stat
	}
	return lookup, nil
}

func formatWorktreeStatus(status string, stat worktree.DiffStat, ok bool) string {
	if !ok || !stat.HasChanges() {
		return status
	}
	diffStat := formatDiffStat(stat)
	if status == "" || status == "clean" {
		return diffStat
	}
	return status + ", " + diffStat
}

func formatDiffStat(stat worktree.DiffStat) string {
	fileLabel := "files"
	if stat.Files == 1 {
		fileLabel = "file"
	}
	return fmt.Sprintf("%d %s (+%d -%d)", stat.Files, fileLabel, stat.Insertions, stat.Deletions)
}

func loadWorktreeReviews(wtClient *worktree.Client, worktrees []worktree.Info) worktree.ReviewLookup {
	if !wtShowPR || len(worktrees) == 0 {
		return worktree.ReviewLookup{}
	}
	return wtClient.ReviewStates(worktrees)
}

func printReviewWarning(w io.Writer, reviews worktree.ReviewLookup) {
	if reviews.Warning == "" {
		return
	}
	fmt.Fprintln(w, "Warning: "+reviews.Warning)
}

func formatWorktreeReview(reviews map[string]worktree.ReviewInfo, branch string) string {
	if reviews == nil {
		return ""
	}
	review, ok := reviews[branch]
	if !ok {
		return "-"
	}
	if review.State == "" {
		return fmt.Sprintf("#%d", review.Number)
	}
	return fmt.Sprintf("#%d %s", review.Number, review.State)
}

func formatWorktreeReviewDisplay(reviews map[string]worktree.ReviewInfo, branch string, links bool) string {
	text := formatWorktreeReview(reviews, branch)
	if text == "" || text == "-" || !links {
		return text
	}
	review, ok := reviews[branch]
	if !ok || review.URL == "" || review.Number == 0 {
		return text
	}
	number := fmt.Sprintf("#%d", review.Number)
	linked := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", review.URL, number)
	return strings.Replace(text, number, linked, 1)
}

func terminalLinksEnabled(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok || os.Getenv("TERM") == "dumb" {
		return false
	}
	return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
}

func padVisibleRight(text string, visibleLen int, width int) string {
	if visibleLen >= width {
		return text
	}
	return text + strings.Repeat(" ", width-visibleLen)
}

func printWorktreeTable(
	wtClient *worktree.Client,
	worktrees []worktree.Info,
	reviews map[string]worktree.ReviewInfo,
	diffStats map[string]worktree.DiffStat,
) {
	if len(worktrees) == 0 {
		return
	}

	root := getDisplayRoot(wtClient)
	writer := outWriter()
	links := terminalLinksEnabled(writer)

	maxName := len("Name")
	maxBranch := len("Branch")
	maxPR := len("PR")
	for _, wt := range worktrees {
		name := displayWorktreeName(root, wt.Path)
		maxName = max(maxName, len(name))
		maxBranch = max(maxBranch, len(wt.Branch))
		maxPR = max(maxPR, len(formatWorktreeReview(reviews, wt.Branch)))
	}

	maxName += 2
	maxBranch += 2
	maxPR += 2

	reviewHeading := ""
	if reviews != nil {
		reviewHeading = fmt.Sprintf("%-*s ", maxPR, "PR")
	}
	fmt.Fprintf(writer, "%-*s %-*s %-8s %sSTATUS\n", maxName, "NAME", maxBranch, "BRANCH", "COMMIT", reviewHeading)

	for _, wt := range worktrees {
		name := displayWorktreeName(root, wt.Path)
		shortCommit := stringsutil.ShortHash(wt.Commit, 7, "")
		stat, hasStat := diffStats[wt.Path]
		status := formatWorktreeStatus(resolveWorktreeStatus(wtClient, root, wt), stat, hasStat)
		reviewColumn := ""
		if reviews != nil {
			text := formatWorktreeReview(reviews, wt.Branch)
			display := formatWorktreeReviewDisplay(reviews, wt.Branch, links)
			reviewColumn = padVisibleRight(display, len(text), maxPR) + " "
		}
		fmt.Fprintf(writer, "%-*s %-*s %-8s %s%s\n", maxName, name, maxBranch, wt.Branch, shortCommit, reviewColumn, status)
	}
}

func buildWorktreeJSON(
	wtClient *worktree.Client,
	worktrees []worktree.Info,
	reviews map[string]worktree.ReviewInfo,
	diffStats map[string]worktree.DiffStat,
) []WorktreeJSON {
	root := getDisplayRoot(wtClient)
	result := make([]WorktreeJSON, 0, len(worktrees))
	for _, wt := range worktrees {
		stat, hasStat := diffStats[wt.Path]
		item := WorktreeJSON{
			Name:   displayWorktreeName(root, wt.Path),
			Path:   wt.Path,
			Branch: wt.Branch,
			Commit: wt.Commit,
			Status: resolveWorktreeStatus(wtClient, root, wt),
		}
		if hasStat && stat.HasChanges() {
			item.DiffBase = stat.Base
			item.ChangedFiles = &stat.Files
			item.Insertions = &stat.Insertions
			item.Deletions = &stat.Deletions
		}
		if reviews != nil {
			if review, ok := reviews[wt.Branch]; ok {
				item.ReviewProvider = review.Provider
				item.ReviewNumber = review.Number
				item.ReviewState = review.State
				item.ReviewURL = review.URL
			} else {
				item.ReviewState = "none"
			}
		}
		result = append(result, item)
	}
	return result
}
