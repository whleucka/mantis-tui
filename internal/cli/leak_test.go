package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allCommands exercises every subcommand, including write and error paths.
var allCommands = [][]string{
	{"hosts"},
	{"list"},
	{"list", "--project", "project-4"},
	{"show", "33", "--notes", "--history"},
	{"update", "33", "--status", "resolved"},
	{"update", "33", "--category", "General"},
	{"assign", "33", "user1"},
	{"monitor", "33"},
	{"unmonitor", "33"},
	{"note", "33", "-m", "hello", "--time", "0:30"},
	{"note", "33", "--edit"},
	{"note", "delete", "33", "61", "--yes"},
	{"create", "--project", "4", "--category", "General", "--summary", "s", "-d", "d"},
	{"delete", "33", "--yes"},
	{"open", "33"},
}

func checkNoLeak(t *testing.T, args []string, stdout, stderr string, err error) {
	t.Helper()
	for name, s := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if strings.Contains(s, cliTestToken) {
			t.Errorf("%v: token leaked on %s: %q", args, name, s)
		}
	}
	if err != nil && strings.Contains(err.Error(), cliTestToken) {
		t.Errorf("%v: token leaked in error: %v", args, err)
	}
}

func editorScript(t *testing.T) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "ed.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nprintf 'text\\n' >> \"$1\"\n"), 0o700)
	t.Setenv("VISUAL", script)
}

// A hostile server echoes the Authorization header into every error body.
func TestNoTokenLeakWhenServerEchoesIt(t *testing.T) {
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"message":"bad request with token ` + r.Header.Get("Authorization") + `"}`))
	}))
	defer echo.Close()

	isolateEnv(t)
	t.Setenv("MANTIS_TEST_FAKE", cliTestToken)
	editorScript(t)
	cfg := writeTestConfig(t, "[[hosts]]\nname = \"fake\"\nurl = \""+echo.URL+"\"\nenv = \"MANTIS_TEST_FAKE\"\n")

	for _, args := range allCommands {
		d := testDeps()
		d.openURL = func(string) error { return nil }
		stdout, stderr, err := runCLIWith(t, d, append([]string{"--config", cfg}, args...)...)
		checkNoLeak(t, args, stdout, stderr, err)
		stdout, stderr, err = runCLIWith(t, d, append([]string{"--config", cfg, "--json"}, args...)...)
		checkNoLeak(t, args, stdout, stderr, err)
	}
}

// Normal successful runs must not print the token either.
func TestNoTokenLeakOnSuccess(t *testing.T) {
	fm, cfg := setupWriteFake(t)
	fm.on("GET", "/projects", 200, mantisFixture(t, "projects"))
	fm.on("GET", "/issues", 200, mantisFixture(t, "issues_list"))
	fm.on("POST", "/issues", 201, []byte(`{"issue":{"id":1234}}`))
	fm.on("POST", "/issues/33/notes", 201, []byte(`{"note":{"id":61}}`))
	fm.on("DELETE", "/issues/33/notes/61", 200, []byte(`{}`))
	fm.on("DELETE", "/issues/33", 204, nil)
	fm.on("GET", "/config", 200, []byte(`{"configs":[{"option":"time_tracking_enabled","value":1}]}`))
	editorScript(t)

	for _, args := range allCommands {
		d := testDeps()
		d.openURL = func(string) error { return nil }
		stdout, stderr, err := runCLIWith(t, d, append([]string{"--config", cfg, "--json"}, args...)...)
		checkNoLeak(t, args, stdout, stderr, err)
	}
}
