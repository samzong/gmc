package task

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func (e *Engine) Refresh(taskRef string) ([]RefreshResult, error) {
	var ids []string
	if strings.TrimSpace(taskRef) == "" {
		var err error
		ids, err = e.store.ListTaskIDs()
		if err != nil {
			return nil, err
		}
	} else {
		taskID, err := e.store.ResolveTaskID(taskRef)
		if err != nil {
			return nil, err
		}
		ids = []string{taskID}
	}
	results := make([]RefreshResult, 0, len(ids))
	for _, taskID := range ids {
		res, err := e.refreshTask(taskID)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

func (e *Engine) refreshTask(taskID string) (RefreshResult, error) {
	rec, err := e.store.LoadTask(taskID)
	if err != nil {
		return RefreshResult{}, err
	}
	res := RefreshResult{TaskID: taskID, State: rec.State, Session: "none", WorktreeStatus: "none"}
	attempt, err := e.store.LoadAttempt(taskID)
	hasAttempt := true
	if err != nil {
		if !errors.Is(err, ErrNoAttempt) {
			return RefreshResult{}, err
		}
		hasAttempt = false
	}
	if hasAttempt {
		res.Worktree = attempt.Worktree
		res.WorktreeStatus = e.worktreeStatus(attempt.Worktree)
	}
	runs, err := e.store.LoadRuns(taskID)
	if err != nil {
		return RefreshResult{}, err
	}
	for i := range runs {
		run := &runs[i]
		if run.Status != RunStatusRunning {
			continue
		}
		var status, eventType string
		switch {
		case run.Runtime == RunRuntimeTmux && run.Session != "":
			profile := TmuxProfile{Session: run.Session, Socket: run.Socket}
			if tmuxReachable() && !tmuxSessionAlive(profile) {
				status, eventType = RunStatusExited, EventRunExited
			}
		case run.Runtime == RunRuntimeHeadless && run.PID > 0:
			if !processAlive(run.PID) {
				status, eventType = RunStatusLost, EventRunLost
			}
		}
		if status == "" {
			continue
		}
		fresh, err := e.store.LoadRuns(taskID)
		if err != nil {
			return RefreshResult{}, err
		}
		stillRunning := false
		for _, f := range fresh {
			if f.ID == run.ID && f.Status == RunStatusRunning {
				stillRunning = true
				break
			}
		}
		if !stillRunning {
			continue
		}
		run.Status = status
		ended := time.Now().UTC()
		run.EndedAt = &ended
		if err := e.store.SaveRun(*run); err != nil {
			return RefreshResult{}, err
		}
		res.Updated = append(res.Updated, fmt.Sprintf("%s: %s → %s", run.ID, RunStatusRunning, status))
		if err := storeAppendEvent(e.store, EventRecord{
			Type:      eventType,
			TaskID:    taskID,
			AttemptID: run.AttemptID,
			RunID:     run.ID,
			Node:      run.Node,
			Status:    status,
		}); err != nil {
			res.Updated = append(res.Updated, fmt.Sprintf("%s: warning: %v", run.ID, err))
		}
	}
	if hasAttempt {
		if profile, ok := latestAgentSession(attempt, runs); ok {
			switch {
			case !tmuxReachable():
				res.Session = "unknown"
			case tmuxSessionAlive(profile):
				res.Session = "alive"
			default:
				res.Session = "gone"
			}
		}
	}
	return res, nil
}

func (e *Engine) GC(opts GCOptions) ([]GCItem, error) {
	ids, err := e.store.ListTaskIDs()
	if err != nil {
		return nil, err
	}
	referenced := map[string]bool{}
	profiles := map[string]TmuxProfile{}
	var items []GCItem
	for _, taskID := range ids {
		rec, err := e.store.LoadTask(taskID)
		if err != nil {
			return nil, err
		}
		attempt, attemptErr := e.store.LoadAttempt(taskID)
		hasAttempt := true
		if attemptErr != nil {
			if !errors.Is(attemptErr, ErrNoAttempt) {
				return nil, attemptErr
			}
			hasAttempt = false
		}
		runs, err := e.store.LoadRuns(taskID)
		if err != nil {
			return nil, err
		}
		current := strings.TrimSpace(rec.CurrentNode)
		if current == "" {
			current = rec.State
		}
		latest, hasLatest := TmuxProfile{}, false
		if hasAttempt {
			latest, hasLatest = latestAgentSession(attempt, runs)
		}
		seen := map[string]bool{}
		var refs []gcSessionRef
		addRef := func(profile TmuxProfile, node string) {
			if profile.Session == "" || seen[profile.Session] {
				return
			}
			seen[profile.Session] = true
			refs = append(refs, gcSessionRef{profile: profile, node: node})
		}
		if hasAttempt {
			for _, session := range attempt.TmuxSessions {
				addRef(TmuxProfile{Session: session.Session, Socket: session.Socket}, session.Node)
			}
			addRef(TmuxProfile{Session: attempt.TmuxSession, Socket: attempt.TmuxSocket}, "")
			for _, run := range runs {
				addRef(TmuxProfile{Session: run.Session, Socket: run.Socket}, run.Node)
			}
		}
		for _, ref := range refs {
			referenced[ref.profile.Session] = true
			profiles[ref.profile.Session] = ref.profile
			if !tmuxSessionAlive(ref.profile) {
				continue
			}
			item := GCItem{TaskID: taskID, Kind: "tmux-session", Target: ref.profile.Session}
			switch {
			case tmuxSessionAttached(ref.profile):
				item.Action = "keep"
				item.Reason = "attached"
			case rec.State == "done":
				item.Action = "kill"
				item.Reason = "task is done"
			case !hasLatest || ref.profile.Session != latest.Session:
				item.Action = "kill"
				node := ref.node
				if node == "" {
					node = "unknown"
				}
				item.Reason = fmt.Sprintf("node %s finished; task is at %s", node, current)
			default:
				item.Action = "keep"
				item.Reason = fmt.Sprintf("task in progress at %s", current)
			}
			items = append(items, item)
		}
		if hasAttempt {
			switch status := e.worktreeStatus(attempt.Worktree); {
			case status == "missing":
				items = append(items, GCItem{
					TaskID: taskID, Kind: "worktree", Target: attempt.Worktree,
					Action: "keep",
					Reason: fmt.Sprintf("worktree missing; remove the record with: gmc task rm %s", taskID),
				})
			case status == "unknown":
				items = append(items, GCItem{
					TaskID: taskID, Kind: "worktree", Target: attempt.Worktree,
					Action: "keep", Reason: "could not determine worktree status",
				})
			case status != "clean":
				items = append(items, GCItem{
					TaskID: taskID, Kind: "worktree", Target: attempt.Worktree,
					Action: "keep", Reason: "uncommitted changes",
				})
			case rec.State == "done":
				items = append(items, GCItem{
					TaskID: taskID, Kind: "worktree", Target: attempt.Worktree,
					Action: "suggest",
					Reason: fmt.Sprintf("task done and worktree clean; remove with: gmc task rm %s", taskID),
				})
			}
		}
	}
	live, err := tmuxListSessions(gmcTmuxSocket)
	if err != nil {
		return nil, err
	}
	for _, name := range live {
		if referenced[name] {
			continue
		}
		items = append(items, GCItem{
			Kind:   "tmux-session",
			Target: name,
			Action: "keep",
			Reason: "not referenced by any task",
		})
	}
	if opts.Apply {
		for i := range items {
			if items[i].Action != "kill" {
				continue
			}
			profile, ok := profiles[items[i].Target]
			if !ok {
				profile = TmuxProfile{Session: items[i].Target, Socket: gmcTmuxSocket}
			}
			if err := killTmuxSession(profile); err != nil {
				items[i].Result = err.Error()
				continue
			}
			items[i].Result = "killed"
			if err := storeAppendEvent(e.store, EventRecord{
				Type:    EventGCKilled,
				TaskID:  items[i].TaskID,
				Message: items[i].Target,
			}); err != nil {
				items[i].Result = fmt.Sprintf("killed; event warning: %v", err)
			}
		}
	}
	return items, nil
}

type gcSessionRef struct {
	profile TmuxProfile
	node    string
}

func (e *Engine) worktreeStatus(path string) string {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return "missing"
	}
	if e.wt == nil {
		return "unknown"
	}
	return e.wt.GetWorktreeStatus(path)
}

func latestAgentSession(attempt AttemptRecord, runs []RunRecord) (TmuxProfile, bool) {
	for i := len(attempt.TmuxSessions) - 1; i >= 0; i-- {
		if attempt.TmuxSessions[i].Session != "" {
			return TmuxProfile{
				Session: attempt.TmuxSessions[i].Session,
				Socket:  attempt.TmuxSessions[i].Socket,
			}, true
		}
	}
	if attempt.TmuxSession != "" {
		return TmuxProfile{Session: attempt.TmuxSession, Socket: attempt.TmuxSocket}, true
	}
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Kind == RunKindAgent && runs[i].Session != "" {
			return TmuxProfile{Session: runs[i].Session, Socket: runs[i].Socket}, true
		}
	}
	return TmuxProfile{}, false
}
