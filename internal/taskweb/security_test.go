package taskweb

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardRequests(t *testing.T) {
	srv, _ := newTestServer(t)
	baseURL := serveTest(t, srv)

	for _, test := range []struct {
		name, method, path, body string
		token, origin, host      string
		contentType, errorText   string
		status                   int
	}{
		{
			name: "anonymous read", method: http.MethodGet, path: "/api/v1/project",
			status: http.StatusUnauthorized,
		},
		{
			name: "wrong token", method: http.MethodGet, path: "/api/v1/project",
			token: "not-the-token", status: http.StatusUnauthorized,
		},
		{
			name: "cross origin", method: http.MethodPost, path: "/api/v1/tasks",
			body: `{"source":"csrf"}`, token: srv.token, origin: "https://evil.example",
			contentType: "text/plain", status: http.StatusForbidden,
		},
		{
			name: "same origin", method: http.MethodGet, path: "/api/v1/project",
			token: srv.token, origin: baseURL, status: http.StatusOK,
		},
		{
			name: "foreign host", method: http.MethodGet, path: "/",
			host: "evil.example", status: http.StatusForbidden,
		},
		{
			name: "non-JSON content type", method: http.MethodPost, path: "/api/v1/tasks",
			body: `{"source":"x"}`, token: srv.token, contentType: "text/plain",
			status: http.StatusBadRequest, errorText: "application/json",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(test.method, baseURL+test.path, strings.NewReader(test.body))
			require.NoError(t, err)
			for key, value := range map[string]string{
				tokenHeader: test.token, "Origin": test.origin, "Content-Type": test.contentType,
			} {
				if value != "" {
					req.Header.Set(key, value)
				}
			}
			if test.host != "" {
				req.Host = test.host
			}
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
			assert.Equal(t, test.status, resp.StatusCode)
			if test.errorText != "" {
				var body errorResponse
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body.Error, test.errorText)
			}
		})
	}
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

func TestGuardRefusesFraming(t *testing.T) {
	srv, _ := newTestServer(t)
	baseURL := serveTest(t, srv)

	for _, path := range []string{"/", "/api/v1/project"} {
		resp := apiGet(t, srv, baseURL+path)
		t.Cleanup(func() { _ = resp.Body.Close() })

		assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"), path)
		assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'", path)
		assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), path)
	}
}

func TestIndexInjectsSessionToken(t *testing.T) {
	srv, _ := newTestServer(t)
	baseURL := serveTest(t, srv)

	resp, err := http.Get(baseURL + "/")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "gmc task webui")
	assert.Contains(t, string(body), srv.token)
	assert.NotContains(t, string(body), tokenPlaceholder, "the placeholder must not survive into the page")
}

func TestStaticAssetsNeedNoToken(t *testing.T) {
	srv, _ := newTestServer(t)
	baseURL := serveTest(t, srv)

	for _, path := range []string{"/app.js", "/app.css"} {
		resp, err := http.Get(baseURL + path)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		assert.Equal(t, http.StatusOK, resp.StatusCode, path)
	}
}

func TestTraversalTaskIDIsRejected(t *testing.T) {
	srv, root := newTestServer(t)
	baseURL := serveTest(t, srv)

	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(victim, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(victim, "task.yaml"), []byte("source: OUTSIDE_STORE_ROOT\n"), 0o600))

	escaped := url.PathEscape("../../victim")

	resp := apiGet(t, srv, baseURL+"/api/v1/tasks/"+escaped)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.NotContains(t, string(body), "OUTSIDE_STORE_ROOT")

	resp = apiRequest(t, srv, http.MethodDelete, baseURL+"/api/v1/tasks/"+escaped, "")
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	assert.DirExists(t, victim, "a traversal id must never delete outside the store")
	assert.FileExists(t, filepath.Join(victim, "task.yaml"))
}

func TestStartRejectsClientSuppliedCommand(t *testing.T) {
	srv, root := newTestServer(t)
	baseURL := serveTest(t, srv)

	created := createAPITask(t, srv, baseURL, "start test")
	resp := apiPost(t, srv, baseURL+"/api/v1/tasks/"+created.ID+"/start",
		`{"command":"touch /tmp/gmc-pwned"}`)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, string(body), "command")
	_, statErr := os.Stat(filepath.Join(root, "..", "gmc-pwned"))
	assert.True(t, os.IsNotExist(statErr), "the supplied command must not run")
}
