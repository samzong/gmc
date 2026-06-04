package task

import (
	"os"
	"path/filepath"
	"strings"
)

const artifactSummaryMax = 500

// WriteRunArtifact copies log output into the task artifacts directory.
func (s *Store) WriteRunArtifact(taskID, runID, logPath string) (relPath string, summary string, err error) {
	if err := os.MkdirAll(filepath.Join(s.taskDir(taskID), "artifacts"), 0o755); err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", "", err
	}
	relPath = filepath.Join("artifacts", runID+".md")
	abs := filepath.Join(s.taskDir(taskID), relPath)
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return "", "", err
	}
	return relPath, SummarizeLog(string(data)), nil
}

// SummarizeLog returns a short preview for show/list.
func SummarizeLog(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= 8 {
			break
		}
	}
	s := strings.Join(lines, "\n")
	if len(s) > artifactSummaryMax {
		return s[:artifactSummaryMax] + "…"
	}
	return s
}

// LatestStageResult picks the newest passed/failed review or check run.
func LatestStageResult(runs []RunRecord) *RunRecord {
	for i := len(runs) - 1; i >= 0; i-- {
		r := runs[i]
		switch r.Type {
		case RunTypeAgentReview, RunTypeCommandCheck:
			if r.State == RunPassed || r.State == RunFailed {
				return &runs[i]
			}
		}
	}
	return nil
}
