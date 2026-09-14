package taskweb

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samzong/gmc/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardRequiresSessionToken(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/v1/project")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "an anonymous read must not be served")

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/project", nil)
	require.NoError(t, err)
	req.Header.Set(tokenHeader, "not-the-token")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGuardRejectsCrossOriginRequests(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks", strings.NewReader(`{"source":"csrf"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set(tokenHeader, srv.token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestGuardAllowsSameOriginRequests(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/project", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", ts.URL)
	req.Header.Set(tokenHeader, srv.token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGuardRejectsForeignHost(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	require.NoError(t, err)
	req.Host = "evil.example"

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHostAllowed(t *testing.T) {
	srv := &Server{port: "24508"}

	assert.True(t, srv.hostAllowed("127.0.0.1:24508"))
	assert.True(t, srv.hostAllowed("localhost:24508"))
	assert.False(t, srv.hostAllowed("127.0.0.1:9999"), "a different port is a different server")
	assert.False(t, srv.hostAllowed("evil.example:24508"))
	assert.False(t, srv.hostAllowed("evil.example"))
	assert.False(t, srv.hostAllowed(""))
}

func TestOriginAllowed(t *testing.T) {
	srv := &Server{port: "24508"}

	assert.True(t, srv.originAllowed("http://127.0.0.1:24508"))
	assert.True(t, srv.originAllowed("http://localhost:24508"))
	assert.False(t, srv.originAllowed("https://evil.example"))
	assert.False(t, srv.originAllowed("null"), "file:// and sandboxed pages send a null origin")
	assert.False(t, srv.originAllowed("not a url"))
}

func TestGuardRejectsNonJSONContentType(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks", strings.NewReader(`{"source":"x"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set(tokenHeader, srv.token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body errorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body.Error, "application/json")
}

func TestGuardRefusesFraming(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	for _, path := range []string{"/", "/api/v1/project"} {
		resp := apiGet(t, srv, ts.URL+path)
		t.Cleanup(func() { _ = resp.Body.Close() })

		assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"), path)
		assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'", path)
		assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), path)
	}
}

func TestIndexInjectsSessionToken(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), srv.token)
	assert.NotContains(t, string(body), tokenPlaceholder, "the placeholder must not survive into the page")
}

func TestStaticAssetsNeedNoToken(t *testing.T) {
	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	for _, path := range []string{"/app.js", "/app.css"} {
		resp, err := http.Get(ts.URL + path)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		assert.Equal(t, http.StatusOK, resp.StatusCode, path)
	}
}

func TestTraversalTaskIDIsRejected(t *testing.T) {
	srv, root := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(victim, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(victim, "task.yaml"), []byte("source: OUTSIDE_STORE_ROOT\n"), 0o600))

	escaped := url.PathEscape("../../victim")

	resp := apiGet(t, srv, ts.URL+"/api/v1/tasks/"+escaped)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotContains(t, string(body), "OUTSIDE_STORE_ROOT")

	resp = apiRequest(t, srv, http.MethodDelete, ts.URL+"/api/v1/tasks/"+escaped, "")
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	assert.DirExists(t, victim, "a traversal id must never delete outside the store")
	assert.FileExists(t, filepath.Join(victim, "task.yaml"))
}

func TestStartRejectsClientSuppliedCommand(t *testing.T) {
	srv, root := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp := apiPost(t, srv, ts.URL+"/api/v1/tasks", `{"source":"start test"}`)
	var created task.Record
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NoError(t, resp.Body.Close())

	resp = apiPost(t, srv, ts.URL+"/api/v1/tasks/"+created.ID+"/start",
		`{"command":"touch /tmp/gmc-pwned"}`)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, string(body), "command")
	_, statErr := os.Stat(filepath.Join(root, "..", "gmc-pwned"))
	assert.True(t, os.IsNotExist(statErr), "the supplied command must not run")
}
