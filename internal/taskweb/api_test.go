package taskweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testServerOptions() Options {
	return Options{
		GMCBinary: "/bin/gmc",
		TerminalLauncher: func(string, string, string) error {
			return errors.New("test stub")
		},
	}
}

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	engine := task.NewEngine(task.NewStore(root), nil)
	srv, err := New(engine, root, testServerOptions())
	require.NoError(t, err)
	return srv, root
}

func apiRequest(t *testing.T, srv *Server, method, url, body string) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	req.Header.Set(tokenHeader, srv.token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func apiGet(t *testing.T, srv *Server, url string) *http.Response {
	t.Helper()
	return apiRequest(t, srv, http.MethodGet, url, "")
}

func apiPost(t *testing.T, srv *Server, url, body string) *http.Response {
	t.Helper()
	return apiRequest(t, srv, http.MethodPost, url, body)
}

func readJSONResponse[T any](t *testing.T, resp *http.Response, status int) T {
	t.Helper()
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	require.Equal(t, status, resp.StatusCode)
	var value T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&value))
	return value
}

func createAPITask(t *testing.T, srv *Server, url, source string) task.Record {
	t.Helper()
	body, err := json.Marshal(map[string]string{"source": source})
	require.NoError(t, err)
	return readJSONResponse[task.Record](t, apiPost(t, srv, url+"/api/v1/tasks", string(body)), http.StatusCreated)
}

func writeAttempt(t *testing.T, storeRoot, taskID string, attempt task.AttemptRecord) {
	t.Helper()
	attempt.TaskID = taskID
	if attempt.ID == "" {
		attempt.ID = "attempt-1"
	}
	require.NoError(t, task.NewStore(storeRoot).SaveAttempt(attempt))
}

func writeTaskState(t *testing.T, storeRoot string, rec task.Record) {
	t.Helper()
	store := task.NewStore(storeRoot)
	stored, err := store.LoadTask(rec.ID)
	require.NoError(t, err)
	stored.State = rec.State
	stored.CurrentNode = rec.CurrentNode
	require.NoError(t, store.CreateTask(stored))
}

func serveTest(t *testing.T, srv *Server) string {
	t.Helper()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestListenLoopbackOnly(t *testing.T) {
	srv, _ := newTestServer(t)
	_, err := srv.Listen("0.0.0.0:0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loopback")
}

func TestListenBindsLoopback(t *testing.T) {
	srv, _ := newTestServer(t)
	url, err := srv.Listen("127.0.0.1:0")
	require.NoError(t, err)
	assert.Contains(t, url, "127.0.0.1:")
	host, _, err := net.SplitHostPort(url[len("http://"):])
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", host)
}

func TestListenLoopbackPreferredPort(t *testing.T) {
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := hold.Addr().(*net.TCPAddr).Port
	require.NoError(t, hold.Close())

	srv, _ := newTestServer(t)
	url, usedPreferred, err := srv.ListenLoopback(port)
	require.NoError(t, err)
	assert.True(t, usedPreferred)
	assert.Contains(t, url, fmt.Sprintf("127.0.0.1:%d", port))
}

func TestListenLoopbackFallsBackWhenPreferredTaken(t *testing.T) {
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := hold.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { _ = hold.Close() })

	srv, _ := newTestServer(t)
	url, usedPreferred, err := srv.ListenLoopback(port)
	require.NoError(t, err)
	assert.False(t, usedPreferred)
	assert.Contains(t, url, "127.0.0.1:")
	host, gotPort, err := net.SplitHostPort(url[len("http://"):])
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", host)
	assert.NotEqual(t, strconv.Itoa(port), gotPort)
}

func TestAPIProjectAndWorkflow(t *testing.T) {
	srv, _ := newTestServer(t)
	url := serveTest(t, srv)

	project := readJSONResponse[ProjectInfo](t, apiGet(t, srv, url+"/api/v1/project"), http.StatusOK)
	assert.NotEmpty(t, project.Path)
	assert.Equal(t, "ghostty", project.SuggestedTerminal)

	wf := readJSONResponse[WorkflowResponse](t, apiGet(t, srv, url+"/api/v1/workflow"), http.StatusOK)
	assert.Equal(t, "plan", wf.Start)
	assert.Equal(t, []string{"plan", "code", "review", "ship"}, wf.Order)
}

func TestAPITaskCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	url := serveTest(t, srv)

	created := createAPITask(t, srv, url, "webui task")
	cards := readJSONResponse[[]TaskCard](t, apiGet(t, srv, url+"/api/v1/tasks"), http.StatusOK)
	require.Len(t, cards, 1)
	assert.Equal(t, 1, cards[0].Index)
	assert.Equal(t, "webui task", cards[0].Title)

	detail := readJSONResponse[TaskDetail](t, apiGet(t, srv, url+"/api/v1/tasks/"+created.ID), http.StatusOK)
	assert.Equal(t, "webui task", detail.Source)

	resp := apiRequest(t, srv, http.MethodDelete, url+"/api/v1/tasks/"+created.ID, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestAPIAttachValidation(t *testing.T) {
	srv, root := newTestServer(t)
	url := serveTest(t, srv)

	created := createAPITask(t, srv, url, "attach test")
	resp := apiPost(t, srv, url+"/api/v1/tasks/"+created.ID+"/attach", `{"terminal":"iterm2"}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	writeAttempt(t, root, created.ID, task.AttemptRecord{TmuxSession: "sess-demo", TmuxSocket: "gmc-task"})
	resp = apiPost(t, srv, url+"/api/v1/tasks/"+created.ID+"/attach", `{"terminal":"ghostty"}`)
	attach := readJSONResponse[AttachResponse](t, resp, http.StatusOK)
	assert.False(t, attach.Opened)
	assert.Contains(t, attach.CLI, "task attach")
	assert.Contains(t, attach.CLI, created.ID)
}

func TestAPIAttachWithLauncher(t *testing.T) {
	root := t.TempDir()
	var launched string
	engine := task.NewEngine(task.NewStore(root), nil)
	srv, err := New(engine, root, Options{
		GMCBinary: "/bin/gmc",
		TerminalLauncher: func(terminal, command, workdir string) error {
			launched = terminal + "|" + command + "|" + workdir
			return nil
		},
	})
	require.NoError(t, err)

	url := serveTest(t, srv)
	created := createAPITask(t, srv, url, "launch")

	writeAttempt(t, root, created.ID, task.AttemptRecord{
		Worktree:    "/tmp/wt",
		TmuxSession: "sess-demo",
		TmuxSocket:  "gmc-task",
	})

	resp := apiPost(t, srv, url+"/api/v1/tasks/"+created.ID+"/attach", `{"terminal":"ghostty"}`)
	attach := readJSONResponse[AttachResponse](t, resp, http.StatusOK)
	assert.True(t, attach.Opened)
	assert.Contains(t, launched, "ghostty|")
	assert.Contains(t, launched, created.ID)
}

func TestAPIMoveRejectsUnknownNode(t *testing.T) {
	srv, root := newTestServer(t)
	url := serveTest(t, srv)

	created := createAPITask(t, srv, url, "move test")

	writeTaskState(t, root, task.Record{ID: created.ID, State: "plan", CurrentNode: "plan"})
	writeAttempt(t, root, created.ID, task.AttemptRecord{})

	resp := apiPost(t, srv, url+"/api/v1/tasks/"+created.ID+"/move", `{"to":"missing-node"}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestOpenBrowserUsesOpener(t *testing.T) {
	srv, _ := newTestServer(t)
	opened := ""
	_, err := srv.Listen("127.0.0.1:0")
	require.NoError(t, err)
	srv.opts.BrowserOpener = func(url string) error {
		opened = url
		return nil
	}
	require.NoError(t, srv.OpenBrowser())
	assert.NotEmpty(t, opened)
}
