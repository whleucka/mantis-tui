package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupCreateFake(t *testing.T) (*fakeMantis, string) {
	fm := newFakeMantis(t)
	fm.on("GET", "/config", 200, mantisFixture(t, "config_enums"))
	fm.on("GET", "/projects", 200, mantisFixture(t, "projects"))
	fm.on("GET", "/projects/4", 200, mantisFixture(t, "project"))
	fm.on("GET", "/projects/4/users", 200, mantisFixture(t, "project_users"))
	fm.on("POST", "/issues", 201, []byte(`{"issue":{"id":1234,"summary":"Crash"}}`))
	for _, id := range []string{"1", "2", "3"} {
		fm.on("DELETE", "/issues/"+id, 204, nil)
	}
	return fm, fakeHostConfig(t, fm, "")
}

func TestCreateSendsResolvedBody(t *testing.T) {
	fm, cfg := setupCreateFake(t)

	out, _, err := runCLI(t, "--config", cfg, "create",
		"--project", "project-4", "--category", "general", "--summary", "Crash",
		"--priority", "high", "--severity", "minor", "--reproducibility", "always",
		"--assign", "user1", "-d", "Steps: open, save.")
	if err != nil {
		t.Fatal(err)
	}
	r := fm.lastRequest("POST", "/issues")
	if r == nil {
		t.Fatal("no POST /issues")
	}
	var body map[string]any
	_ = json.Unmarshal(r.Body, &body)
	b, _ := json.Marshal(body)
	for _, want := range []string{
		`"summary":"Crash"`, `"description":"Steps: open, save."`,
		`"project":{"id":4,"name":"project-4"}`, `"category":{"id":1,"name":"General"}`,
		`"priority":{"id":40,"name":"high"}`, `"severity":{"id":50,"name":"minor"}`,
		`"reproducibility":{"id":10,"name":"always"}`, `"handler":{"id":1,"name":"user1"}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("body missing %s\n%s", want, b)
		}
	}
	if !strings.Contains(out, "#1234") || !strings.Contains(out, "view.php?id=1234") {
		t.Errorf("output = %q", out)
	}
}

func TestCreateWithEditorDescription(t *testing.T) {
	fm, cfg := setupCreateFake(t)
	script := filepath.Join(t.TempDir(), "ed.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nprintf 'Written in the editor\\n' >> \"$1\"\n"), 0o700)
	t.Setenv("VISUAL", script)

	if _, _, err := runCLI(t, "--config", cfg, "create", "--project", "4", "--category", "General", "--summary", "S", "--edit"); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(fm.lastRequest("POST", "/issues").Body, &body)
	if body["description"] != "Written in the editor" {
		t.Errorf("description = %v", body["description"])
	}
}

func TestCreateUsageErrors(t *testing.T) {
	fm, cfg := setupCreateFake(t)
	cases := [][]string{
		{"create", "--category", "General", "--summary", "S", "-d", "d"},                // no project
		{"create", "--project", "4", "--category", "General", "-d", "d"},                // no summary
		{"create", "--project", "4", "--summary", "S", "-d", "d"},                       // no category
		{"create", "--project", "4", "--category", "General", "--summary", "S"},         // no description, no TTY
		{"create", "--project", "4", "--category", "Nope", "--summary", "S", "-d", "d"}, // bad category
		{"create", "--project", "4", "--category", "General", "--summary", "S", "-d", "d", "--edit"},
	}
	for _, args := range cases {
		_, _, err := runCLI(t, append([]string{"--config", cfg}, args...)...)
		if ExitCode(err) != 2 {
			t.Errorf("args %v: exit %d err %v, want 2", args, ExitCode(err), err)
		}
	}
	if fm.lastRequest("POST", "/issues") != nil {
		t.Error("no issue should have been created")
	}
}

func TestDeleteRefusesWithoutYesOffTTY(t *testing.T) {
	fm, cfg := setupCreateFake(t)
	_, _, err := runCLI(t, "--config", cfg, "delete", "1", "2")
	if ExitCode(err) != 2 || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("exit %d err %v", ExitCode(err), err)
	}
	if fm.lastRequest("DELETE", "/issues/1") != nil {
		t.Error("nothing should be deleted")
	}
}

func TestDeletePromptsOnceWithCount(t *testing.T) {
	fm, cfg := setupCreateFake(t)
	d := testDeps()
	d.isTerminal = func() bool { return true }
	d.stdin = strings.NewReader("y\n")

	out, stderr, err := runCLIWith(t, d, "--config", cfg, "delete", "1", "2", "3")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(stderr, "[y/N]") != 1 || !strings.Contains(stderr, "Delete 3 issues") {
		t.Errorf("want one prompt naming 3 issues, stderr = %q", stderr)
	}
	for _, id := range []string{"1", "2", "3"} {
		if fm.lastRequest("DELETE", "/issues/"+id) == nil {
			t.Errorf("issue %s not deleted", id)
		}
		if !strings.Contains(out, "#"+id+" deleted") {
			t.Errorf("output missing #%s deleted: %q", id, out)
		}
	}
}

func TestDeleteWithYes(t *testing.T) {
	fm, cfg := setupCreateFake(t)
	if _, _, err := runCLI(t, "--config", cfg, "delete", "1", "--yes"); err != nil {
		t.Fatal(err)
	}
	if fm.lastRequest("DELETE", "/issues/1") == nil {
		t.Error("not deleted")
	}
}
