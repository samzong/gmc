package formatter

import (
	"path/filepath"
	"strconv"
	"strings"
)

type DiffFile struct {
	Path          string
	OldPath       string
	Header        string
	Hunks         []string
	IsBinary      bool
	IsRename      bool
	Priority      int
	Added         int
	Deleted       int
	HasModeChange bool
}

type diffStat struct {
	Added    int
	Deleted  int
	IsBinary bool
}

var (
	lowPriorityPatterns = []string{
		"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"Pipfile.lock", "composer.lock", "Cargo.lock", "Gemfile.lock",
		"*.lock",
		"*.pb.go", "*_generated.go", "*_gen.go",
		"*.min.js", "*.min.css", "*.map",
	}
	lowPriorityDirs = []string{"vendor", "node_modules", "third_party"}
)

func truncateDiffWithStats(diff string, stats string, limit int) string {
	if len(diff) <= limit {
		return diff
	}

	files := parseDiff(diff)
	if len(files) == 0 {
		return truncateToValidUTF8(diff, limit) + "...(content is too long, truncated)"
	}

	statMap := parseNumstat(stats)
	for i := range files {
		files[i].Priority = classifyFile(files[i].Path)
		applyStats(&files[i], statMap)
		if files[i].Added == 0 && files[i].Deleted == 0 && len(files[i].Hunks) > 0 {
			files[i].Added, files[i].Deleted = countHunkChanges(files[i].Hunks)
		}
	}

	result := truncateDiff(files, limit)
	if result == "" {
		return truncateToValidUTF8(diff, limit) + "...(content is too long, truncated)"
	}
	return result
}

func parseDiff(raw string) []DiffFile {
	if !strings.Contains(raw, "diff --") {
		return nil
	}

	lines := strings.Split(raw, "\n")
	var files []DiffFile
	var current *DiffFile
	inHunk := false

	for _, line := range lines {
		if isDiffHeader(line) {
			if current != nil {
				files = append(files, *current)
			}
			current = &DiffFile{}
			inHunk = false
			current.Header = line + "\n"
			newPath, oldPath := parseDiffHeaderPaths(line)
			if oldPath != "" {
				current.OldPath = oldPath
			}
			if newPath != "" {
				current.Path = newPath
			}
			if current.OldPath != "" && current.Path != "" && current.OldPath != current.Path {
				current.IsRename = true
			}
			continue
		}

		if current == nil {
			continue
		}

		if isHunkHeader(line) {
			inHunk = true
			current.Hunks = append(current.Hunks, line+"\n")
			continue
		}

		if inHunk {
			current.Hunks[len(current.Hunks)-1] += line + "\n"
			continue
		}

		current.Header += line + "\n"
		applyHeaderLine(current, line)
	}

	if current != nil {
		files = append(files, *current)
	}

	return files
}

func isDiffHeader(line string) bool {
	return strings.HasPrefix(line, "diff --git ") ||
		strings.HasPrefix(line, "diff --cc ") ||
		strings.HasPrefix(line, "diff --combined ")
}

func isHunkHeader(line string) bool {
	return strings.HasPrefix(line, "@@ ") || strings.HasPrefix(line, "@@@ ")
}

func parseDiffHeaderPaths(line string) (string, string) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return "", ""
	}
	if fields[1] == "--cc" || fields[1] == "--combined" {
		path := strings.TrimSpace(fields[2])
		path = strings.Trim(path, "\"")
		return path, path
	}
	if len(fields) < 4 {
		return "", ""
	}
	oldPath := normalizeDiffPath(fields[2], "a/")
	newPath := normalizeDiffPath(fields[3], "b/")
	return newPath, oldPath
}

func normalizeDiffPath(path string, prefix string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "\"")
	path = strings.TrimPrefix(path, prefix)
	return path
}

func applyHeaderLine(file *DiffFile, line string) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "rename from ") {
		file.IsRename = true
		file.OldPath = strings.TrimPrefix(line, "rename from ")
		file.OldPath = strings.Trim(file.OldPath, "\"")
		return
	}
	if strings.HasPrefix(line, "rename to ") {
		file.IsRename = true
		file.Path = strings.TrimPrefix(line, "rename to ")
		file.Path = strings.Trim(file.Path, "\"")
		return
	}
	if strings.HasPrefix(line, "old mode ") ||
		strings.HasPrefix(line, "new mode ") ||
		strings.HasPrefix(line, "new file mode ") ||
		strings.HasPrefix(line, "deleted file mode ") {
		file.HasModeChange = true
		return
	}
	if strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") {
		file.IsBinary = true
	}
}

func parseNumstat(raw string) map[string]diffStat {
	stats := make(map[string]diffStat)
	if raw == "" {
		return stats
	}

	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		addedStr := parts[0]
		deletedStr := parts[1]
		pathPart := strings.TrimSpace(parts[2])
		stat := diffStat{}
		if addedStr == "-" && deletedStr == "-" {
			stat.IsBinary = true
		} else {
			added, err := strconv.Atoi(addedStr)
			if err == nil {
				stat.Added = added
			}
			deleted, err := strconv.Atoi(deletedStr)
			if err == nil {
				stat.Deleted = deleted
			}
		}

		oldPath, newPath, renamed := splitRenamePath(pathPart)
		if renamed {
			stats[newPath] = stat
			stats[oldPath] = stat
			continue
		}
		stats[pathPart] = stat
	}

	return stats
}

func splitRenamePath(path string) (string, string, bool) {
	if !strings.Contains(path, "=>") {
		return "", "", false
	}

	if strings.Contains(path, "{") && strings.Contains(path, "}") {
		oldPath := ""
		newPath := ""
		rest := path
		for {
			start := strings.Index(rest, "{")
			end := strings.Index(rest, "}")
			if start == -1 || end == -1 || end < start {
				oldPath += rest
				newPath += rest
				break
			}
			oldPath += rest[:start]
			newPath += rest[:start]
			segment := rest[start+1 : end]
			parts := strings.SplitN(segment, "=>", 2)
			if len(parts) != 2 {
				oldPath += "{" + segment + "}"
				newPath += "{" + segment + "}"
			} else {
				oldPath += strings.TrimSpace(parts[0])
				newPath += strings.TrimSpace(parts[1])
			}
			rest = rest[end+1:]
		}
		return strings.TrimSpace(oldPath), strings.TrimSpace(newPath), true
	}

	parts := strings.SplitN(path, "=>", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	oldPath := strings.TrimSpace(parts[0])
	newPath := strings.TrimSpace(parts[1])
	return oldPath, newPath, true
}

func applyStats(file *DiffFile, stats map[string]diffStat) {
	stat, ok := stats[file.Path]
	if !ok && file.OldPath != "" {
		stat, ok = stats[file.OldPath]
	}
	if !ok {
		return
	}
	file.Added = stat.Added
	file.Deleted = stat.Deleted
	if stat.IsBinary {
		file.IsBinary = true
	}
}

func classifyFile(filePath string) int {
	if filePath == "" {
		return 0
	}
	segments := strings.Split(filePath, "/")
	for _, seg := range segments {
		for _, dir := range lowPriorityDirs {
			if seg == dir {
				return 2
			}
		}
	}

	base := filepath.Base(filePath)
	for _, pattern := range lowPriorityPatterns {
		matched, err := filepath.Match(pattern, base)
		if err == nil && matched {
			return 2
		}
	}

	return 0
}

func countHunkChanges(hunks []string) (int, int) {
	added := 0
	deleted := 0
	for _, hunk := range hunks {
		for _, line := range strings.Split(hunk, "\n") {
			if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
				continue
			}
			if strings.HasPrefix(line, "+") {
				added++
			} else if strings.HasPrefix(line, "-") {
				deleted++
			}
		}
	}
	return added, deleted
}

func summarizeFile(file DiffFile) string {
	if file.IsBinary {
		return file.Path + " (binary)"
	}
	if file.IsRename && file.OldPath != "" && file.Path != "" {
		return file.OldPath + " -> " + file.Path + " (renamed)"
	}
	if file.HasModeChange {
		return file.Path + " (mode changed)"
	}
	return file.Path + " (+" + strconv.Itoa(file.Added) + "/-" + strconv.Itoa(file.Deleted) + ")"
}

func truncateDiff(files []DiffFile, limit int) string {
	var high []DiffFile
	var mid []DiffFile
	var low []DiffFile
	for _, file := range files {
		switch file.Priority {
		case 0:
			high = append(high, file)
		case 1:
			mid = append(mid, file)
		default:
			low = append(low, file)
		}
	}

	var result strings.Builder
	for _, file := range high {
		if !appendFile(&result, file, limit, false) {
			if !appendFile(&result, file, limit, true) {
				return truncateToValidUTF8(result.String(), limit)
			}
		}
	}

	for _, file := range mid {
		if !appendFile(&result, file, limit, false) {
			if !appendFile(&result, file, limit, true) {
				return truncateToValidUTF8(result.String(), limit)
			}
		}
	}

	for _, file := range low {
		summary := summarizeFile(file) + "\n"
		if result.Len()+len(summary) > limit {
			return truncateToValidUTF8(result.String(), limit)
		}
		result.WriteString(summary)
	}

	return truncateToValidUTF8(result.String(), limit)
}

func appendFile(builder *strings.Builder, file DiffFile, limit int, summaryOnly bool) bool {
	if summaryOnly {
		summary := summarizeFile(file) + "\n"
		if builder.Len()+len(summary) > limit {
			return false
		}
		builder.WriteString(summary)
		return true
	}

	if file.Header == "" {
		file.Header = "diff --git a/" + file.Path + " b/" + file.Path + "\n"
	}

	if builder.Len()+len(file.Header) > limit {
		return false
	}
	builder.WriteString(file.Header)

	for _, hunk := range file.Hunks {
		if builder.Len()+len(hunk) > limit {
			marker := "... (truncated)\n"
			if builder.Len()+len(marker) <= limit {
				builder.WriteString(marker)
			}
			return false
		}
		builder.WriteString(hunk)
	}

	return true
}
