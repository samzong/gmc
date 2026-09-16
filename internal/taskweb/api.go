package taskweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/samzong/gmc/internal/task"
)

func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.project)
}

func (s *Server) handleWorkflow(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, WorkflowResponse{
		Start: s.workflow.Start,
		Order: task.WorkflowNodeOrder(s.workflow),
		Nodes: s.workflow.Nodes,
	})
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	summaries, err := s.engine.ListTasks()
	if err != nil {
		writeAPIError(w, err)
		return
	}
	cards := make([]TaskCard, 0, len(summaries))
	for i, sum := range summaries {
		cards = append(cards, taskCard(i+1, sum))
	}
	writeJSON(w, http.StatusOK, cards)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}
	rec, err := s.engine.CreateTask(source)
	writeResult(w, http.StatusCreated, rec, err)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sum, err := s.engine.ShowTask(id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	detail := TaskDetail{
		Source:  sum.Task.Source,
		Handoff: handoffForTask(sum),
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleStartTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req startTaskRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sum, err := s.engine.Start(task.StartOptions{
		TaskID:     id,
		Agent:      req.Agent,
		BaseBranch: req.BaseBranch,
	})
	writeResult(w, http.StatusOK, sum, err)
}

func (s *Server) handleMoveTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req moveTaskRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMoveTarget(s.workflow, req.To); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sum, err := s.engine.Advance(task.AdvanceOptions{TaskID: id, ToNode: req.To})
	writeResult(w, http.StatusOK, sum, err)
}

func (s *Server) handleAttachTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req attachTaskRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateTerminal(req.Terminal); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sum, err := s.engine.ShowTask(id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if sum.Attempt == nil || sum.Attempt.TmuxSession == "" {
		writeError(w, http.StatusBadRequest, "task has no tmux session")
		return
	}
	cli := attachCLICommand(s.opts.GMCBinary, sum.Task.ID)
	resp := AttachResponse{CLI: cli}
	workdir := s.project.Path
	if s.opts.TerminalLauncher != nil {
		if err := s.opts.TerminalLauncher(req.Terminal, cli, workdir); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Opened = true
			resp.CLI = ""
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRemoveTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	force := false
	if r.ContentLength > 0 {
		var req removeTaskRequest
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		force = req.Force
	}
	err := s.engine.Remove(id, task.RemoveOptions{Force: force})
	writeResult(w, http.StatusOK, map[string]string{"task_id": id, "action": "removed"}, err)
}

const maxRequestBody = 1 << 20

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	defer r.Body.Close()

	if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return fmt.Errorf("Content-Type must be application/json, got %q", contentType)
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeResult(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, status, value)
}

func writeAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, task.ErrInvalidTaskID):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, task.ErrNotFound), errors.Is(err, task.ErrNoAttempt):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		msg := err.Error()
		for _, hint := range []string{
			"not found", "only from new", "already started", "has not started",
			"already done", "no tmux session", "unsupported", "required",
		} {
			if strings.Contains(msg, hint) {
				writeError(w, http.StatusBadRequest, msg)
				return
			}
		}
		writeError(w, http.StatusInternalServerError, msg)
	}
}
