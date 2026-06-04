package task

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/samzong/gmc/internal/worktree"
	"gopkg.in/yaml.v3"
)

var ErrNotFound = errors.New("task not found")

// Store owns the repo-family task ledger under <git-common-dir>/gmc-tasks/.
type Store struct {
	root string
}

// OpenStore resolves the ledger root from the current repository/worktree.
func OpenStore(wt *worktree.Client) (*Store, error) {
	commonDir, err := wt.GetGitCommonDir()
	if err != nil {
		return nil, err
	}
	return NewStore(commonDir), nil
}

// NewStore creates a ledger at <gitCommonDir>/gmc-tasks/.
func NewStore(gitCommonDir string) *Store {
	return &Store{root: filepath.Join(gitCommonDir, "gmc-tasks")}
}

func (s *Store) Root() string { return s.root }

func (s *Store) tasksRoot() string {
	return filepath.Join(s.root, "tasks")
}

func (s *Store) taskDir(taskID string) string {
	return filepath.Join(s.tasksRoot(), taskID)
}

func (s *Store) ensureTaskLayout(taskID string) error {
	dirs := []string{
		s.taskDir(taskID),
		filepath.Join(s.taskDir(taskID), "attempts"),
		filepath.Join(s.taskDir(taskID), "runs"),
		filepath.Join(s.taskDir(taskID), "logs"),
		filepath.Join(s.taskDir(taskID), "artifacts"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateTask(rec Record) error {
	if err := s.ensureTaskLayout(rec.ID); err != nil {
		return err
	}
	return s.writeTask(rec)
}

func (s *Store) writeTask(rec Record) error {
	rec.UpdatedAt = time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.UpdatedAt
	}
	path := filepath.Join(s.taskDir(rec.ID), "task.yaml")
	return writeYAML(path, rec)
}

func (s *Store) LoadTask(taskID string) (Record, error) {
	path := filepath.Join(s.taskDir(taskID), "task.yaml")
	var rec Record
	if err := readYAML(path, &rec); err != nil {
		if os.IsNotExist(err) {
			return Record{}, fmt.Errorf("%w: %s", ErrNotFound, taskID)
		}
		return Record{}, err
	}
	return rec, nil
}

func (s *Store) ListTaskIDs() ([]string, error) {
	entries, err := os.ReadDir(s.tasksRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, ent := range entries {
		if ent.IsDir() {
			ids = append(ids, ent.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *Store) SaveAttempt(rec AttemptRecord) error {
	if err := s.ensureTaskLayout(rec.TaskID); err != nil {
		return err
	}
	rec.UpdatedAt = time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.UpdatedAt
	}
	path := filepath.Join(s.taskDir(rec.TaskID), "attempts", rec.ID+".yaml")
	return writeYAML(path, rec)
}

func (s *Store) LoadAttempt(taskID, attemptID string) (AttemptRecord, error) {
	path := filepath.Join(s.taskDir(taskID), "attempts", attemptID+".yaml")
	var rec AttemptRecord
	if err := readYAML(path, &rec); err != nil {
		if os.IsNotExist(err) {
			return AttemptRecord{}, fmt.Errorf("attempt not found: %s", attemptID)
		}
		return AttemptRecord{}, err
	}
	return rec, nil
}

func (s *Store) ListAttempts(taskID string) ([]AttemptRecord, error) {
	dir := filepath.Join(s.taskDir(taskID), "attempts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []AttemptRecord
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(ent.Name(), ".yaml")
		rec, err := s.LoadAttempt(taskID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) SaveRun(rec RunRecord) error {
	if err := s.ensureTaskLayout(rec.TaskID); err != nil {
		return err
	}
	rec.UpdatedAt = time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.UpdatedAt
	}
	path := filepath.Join(s.taskDir(rec.TaskID), "runs", rec.ID+".yaml")
	return writeYAML(path, rec)
}

func (s *Store) LoadRun(taskID, runID string) (RunRecord, error) {
	path := filepath.Join(s.taskDir(taskID), "runs", runID+".yaml")
	var rec RunRecord
	if err := readYAML(path, &rec); err != nil {
		if os.IsNotExist(err) {
			return RunRecord{}, fmt.Errorf("run not found: %s", runID)
		}
		return RunRecord{}, err
	}
	return rec, nil
}

func (s *Store) ListRuns(taskID string) ([]RunRecord, error) {
	dir := filepath.Join(s.taskDir(taskID), "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []RunRecord
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(ent.Name(), ".yaml")
		rec, err := s.LoadRun(taskID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) LogPath(taskID, runID string) string {
	return filepath.Join(s.taskDir(taskID), "logs", runID+".log")
}

func (s *Store) LoadSummary(taskID string) (Summary, error) {
	task, err := s.LoadTask(taskID)
	if err != nil {
		return Summary{}, err
	}
	attempts, err := s.ListAttempts(taskID)
	if err != nil {
		return Summary{}, err
	}
	runs, err := s.ListRuns(taskID)
	if err != nil {
		return Summary{}, err
	}
	return Summary{Task: task, Attempts: attempts, Runs: runs}, nil
}

// RemoveTaskDir deletes the task ledger directory.
func (s *Store) RemoveTaskDir(taskID string) error {
	dir := s.taskDir(taskID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrNotFound, taskID)
	}
	return os.RemoveAll(dir)
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
	return os.WriteFile(path, data, 0o644)
}
