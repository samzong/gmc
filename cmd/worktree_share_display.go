package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/samzong/gmc/internal/worktree"
	"github.com/spf13/cobra"
)

func printDiscoverOverview(cmd *cobra.Command, results []worktree.DiscoverResult, hooks []worktree.Hook) {
	var directories, files []worktree.DiscoverResult
	projects := make(map[string]int)
	candidates, enabled := 0, 0
	for _, result := range results {
		if result.ProjectMarker {
			label := result.Ecosystem
			if result.PackageManager != "" && !strings.EqualFold(label, result.PackageManager) {
				label += " (" + result.PackageManager + ")"
			}
			projects[label]++
			continue
		}
		if result.Status == "candidate" {
			candidates++
		}
		if info, err := os.Stat(result.Source); err == nil && info.IsDir() {
			directories = append(directories, result)
		} else {
			files = append(files, result)
		}
	}
	out := cmd.OutOrStdout()
	if len(results) == 0 {
		fmt.Fprintln(out, "No shared resources or supported projects found.")
	}
	printDiscoverTable(cmd, "Directories", directories)
	printDiscoverTable(cmd, "Files and unresolved paths", files)
	if len(projects) > 0 {
		labels := make([]string, 0, len(projects))
		for label, count := range projects {
			labels = append(labels, fmt.Sprintf("%s: %d", label, count))
		}
		sort.Strings(labels)
		fmt.Fprintf(out, "Projects: %s\n", strings.Join(labels, "; "))
	}
	if len(directories)+len(files) > 0 {
		fmt.Fprintln(out, "Sizes are logical bytes, not disk savings; + means a partial scan.")
		fmt.Fprintln(out, "Sharing = configured strategy; Current = state in this worktree.")
	}
	for _, result := range results {
		if result.ProjectMarker {
			continue
		}
		if (result.Status == "configured" && strings.Contains(result.Reason, ";")) ||
			result.Source == "" || result.TargetState == "broken-link" || result.TargetState == "unreadable" ||
			result.TargetState == "other-link" {
			fmt.Fprintf(out, "Note: %s — %s\n", result.Path, result.Reason)
			if result.Source != "" {
				fmt.Fprintf(out, "  Source: %s\n", result.Source)
			}
		}
	}
	for _, hook := range hooks {
		if !hook.Disabled {
			enabled++
		}
	}
	if len(hooks) > 0 {
		fmt.Fprintf(out, "\nHooks: %d enabled, %d disabled (creation only, after sharing).\n", enabled, len(hooks)-enabled)
		fmt.Fprintln(out, "  Inspect commands: gmc wt hook list")
	} else {
		fmt.Fprintln(out, "\nHooks: none configured.")
	}
	if candidates > 0 && !discoverAuto {
		fmt.Fprintf(out, "\nApply %d copy candidates and sync rules: gmc wt share discover --auto\n", candidates)
	} else {
		fmt.Fprintln(out, "\nRules: gmc wt share list\nSync:  gmc wt share sync")
	}
	fmt.Fprintln(out, "Full details: gmc wt share discover --output json")
}

func printDiscoverTable(cmd *cobra.Command, title string, results []worktree.DiscoverResult) {
	if len(results) == 0 {
		return
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].SizeBytes == results[j].SizeBytes {
			return results[i].Path < results[j].Path
		}
		return results[i].SizeBytes > results[j].SizeBytes
	})
	fmt.Fprintf(cmd.OutOrStdout(), "%s (%d)\n", title, len(results))
	table := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "SIZE\tSHARING\tCURRENT\tPATH")
	for _, result := range results {
		sharing := string(result.Strategy)
		switch result.Status {
		case "candidate":
			sharing = "copy available"
		case "managed":
			sharing = "not configured"
		case "excluded":
			sharing = "disabled"
		case "tracked":
			sharing = "Git-tracked"
		}
		current := strings.ReplaceAll(result.TargetState, "-", " ")
		if current == "" {
			current = "unknown"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", discoverSize(result), sharing, current, result.Path)
	}
	_ = table.Flush()
	fmt.Fprintln(cmd.OutOrStdout())
}

func discoverSize(result worktree.DiscoverResult) string {
	if result.Source == "" {
		return "unknown"
	}
	size := float64(result.SizeBytes)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	var text string
	if unit == 0 {
		text = fmt.Sprintf("%d B", result.SizeBytes)
	} else {
		text = fmt.Sprintf("%.1f %s", size, units[unit])
	}
	if result.SizeLimited {
		text += "+"
	}
	return text
}
