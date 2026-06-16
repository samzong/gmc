package task

import (
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
	ErrNotFound  = errors.New("task not found")
	ErrNoAttempt = errors.New("attempt not found")
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

func (s *Store) Root() string {
	return s.root
}

func (s *Store) taskRoot() string {
	return filepath.Join(s.root, "tasks")
}

func (s *Store) taskDir(taskID string) string {
	return filepath.Join(s.taskRoot(), taskID)
}

func (s *Store) CreateTask(rec Record) error {
	if err := os.MkdirAll(s.taskDir(rec.ID), 0o755); err != nil {
		return err
	}
	return s.writeTask(rec)
}

func (s *Store) writeTask(rec Record) error {
	rec.UpdatedAt = time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.UpdatedAt
	}
	return writeYAML(filepath.Join(s.taskDir(rec.ID), "task.yaml"), rec)
}

func (s *Store) LoadTask(taskID string) (Record, error) {
	var rec Record
	if err := readYAML(filepath.Join(s.taskDir(taskID), "task.yaml"), &rec); err != nil {
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
	sort.Strings(ids)
	return ids, nil
}

func (s *Store) SaveAttempt(rec AttemptRecord) error {
	if err := os.MkdirAll(s.taskDir(rec.TaskID), 0o755); err != nil {
		return err
	}
	rec.UpdatedAt = time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.UpdatedAt
	}
	return writeYAML(filepath.Join(s.taskDir(rec.TaskID), "attempt.yaml"), rec)
}

func (s *Store) LoadAttempt(taskID string) (AttemptRecord, error) {
	var rec AttemptRecord
	if err := readYAML(filepath.Join(s.taskDir(taskID), "attempt.yaml"), &rec); err != nil {
		if os.IsNotExist(err) {
			return AttemptRecord{}, fmt.Errorf("%w: %s", ErrNoAttempt, taskID)
		}
		return AttemptRecord{}, err
	}
	return rec, nil
}

func (s *Store) LoadSummary(taskID string) (Summary, error) {
	rec, err := s.LoadTask(taskID)
	if err != nil {
		return Summary{}, err
	}
	attempt, err := s.LoadAttempt(taskID)
	if err != nil {
		if errors.Is(err, ErrNoAttempt) {
			return Summary{Task: rec}, nil
		}
		return Summary{}, err
	}
	return Summary{Task: rec, Attempt: &attempt}, nil
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
	if _, err := os.Stat(s.taskDir(taskID)); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrNotFound, taskID)
	}
	return os.RemoveAll(s.taskDir(taskID))
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
