package cmd

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/samzong/gmc/internal/worktree"
)

type WorktreeCreateJSON struct {
	Name     string   `json:"name"`
	Path     string   `json:"path,omitempty"`
	Branch   string   `json:"branch,omitempty"`
	Base     string   `json:"base,omitempty"`
	Created  bool     `json:"created"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func newWorktreeCreateJSON(result worktree.CreateResult) WorktreeCreateJSON {
	item := WorktreeCreateJSON{
		Name:     result.Name,
		Path:     result.Path,
		Branch:   result.Branch,
		Base:     result.Base,
		Created:  result.Created,
		Warnings: result.Warnings,
	}
	if result.Err != nil {
		item.Error = result.Err.Error()
	}
	return item
}

func worktreeReportWriter() io.Writer {
	if outputFormat() == "json" {
		return errWriter()
	}
	return outWriter()
}

func runWorktreeAdd(wtClient *worktree.Client, names []string) error {
	if wtAddPR > 0 {
		return runWorktreeAddPR(wtClient, wtAddPR)
	}
	items := make([]WorktreeCreateJSON, 0, len(names))
	err := addWorktrees(wtClient, names, &items)
	if outputFormat() == "json" {
		if jsonErr := printJSON(outWriter(), items); jsonErr != nil {
			return jsonErr
		}
	}
	return err
}

func addWorktrees(wtClient *worktree.Client, names []string, items *[]WorktreeCreateJSON) error {
	reportOut := worktreeReportWriter()
	baseBranch, err := syncBeforeAdd(wtClient, reportOut)
	if err != nil {
		for _, name := range names {
			*items = append(*items, WorktreeCreateJSON{Name: name, Error: err.Error()})
		}
		return err
	}
	opts := worktree.AddOptions{
		BaseBranch: baseBranch,
		Fetch:      false,
	}
	var failed []string
	for _, name := range names {
		result, err := wtClient.Add(name, opts)
		printWorktreeReportTo(result.Report, reportOut)
		*items = append(*items, newWorktreeCreateJSON(result.CreateResult))
		if err != nil {
			fmt.Fprintf(errWriter(), "Error adding '%s': %v\n", name, err)
			failed = append(failed, name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("failed to add worktrees: %s", strings.Join(failed, ", "))
	}
	return nil
}

func syncBeforeAdd(wtClient *worktree.Client, reportOut io.Writer) (string, error) {
	baseBranch := wtBaseBranch
	if !wtAddSync {
		return baseBranch, nil
	}
	if baseBranch == "" {
		resolved, err := wtClient.ResolveSyncBaseBranch("")
		if err != nil {
			return "", err
		}
		baseBranch = resolved
	}
	report, err := wtClient.Sync(worktree.SyncOptions{BaseBranch: baseBranch, DryRun: false})
	printWorktreeReportTo(report, reportOut)
	return baseBranch, err
}

func runWorktreeAddPR(wtClient *worktree.Client, prNumber int) error {
	result, err := wtClient.AddPR(prNumber, "")
	printWorktreeReportTo(result.Report, worktreeReportWriter())
	if outputFormat() == "json" {
		items := []WorktreeCreateJSON{newWorktreeCreateJSON(result.CreateResult)}
		if jsonErr := printJSON(outWriter(), items); jsonErr != nil {
			return jsonErr
		}
	}
	return err
}

func runWorktreeRemove(wtClient *worktree.Client, names []string) error {
	if wtAll {
		resolved, err := resolveAllRemovableWorktrees(wtClient)
		if err != nil {
			return err
		}
		if len(resolved) == 0 {
			fmt.Fprintln(outWriter(), "No removable worktrees found.")
			return nil
		}
		names = resolved
	}

	opts := worktree.RemoveOptions{
		Force:        wtForce,
		DeleteBranch: wtDeleteBranch,
		DryRun:       wtDryRun,
	}

	result := wtClient.RemoveBatch(names, opts)
	printWorktreeReport(result.Report)

	var failed []string
	for _, name := range names {
		if err, ok := result.Failed[name]; ok {
			fmt.Fprintf(errWriter(), "Error removing '%s': %v\n", name, err)
			failed = append(failed, name)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to remove worktrees: %s", strings.Join(failed, ", "))
	}
	return nil
}

func resolveAllRemovableWorktrees(wtClient *worktree.Client) ([]string, error) {
	all, err := wtClient.List()
	if err != nil {
		return nil, err
	}

	pp, err := wtClient.NewProtectionPolicy()
	if err != nil {
		return nil, err
	}
	root := getDisplayRoot(wtClient)
	var names []string
	for _, wt := range all {
		if pp.IsProtected(wt) {
			continue
		}
		if isExternalWorktree(root, wt.Path) || isAgentWorktree(wt.Path) {
			continue
		}
		names = append(names, displayWorktreeName(root, wt.Path))
	}
	return names, nil
}

func runWorktreeClone(wtClient *worktree.Client, url string) error {
	opts := worktree.CloneOptions{
		Name:     wtProjectName,
		Upstream: wtUpstream,
	}
	report, err := wtClient.Clone(url, opts)
	printWorktreeReport(report)
	return err
}

func runWorktreeDup(wtClient *worktree.Client, args []string) error {
	opts := worktree.DupOptions{
		BaseBranch: wtDupBase,
		Count:      2,
		TaskFiles:  wtDupTasks,
	}

	if len(args) > 0 {
		count, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid count: %s", args[0])
		}
		opts.Count = count
	}

	result, err := wtClient.Dup(opts)
	if outputFormat() == "json" {
		return printWorktreeDupJSON(result, err)
	}
	if err != nil {
		return err
	}
	for _, item := range result.Items {
		for _, warning := range item.Warnings {
			fmt.Fprintln(errWriter(), warning)
		}
	}

	fmt.Fprintf(outWriter(), "Created %d worktrees based on '%s':\n", len(result.Items), result.BaseBranch)
	for _, item := range result.Items {
		relPath := item.Name
		if item.RelativePath != "" {
			relPath = item.RelativePath
		}
		if item.Path == "" {
			fmt.Fprintf(outWriter(), "  %s -> %s\n", relPath, item.Branch)
		} else {
			fmt.Fprintf(outWriter(), "  %s (%s) -> %s\n", relPath, item.Path, item.Branch)
		}
	}
	if len(result.TaskFiles) > 0 {
		fmt.Fprintln(outWriter(), "Copied task files:")
		for _, task := range result.TaskFiles {
			fmt.Fprintf(outWriter(), "  %s\n", task)
		}
	}
	fmt.Fprintln(outWriter())
	fmt.Fprintln(outWriter(), "Next steps:")
	fmt.Fprintln(outWriter(), "  1. Work in each directory with different AI tools")
	fmt.Fprintln(outWriter(), "  2. Evaluate and pick the best solution")
	fmt.Fprintf(outWriter(), "  3. Dry-run promote: gmc wt promote <candidate> --dry-run\n")
	fmt.Fprintf(outWriter(), "  4. Promote winner: gmc wt promote <candidate>\n")
	fmt.Fprintln(outWriter(), "  5. Clean up: gmc wt rm <other-worktrees> -D")

	return nil
}

func printWorktreeDupJSON(result *worktree.DupResult, dupErr error) error {
	items := []WorktreeCreateJSON{}
	if result != nil {
		for _, item := range result.Items {
			for _, warning := range item.Warnings {
				fmt.Fprintln(errWriter(), warning)
			}
			items = append(items, newWorktreeCreateJSON(item.CreateResult))
		}
	}
	if err := printJSON(outWriter(), items); err != nil {
		return err
	}
	return dupErr
}

func runWorktreePromote(wtClient *worktree.Client, candidate string) error {
	report, err := wtClient.Promote(candidate, worktree.PromoteOptions{
		DryRun: wtDryRun,
	})
	printWorktreeReport(report)
	return err
}
