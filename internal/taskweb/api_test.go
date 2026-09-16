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

	resp := apiGet(t, srv, url+"/api/v1/project")
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var project ProjectInfo
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&project))
	assert.NotEmpty(t, project.Path)
	assert.Equal(t, "ghostty", project.SuggestedTerminal)

	resp2 := apiGet(t, srv, url+"/api/v1/workflow")
	t.Cleanup(func() { _ = resp2.Body.Close() })
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var wf WorkflowResponse
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&wf))
	assert.Equal(t, "plan", wf.Start)
	assert.Equal(t, []string{"plan", "code", "review", "ship"}, wf.Order)
}

func TestAPITaskCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	url := serveTest(t, srv)

	createBody := `{"source":"webui task"}`
	resp := apiPost(t, srv, url+"/api/v1/tasks", createBody)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	resp = apiGet(t, srv, url+"/api/v1/tasks")
	var cards []TaskCard
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cards))
	require.NoError(t, resp.Body.Close())
	require.Len(t, cards, 1)
	assert.Equal(t, 1, cards[0].Index)
	assert.Equal(t, "webui task", cards[0].Title)

	resp = apiGet(t, srv, url+"/api/v1/tasks/"+created.ID)
	var detail TaskDetail
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&detail))
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "webui task", detail.Source)

	resp = apiRequest(t, srv, http.MethodDelete, url+"/api/v1/tasks/"+created.ID, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestAPIAttachValidation(t *testing.T) {
	srv, root := newTestServer(t)
	url := serveTest(t, srv)

	resp := apiPost(t, srv, url+"/api/v1/tasks", `{"source":"attach test"}`)
	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	resp = apiPost(t, srv, url+"/api/v1/tasks/"+created.ID+"/attach", `{"terminal":"iterm2"}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	writeAttempt(t, root, created.ID, task.AttemptRecord{TmuxSession: "sess-demo", TmuxSocket: "gmc-task"})
	resp = apiPost(t, srv, url+"/api/v1/tasks/"+created.ID+"/attach", `{"terminal":"ghostty"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var attach AttachResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&attach))
	require.NoError(t, resp.Body.Close())
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

	createBody := `{"source":"launch"}`
	url := serveTest(t, srv)

	resp := apiPost(t, srv, url+"/api/v1/tasks", createBody)
	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	writeAttempt(t, root, created.ID, task.AttemptRecord{
		Worktree:    "/tmp/wt",
		TmuxSession: "sess-demo",
		TmuxSocket:  "gmc-task",
	})

	resp = apiPost(t, srv, url+"/api/v1/tasks/"+created.ID+"/attach", `{"terminal":"ghostty"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var attach AttachResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&attach))
	require.NoError(t, resp.Body.Close())
	assert.True(t, attach.Opened)
	assert.Contains(t, launched, "ghostty|")
	assert.Contains(t, launched, created.ID)
}

func TestAPIMoveRejectsUnknownNode(t *testing.T) {
	srv, root := newTestServer(t)
	url := serveTest(t, srv)

	createBody := `{"source":"move test"}`
	resp := apiPost(t, srv, url+"/api/v1/tasks", createBody)
	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	writeTaskState(t, root, task.Record{ID: created.ID, State: "plan", CurrentNode: "plan"})
	writeAttempt(t, root, created.ID, task.AttemptRecord{})

	resp = apiPost(t, srv, url+"/api/v1/tasks/"+created.ID+"/move", `{"to":"missing-node"}`)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestStaticIndex(t *testing.T) {
	srv, _ := newTestServer(t)
	url := serveTest(t, srv)

	resp, err := http.Get(url + "/")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "gmc task webui")

	resp, err = http.Get(url + "/app.js")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(url + "/app.css")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
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
