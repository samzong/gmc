package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/worktree"
	"gopkg.in/yaml.v3"
)

var (
	ErrNotFound      = errors.New("task not found")
	ErrNoAttempt     = errors.New("attempt not found")
	ErrInvalidTaskID = errors.New("invalid task id")
)

type Store struct {
	root string
}

func OpenStore(wt *worktree.Client) (*Store, error) {
	commonDir, err := wt.GetGitCommonDir()
	if err != nil {
		return nil, err
	}
	return NewStore(commonDir), nil
}

func NewStore(gitCommonDir string) *Store {
	return &Store{root: filepath.Join(gitCommonDir, "gmc-tasks")}
}

func (s *Store) taskRoot() string {
	return filepath.Join(s.root, "tasks")
}

func (s *Store) taskDir(taskID string) (string, error) {
	root := s.taskRoot()
	dir := filepath.Join(root, filepath.FromSlash(taskID))

	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrInvalidTaskID, taskID)
	}
	return dir, nil
}

func (s *Store) CreateTask(rec Record) error {
	dir, err := s.taskDir(rec.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return s.writeTask(rec)
}

func (s *Store) writeTask(rec Record) error {
	dir, err := s.taskDir(rec.ID)
	if err != nil {
		return err
	}
	rec.UpdatedAt = time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.UpdatedAt
	}
	return writeYAML(filepath.Join(dir, "task.yaml"), rec)
}

func (s *Store) LoadTask(taskID string) (Record, error) {
	dir, err := s.taskDir(taskID)
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := readYAML(filepath.Join(dir, "task.yaml"), &rec); err != nil {
		if os.IsNotExist(err) {
			return Record{}, fmt.Errorf("%w: %s", ErrNotFound, taskID)
		}
		return Record{}, err
	}
	return rec, nil
}

func (s *Store) ListTaskIDs() ([]string, error) {
	entries, err := os.ReadDir(s.taskRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, ent := range entries {
		if ent.IsDir() {
			ids = append(ids, ent.Name())
		}
	}
	return ids, nil
}

func (s *Store) SaveAttempt(rec AttemptRecord) error {
	dir, err := s.taskDir(rec.TaskID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	rec.UpdatedAt = time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.UpdatedAt
	}
	return writeYAML(filepath.Join(dir, "attempt.yaml"), rec)
}

func (s *Store) LoadAttempt(taskID string) (AttemptRecord, error) {
	dir, err := s.taskDir(taskID)
	if err != nil {
		return AttemptRecord{}, err
	}
	var rec AttemptRecord
	if err := readYAML(filepath.Join(dir, "attempt.yaml"), &rec); err != nil {
		if os.IsNotExist(err) {
			return AttemptRecord{}, fmt.Errorf("%w: %s", ErrNoAttempt, taskID)
		}
		return AttemptRecord{}, err
	}
	return rec, nil
}

func (s *Store) SaveRun(run RunRecord) error {
	dir, err := s.taskDir(run.TaskID)
	if err != nil {
		return err
	}
	runsDir := filepath.Join(dir, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return err
	}
	return writeYAML(filepath.Join(runsDir, run.ID+".yaml"), run)
}

func (s *Store) LoadRuns(taskID string) ([]RunRecord, error) {
	runs, _, err := s.loadRuns(taskID)
	return runs, err
}

func (s *Store) loadRuns(taskID string) ([]RunRecord, []string, error) {
	dir, err := s.taskDir(taskID)
	if err != nil {
		return nil, nil, err
	}
	runsDir := filepath.Join(dir, "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var runs []RunRecord
	var warnings []string
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(runsDir, ent.Name())
		var run RunRecord
		if err := readYAML(path, &run); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped corrupt run file %s: %v", path, err))
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].StartedAt.Equal(runs[j].StartedAt) {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].StartedAt.Before(runs[j].StartedAt)
	})
	return runs, warnings, nil
}

func (s *Store) AppendEvent(ev EventRecord) error {
	dir, err := s.taskDir(ev.TaskID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func (s *Store) LoadEvents(taskID string) ([]EventRecord, error) {
	dir, err := s.taskDir(taskID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var events []EventRecord
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev EventRecord
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func RunLogRelPath(runID, stream string) string {
	return filepath.ToSlash(filepath.Join("logs", runID+"."+stream+".log"))
}

func (s *Store) RunLogPath(taskID, relPath string) (string, error) {
	dir, err := s.taskDir(taskID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.FromSlash(relPath)), nil
}

func (s *Store) EventsPath(taskID string) (string, error) {
	dir, err := s.taskDir(taskID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "events.jsonl"), nil
}

func (s *Store) LoadSummary(taskID string) (Summary, error) {
	rec, err := s.LoadTask(taskID)
	if err != nil {
		return Summary{}, err
	}
	runs, warnings, err := s.loadRuns(taskID)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{Task: rec, Runs: runs, Warnings: warnings}
	attempt, err := s.LoadAttempt(taskID)
	if err != nil {
		if errors.Is(err, ErrNoAttempt) {
			return sum, nil
		}
		return Summary{}, err
	}
	sum.Attempt = &attempt
	return sum, nil
}

func (s *Store) ListSummaries() ([]Summary, error) {
	ids, err := s.ListTaskIDs()
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(ids))
	for _, id := range ids {
		sum, err := s.LoadSummary(id)
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return out, nil
}

func (s *Store) ResolveTaskID(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("task id is required")
	}
	if _, err := s.LoadTask(ref); err == nil {
		return ref, nil
	} else if errors.Is(err, ErrInvalidTaskID) {
		return "", err
	}
	ids, err := s.ListTaskIDs()
	if err != nil {
		return "", err
	}
	if index, err := strconv.Atoi(ref); err == nil {
		if index < 1 || index > len(ids) {
			if len(ids) == 0 {
				return "", fmt.Errorf("task index %d out of range (no tasks)", index)
			}
			return "", fmt.Errorf("task index %d out of range (use 1-%d)", index, len(ids))
		}
		return ids[index-1], nil
	}
	var matches []string
	for _, id := range ids {
		if strings.HasPrefix(id, ref) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: %s", ErrNotFound, ref)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous task id %q (matches: %s)", ref, strings.Join(matches, ", "))
	}
}

func (s *Store) RemoveTask(taskID string) error {
	dir, err := s.taskDir(taskID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrNotFound, taskID)
	}
	return os.RemoveAll(dir)
}

func readYAML(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, dest)
}

func writeYAML(path string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
