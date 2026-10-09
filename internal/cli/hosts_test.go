package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// runCLI executes the root command in-process and returns stdout, stderr and the error.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return runCLIWith(t, testDeps(), args...)
}

// testDeps never touch the real terminal: stdin is empty and not a TTY, and
// opening a browser fails the test.
func testDeps() deps {
	return deps{
		openURL:    func(u string) error { return fmt.Errorf("test tried to open a browser: %s", u) },
		stdin:      strings.NewReader(""),
		isTerminal: func() bool { return false },
		clipboard:  func(context.Context) *mantis.FileUpload { return nil },
	}
}

// runCLIWith is runCLI with injected dependencies.
func runCLIWith(t *testing.T, d deps, args ...string) (string, string, error) {
	t.Helper()
	def := testDeps()
	if d.openURL == nil {
		d.openURL = def.openURL
	}
	if d.stdin == nil {
		d.stdin = def.stdin
	}
	if d.isTerminal == nil {
		d.isTerminal = def.isTerminal
	}
	if d.clipboard == nil {
		d.clipboard = def.clipboard
	}
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd(d)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func writeTestConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const hostsConfig = `
[[hosts]]
name = "alpha"
url  = "https://alpha.example.com"
env  = "MANTIS_TEST_ALPHA"
default = true

[[hosts]]
name = "beta"
url  = "https://beta.example.com"
env  = "MANTIS_TEST_BETA_UNSET"
`

func TestHostsListsActiveAndDropped(t *testing.T) {
	t.Setenv("MANTIS_TEST_ALPHA", "secret-alpha-token")
	t.Setenv("MANTIS_TEST_BETA_UNSET", "")
	path := writeTestConfig(t, hostsConfig)

	out, _, err := runCLI(t, "--config", path, "hosts")
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("want header + 2 rows, got:\n%s", out)
	}
	if !strings.Contains(lines[1], "alpha") || !strings.Contains(lines[1], "active (default)") {
		t.Errorf("alpha row = %q", lines[1])
	}
	if !strings.Contains(lines[2], "beta") || !strings.Contains(lines[2], "dropped") || !strings.Contains(lines[2], "MANTIS_TEST_BETA_UNSET") {
		t.Errorf("beta row = %q", lines[2])
	}
	if strings.Contains(out, "secret-alpha-token") {
		t.Error("hosts output leaks the token")
	}
}

func TestHostsJSON(t *testing.T) {
	t.Setenv("MANTIS_TEST_ALPHA", "secret-alpha-token")
	t.Setenv("MANTIS_TEST_BETA_UNSET", "")
	path := writeTestConfig(t, hostsConfig)

	out, _, err := runCLI(t, "--config", path, "--json", "hosts")
	if err != nil {
		t.Fatal(err)
	}
	var got []struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Default bool   `json:"default"`
		Status  string `json:"status"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 || got[0].Status != "active" || !got[0].Default || got[1].Status != "dropped" || got[1].Reason == "" {
		t.Errorf("got %+v", got)
	}
	if strings.Contains(out, "secret-alpha-token") {
		t.Error("hosts JSON leaks the token")
	}
}

func TestHostsMissingExplicitConfig(t *testing.T) {
	_, _, err := runCLI(t, "--config", filepath.Join(t.TempDir(), "nope.toml"), "hosts")
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d (err %v), want 2 for a missing --config file", ExitCode(err), err)
	}
}

func TestHostsPrintsConfigWarnings(t *testing.T) {
	path := writeTestConfig(t, "[[hosts]]\nname = \"x\"\nurl = \"https://x\"\ntoken = \"inline-secret\"\n")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runCLI(t, "--config", path, "hosts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "chmod 600") {
		t.Errorf("expected permission warning on stderr, got %q", stderr)
	}
	if strings.Contains(stderr, "inline-secret") {
		t.Error("warning leaks the token")
	}
}
