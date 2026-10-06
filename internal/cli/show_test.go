package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestShowPrintsIssue(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues/33", 200, mantisFixture(t, "issue"))
	cfg := fakeHostConfig(t, fm, "")

	out, _, err := runCLI(t, "--config", cfg, "show", "33")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"#33", "Sample summary", "closed", "Priority", "Severity", "Reporter", "Sample description"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Sample note text") || strings.Contains(out, "History") {
		t.Errorf("notes/history should be hidden without flags:\n%s", out)
	}
}

func TestShowNotesAndHistory(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues/33", 200, mantisFixture(t, "issue"))
	cfg := fakeHostConfig(t, fm, "")

	out, _, err := runCLI(t, "--config", cfg, "show", "33", "--notes", "--history")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Notes", "Sample note text", "History", "New Issue"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestShowJSONIsServerJSON(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues/33", 200, mantisFixture(t, "issue"))
	cfg := fakeHostConfig(t, fm, "")

	out, _, err := runCLI(t, "--config", cfg, "--json", "show", "33")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Issues []struct {
			ID int `json:"id"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || len(v.Issues) != 1 || v.Issues[0].ID != 33 {
		t.Fatalf("got %s (err %v)", out, err)
	}
}

func TestShowExitCodes(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		status int
		body   string
		want   int
	}{
		{"not found", []string{"show", "9"}, 404, `{"message":"Issue #9 not found"}`, 3},
		{"server error", []string{"show", "9"}, 500, `{"message":"boom"}`, 1},
		{"forbidden", []string{"show", "9"}, 403, `{"message":"Access denied"}`, 4},
		{"non-numeric id", []string{"show", "abc"}, 200, `{}`, 2},
		{"missing id", []string{"show"}, 200, `{}`, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := newFakeMantis(t)
			fm.on("GET", "/issues/9", tt.status, []byte(tt.body))
			cfg := fakeHostConfig(t, fm, "")

			_, _, err := runCLI(t, append([]string{"--config", cfg}, tt.args...)...)
			if got := ExitCode(err); got != tt.want {
				t.Errorf("exit code = %d (err %v), want %d", got, err, tt.want)
			}
		})
	}
}

func TestExitCodeAuthFailure(t *testing.T) {
	fm := newFakeMantis(t)
	cfg := fakeHostConfig(t, fm, "")
	t.Setenv("MANTIS_TEST_FAKE", "wrong-token")

	_, _, err := runCLI(t, "--config", cfg, "show", "1")
	if ExitCode(err) != 4 {
		t.Fatalf("exit code = %d (err %v), want 4", ExitCode(err), err)
	}
}

func TestExitCodeUsage(t *testing.T) {
	isolateEnv(t)
	cases := [][]string{
		{"--no-such-flag"},
		{"--config", "/nonexistent/config.toml", "list"},
		{"no-such-command"},
	}
	for _, args := range cases {
		_, _, err := runCLI(t, args...)
		if ExitCode(err) != 2 {
			t.Errorf("args %v: exit code = %d (err %v), want 2", args, ExitCode(err), err)
		}
	}
	if ExitCode(nil) != 0 {
		t.Error("nil error should be exit 0")
	}
}

func TestHostSelectionErrorsAreUsageErrors(t *testing.T) {
	isolateEnv(t)
	t.Setenv("MANTIS_TEST_A", "x")
	t.Setenv("MANTIS_TEST_B", "y")
	cfg := writeTestConfig(t, `
[[hosts]]
name = "a"
url = "https://a.example"
env = "MANTIS_TEST_A"
[[hosts]]
name = "b"
url = "https://b.example"
env = "MANTIS_TEST_B"
`)
	_, _, err := runCLI(t, "--config", cfg, "show", "1")
	if ExitCode(err) != 2 || !strings.Contains(err.Error(), "--host") {
		t.Errorf("ambiguous host: exit %d err %v, want 2 mentioning --host", ExitCode(err), err)
	}
	_, _, err = runCLI(t, "--config", cfg, "--host", "zzz", "show", "1")
	if ExitCode(err) != 2 {
		t.Errorf("unknown host: exit %d err %v, want 2", ExitCode(err), err)
	}
}
