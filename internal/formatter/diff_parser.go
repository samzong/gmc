package formatter

import "strings"

func parseDiff(raw string) []DiffFile {
	if !strings.Contains(raw, "diff --") {
		return nil
	}

	lines := strings.Split(raw, "\n")
	var files []DiffFile
	var current *DiffFile

	for _, line := range lines {
		if isDiffHeader(line) {
			files = append(files, DiffFile{Header: line + "\n"})
			current = &files[len(files)-1]
			current.Path, current.OldPath = parseDiffHeaderPaths(line)
			if current.OldPath != "" && current.Path != "" && current.OldPath != current.Path {
				current.IsRename = true
			}
			continue
		}

		if current == nil {
			continue
		}

		if isHunkHeader(line) {
			current.Hunks = append(current.Hunks, line+"\n")
			continue
		}

		if len(current.Hunks) > 0 {
			current.Hunks[len(current.Hunks)-1] += line + "\n"
			continue
		}

		current.Header += line + "\n"
		applyHeaderLine(current, line)
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
	switch {
	case strings.HasPrefix(line, "rename from "):
		file.IsRename = true
		file.OldPath = strings.Trim(strings.TrimPrefix(line, "rename from "), "\"")
	case strings.HasPrefix(line, "rename to "):
		file.IsRename = true
		file.Path = strings.Trim(strings.TrimPrefix(line, "rename to "), "\"")
	case strings.HasPrefix(line, "new file mode "):
		file.IsNew = true
	case strings.HasPrefix(line, "old mode "), strings.HasPrefix(line, "new mode "):
		file.HasModeChange = true
	case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
		file.IsBinary = true
	}
}
