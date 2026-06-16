package task

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const workflowHandoffDir = ".gmc/workflow"

func ReadNodeHandoff(worktreePath, nodeID string) (content string, ok bool, err error) {
	worktreePath = strings.TrimSpace(worktreePath)
	nodeID = strings.TrimSpace(nodeID)
	if worktreePath == "" {
		return "", false, errors.New("worktree path is required")
	}
	if nodeID == "" {
		return "", false, errors.New("node id is required")
	}
	path := filepath.Join(worktreePath, workflowHandoffDir, nodeID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}
