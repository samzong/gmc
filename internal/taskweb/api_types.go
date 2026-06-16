package taskweb

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/samzong/gmc/internal/task"
)

type ProjectInfo struct {
	Path              string `json:"path"`
	SuggestedTerminal string `json:"suggested_terminal"`
}

func BuildProjectInfo(repoPath string) (ProjectInfo, task.WorkflowDefinition, error) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return ProjectInfo{}, task.WorkflowDefinition{}, errors.New("repository path is required")
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return ProjectInfo{}, task.WorkflowDefinition{}, err
	}
	cfg, _, err := task.LoadWorkflowConfig()
	if err != nil {
		return ProjectInfo{}, task.WorkflowDefinition{}, err
	}
	wf, err := task.SelectWorkflow(cfg, "")
	if err != nil {
		return ProjectInfo{}, task.WorkflowDefinition{}, err
	}
	return ProjectInfo{
		Path:              abs,
		SuggestedTerminal: SuggestedTerminal(),
	}, wf, nil
}

type TaskCard struct {
	Index       int    `json:"index"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	State       string `json:"state"`
	CurrentNode string `json:"current_node,omitempty"`
	Agent       string `json:"agent,omitempty"`
}

type Handoff struct {
	Content string `json:"content"`
}

type TaskDetail struct {
	Source  string   `json:"source"`
	Handoff *Handoff `json:"handoff,omitempty"`
}

type WorkflowResponse struct {
	Start string                       `json:"start"`
	Order []string                     `json:"order"`
	Nodes map[string]task.WorkflowNode `json:"nodes"`
}

type AttachResponse struct {
	Opened bool   `json:"opened"`
	CLI    string `json:"cli,omitempty"`
	Error  string `json:"error,omitempty"`
}

type createTaskRequest struct {
	Source string `json:"source"`
}

type startTaskRequest struct {
	Agent      string `json:"agent,omitempty"`
	Command    string `json:"command,omitempty"`
	BaseBranch string `json:"base_branch,omitempty"`
}

type moveTaskRequest struct {
	To string `json:"to"`
}

type attachTaskRequest struct {
	Terminal string `json:"terminal"`
}

type removeTaskRequest struct {
	Force bool `json:"force,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}
