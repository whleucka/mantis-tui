package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const cliTestToken = "cli-sentinel-token-77"

// isolateEnv keeps the developer's real env and state file out of CLI tests.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MANTIS_TUI_HOST", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func mantisFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "mantis", "testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeMantis routes "METHOD /path" to canned responses and records requests.
type fakeMantis struct {
	*httptest.Server
	mu       sync.Mutex
	routes   map[string]fakeResponse
	requests []recordedRequest
}

type fakeResponse struct {
	status int
	body   []byte
}

type recordedRequest struct {
	Method, Path, Query string
	Body                []byte
}

func newFakeMantis(t *testing.T) *fakeMantis {
	t.Helper()
	fm := &fakeMantis{routes: map[string]fakeResponse{}}
	fm.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		path := strings.TrimPrefix(r.URL.Path, "/api/rest")
		fm.mu.Lock()
		fm.requests = append(fm.requests, recordedRequest{r.Method, path, r.URL.RawQuery, body})
		resp, ok := fm.routes[r.Method+" "+path]
		fm.mu.Unlock()
		if r.Header.Get("Authorization") != cliTestToken {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"message":"API token not found"}`))
			return
		}
		if !ok {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"no route ` + r.Method + ` ` + path + `"}`))
			return
		}
		w.WriteHeader(resp.status)
		_, _ = w.Write(resp.body)
	}))
	t.Cleanup(fm.Close)
	return fm
}

func (fm *fakeMantis) on(method, path string, status int, body []byte) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.routes[method+" "+path] = fakeResponse{status, body}
}

func (fm *fakeMantis) lastRequest(method, path string) *recordedRequest {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	for i := len(fm.requests) - 1; i >= 0; i-- {
		if fm.requests[i].Method == method && fm.requests[i].Path == path {
			return &fm.requests[i]
		}
	}
	return nil
}

// fakeHostConfig writes a config with one host "fake" pointing at fm.
func fakeHostConfig(t *testing.T, fm *fakeMantis, extra string) string {
	t.Helper()
	isolateEnv(t)
	t.Setenv("MANTIS_TEST_FAKE", cliTestToken)
	return writeTestConfig(t, extra+`
[[hosts]]
name = "fake"
url  = "`+fm.URL+`"
env  = "MANTIS_TEST_FAKE"
`)
}
