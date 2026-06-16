package taskweb

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/samzong/gmc/internal/task"
)

type Options struct {
	BrowserOpener    func(url string) error
	GMCBinary        string
	TerminalLauncher func(terminal, command, workdir string) error
}

type Server struct {
	engine   *task.Engine
	project  ProjectInfo
	workflow task.WorkflowDefinition
	opts     Options
	mux      *http.ServeMux
	http     *http.Server
	listener net.Listener
	url      string
}

func New(engine *task.Engine, repoPath string, opts Options) (*Server, error) {
	if engine == nil {
		return nil, errors.New("task engine is required")
	}
	project, wf, err := BuildProjectInfo(repoPath)
	if err != nil {
		return nil, err
	}
	if opts.GMCBinary == "" {
		binary, err := os.Executable()
		if err != nil {
			return nil, err
		}
		opts.GMCBinary = binary
	}
	if opts.TerminalLauncher == nil {
		opts.TerminalLauncher = LaunchTerminal
	}
	s := &Server{
		engine:   engine,
		project:  project,
		workflow: wf,
		opts:     opts,
		mux:      http.NewServeMux(),
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

const PreferredWebUIPort = 24508

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil && errors.Is(opErr.Err, syscall.EADDRINUSE) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "address already in use")
}

func (s *Server) ListenLoopback(preferredPort int) (url string, usedPreferred bool, err error) {
	if preferredPort > 0 {
		url, err = s.Listen(fmt.Sprintf("127.0.0.1:%d", preferredPort))
		if err == nil {
			return url, true, nil
		}
		if !isAddrInUse(err) {
			return "", false, err
		}
	}
	url, err = s.Listen("127.0.0.1:0")
	if err != nil {
		return "", false, err
	}
	return url, false, nil
}

func (s *Server) Listen(hostPort string) (string, error) {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", err
	}
	if host != "127.0.0.1" && host != "localhost" {
		return "", fmt.Errorf("refusing to bind outside loopback: %s", hostPort)
	}
	ln, err := net.Listen("tcp", hostPort)
	if err != nil {
		return "", err
	}
	s.listener = ln
	s.url = "http://" + ln.Addr().String()
	s.http = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s.url, nil
}

func (s *Server) Serve() error {
	if s.http == nil || s.listener == nil {
		return errors.New("server is not listening")
	}
	err := s.http.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) OpenBrowser() error {
	if s.url == "" {
		return errors.New("server URL is not available")
	}
	opener := s.opts.BrowserOpener
	if opener == nil {
		opener = DefaultBrowserOpener
	}
	return opener(s.url)
}

func (s *Server) ProjectPath() string {
	return s.project.Path
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/v1/project", s.handleProject)
	s.mux.HandleFunc("GET /api/v1/workflow", s.handleWorkflow)
	s.mux.HandleFunc("GET /api/v1/tasks", s.handleListTasks)
	s.mux.HandleFunc("POST /api/v1/tasks", s.handleCreateTask)
	s.mux.HandleFunc("GET /api/v1/tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/start", s.handleStartTask)
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/move", s.handleMoveTask)
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/attach", s.handleAttachTask)
	s.mux.HandleFunc("DELETE /api/v1/tasks/{id}", s.handleRemoveTask)
	s.registerStaticRoutes()
}

func (s *Server) registerStaticRoutes() {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("taskweb static files: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	s.mux.Handle("GET /{$}", fileServer)
	s.mux.Handle("GET /app.js", fileServer)
	s.mux.Handle("GET /app.css", fileServer)
}

func taskCard(index int, sum task.Summary) TaskCard {
	card := TaskCard{
		Index:       index,
		ID:          sum.Task.ID,
		Title:       task.DisplayTitle(sum.Task),
		State:       sum.Task.State,
		CurrentNode: sum.Task.CurrentNode,
	}
	if sum.Attempt != nil {
		card.Agent = sum.Attempt.Agent
	}
	return card
}

func handoffForTask(sum task.Summary) *Handoff {
	if sum.Attempt == nil || sum.Attempt.Worktree == "" {
		return nil
	}
	nodeID := strings.TrimSpace(sum.Task.CurrentNode)
	if nodeID == "" {
		nodeID = strings.TrimSpace(sum.Task.State)
	}
	if nodeID == "" || nodeID == task.TaskNew || nodeID == "done" {
		return nil
	}
	content, ok, err := task.ReadNodeHandoff(sum.Attempt.Worktree, nodeID)
	if err != nil || !ok {
		return nil
	}
	return &Handoff{Content: content}
}

func validateMoveTarget(wf task.WorkflowDefinition, to string) error {
	to = strings.TrimSpace(to)
	if to == "" {
		return errors.New("to is required")
	}
	if to == "done" {
		return nil
	}
	if _, ok := wf.Nodes[to]; !ok {
		return fmt.Errorf("unknown workflow node %q", to)
	}
	return nil
}

func validateTerminal(id string) error {
	switch strings.TrimSpace(id) {
	case "ghostty", "iterm", "terminal":
		return nil
	default:
		return fmt.Errorf("unsupported terminal %q", id)
	}
}
