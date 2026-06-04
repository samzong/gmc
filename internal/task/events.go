package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	EventTaskCreated            = "task.created"
	EventAttemptCreated         = "attempt.created"
	EventRunStarted             = "run.started"
	EventRunCompleted           = "run.completed"
	EventRunFailed              = "run.failed"
	EventSessionAttached        = "session.attached"
	EventSessionDetached        = "session.detached"
	EventReviewFindingsRecorded = "review.findings_recorded"
	EventStageAdvanced          = "stage.advanced"
	EventTaskArchived           = "task.archived"
	EventGCOrphanDetected       = "gc.orphan_detected"
)

func (s *Store) appendEvent(taskID string, ev Event) error {
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}
	if ev.TaskID == "" {
		ev.TaskID = taskID
	}
	path := s.eventsPath(taskID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) listEvents(taskID string) ([]Event, error) {
	path := s.eventsPath(taskID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var events []Event
	for _, line := range splitLines(data) {
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("parse event log: %w", err)
		}
		events = append(events, ev)
	}
	return events, nil
}

func splitLines(data []byte) []string {
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func (s *Store) eventsPath(taskID string) string {
	return filepath.Join(s.taskDir(taskID), "events.jsonl")
}
