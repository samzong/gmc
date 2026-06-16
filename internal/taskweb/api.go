package taskweb

import (
	"encoding/json"
	"errors"
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
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}
	rec, err := s.engine.CreateTask(source)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
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
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sum, err := s.engine.Start(task.StartOptions{
		TaskID:     id,
		Agent:      req.Agent,
		Command:    req.Command,
		BaseBranch: req.BaseBranch,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleMoveTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req moveTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMoveTarget(s.workflow, req.To); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sum, err := s.engine.Advance(task.AdvanceOptions{TaskID: id, ToNode: req.To})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleAttachTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req attachTaskRequest
	if err := decodeJSON(r, &req); err != nil {
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
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		force = req.Force
	}
	if err := s.engine.Remove(id, task.RemoveOptions{Force: force}); err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"task_id": id, "action": "removed"})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
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

func writeAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, task.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, task.ErrNoAttempt):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		msg := err.Error()
		if strings.Contains(msg, "not found") ||
			strings.Contains(msg, "only from new") ||
			strings.Contains(msg, "already started") ||
			strings.Contains(msg, "has not started") ||
			strings.Contains(msg, "already done") ||
			strings.Contains(msg, "no tmux session") ||
			strings.Contains(msg, "unsupported") ||
			strings.Contains(msg, "required") {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeError(w, http.StatusInternalServerError, msg)
	}
}
