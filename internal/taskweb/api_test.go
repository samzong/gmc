package taskweb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/samzong/gmc/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func writeAttempt(t *testing.T, storeRoot, taskID string, attempt task.AttemptRecord) {
	t.Helper()
	attempt.TaskID = taskID
	if attempt.ID == "" {
		attempt.ID = "attempt-1"
	}
	now := time.Now().UTC()
	if attempt.CreatedAt.IsZero() {
		attempt.CreatedAt = now
	}
	attempt.UpdatedAt = now
	dir := filepath.Join(storeRoot, "gmc-tasks", "tasks", taskID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := yaml.Marshal(attempt)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "attempt.yaml"), data, 0o644))
}

func writeTaskState(t *testing.T, storeRoot string, rec task.Record) {
	t.Helper()
	dir := filepath.Join(storeRoot, "gmc-tasks", "tasks", rec.ID)
	path := filepath.Join(dir, "task.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var stored task.Record
	require.NoError(t, yaml.Unmarshal(data, &stored))
	stored.State = rec.State
	stored.CurrentNode = rec.CurrentNode
	stored.UpdatedAt = time.Now().UTC()
	out, err := yaml.Marshal(stored)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
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
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/v1/project")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var project ProjectInfo
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&project))
	assert.NotEmpty(t, project.Path)
	assert.Equal(t, "ghostty", project.SuggestedTerminal)

	resp2, err := http.Get(ts.URL + "/api/v1/workflow")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp2.Body.Close() })
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var wf WorkflowResponse
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&wf))
	assert.Equal(t, "plan", wf.Start)
	assert.Equal(t, []string{"plan", "code", "review", "ship"}, wf.Order)
}

func TestAPITaskCRUD(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	createBody := bytes.NewBufferString(`{"source":"webui task"}`)
	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", createBody)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	resp, err = http.Get(ts.URL + "/api/v1/tasks")
	require.NoError(t, err)
	var cards []TaskCard
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cards))
	require.NoError(t, resp.Body.Close())
	require.Len(t, cards, 1)
	assert.Equal(t, 1, cards[0].Index)
	assert.Equal(t, "webui task", cards[0].Title)

	resp, err = http.Get(ts.URL + "/api/v1/tasks/" + created.ID)
	require.NoError(t, err)
	var detail TaskDetail
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&detail))
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "webui task", detail.Source)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/tasks/"+created.ID, nil)
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestAPIAttachValidation(t *testing.T) {
	srv, root := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	createBody := bytes.NewBufferString(`{"source":"attach test"}`)
	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", createBody)
	require.NoError(t, err)
	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	body := bytes.NewBufferString(`{"terminal":"iterm2"}`)
	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+created.ID+"/attach", "application/json", body)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	writeAttempt(t, root, created.ID, task.AttemptRecord{TmuxSession: "sess-demo", TmuxSocket: "gmc-task"})
	body = bytes.NewBufferString(`{"terminal":"ghostty"}`)
	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+created.ID+"/attach", "application/json", body)
	require.NoError(t, err)
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

	createBody := bytes.NewBufferString(`{"source":"launch"}`)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", createBody)
	require.NoError(t, err)
	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	writeAttempt(t, root, created.ID, task.AttemptRecord{
		Worktree:    "/tmp/wt",
		TmuxSession: "sess-demo",
		TmuxSocket:  "gmc-task",
	})

	body := bytes.NewBufferString(`{"terminal":"ghostty"}`)
	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+created.ID+"/attach", "application/json", body)
	require.NoError(t, err)
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
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	createBody := bytes.NewBufferString(`{"source":"move test"}`)
	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", createBody)
	require.NoError(t, err)
	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	writeTaskState(t, root, task.Record{ID: created.ID, State: "plan", CurrentNode: "plan"})
	writeAttempt(t, root, created.ID, task.AttemptRecord{})

	body := bytes.NewBufferString(`{"to":"missing-node"}`)
	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+created.ID+"/move", "application/json", body)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestStaticIndex(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "gmc task webui")

	resp, err = http.Get(ts.URL + "/app.js")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = http.Get(ts.URL + "/app.css")
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
