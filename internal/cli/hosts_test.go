package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI executes the root command in-process and returns stdout, stderr and the error.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
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
	if err == nil {
		t.Fatal("expected error for missing --config file")
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
