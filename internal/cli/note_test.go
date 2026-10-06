package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupNoteFake(t *testing.T) (*fakeMantis, string) {
	fm := newFakeMantis(t)
	fm.on("POST", "/issues/33/notes", 201, []byte(`{"note":{"id":61,"text":"x"}}`))
	fm.on("DELETE", "/issues/33/notes/61", 200, []byte(`{"issue":{"id":33}}`))
	return fm, fakeHostConfig(t, fm, "")
}

func noteBody(t *testing.T, fm *fakeMantis) map[string]any {
	t.Helper()
	r := fm.lastRequest("POST", "/issues/33/notes")
	if r == nil {
		t.Fatal("no note POST")
	}
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNoteWithMessage(t *testing.T) {
	fm, cfg := setupNoteFake(t)
	out, _, err := runCLI(t, "--config", cfg, "note", "33", "-m", "fixed in abc123", "--time", "0:30", "--private")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(noteBody(t, fm))
	want := `{"text":"fixed in abc123","time_tracking":{"duration":"0:30"},"view_state":{"name":"private"}}`
	if string(b) != want {
		t.Errorf("body = %s\nwant   %s", b, want)
	}
	if !strings.Contains(out, "#33") || !strings.Contains(out, "61") {
		t.Errorf("output = %q", out)
	}
}

func TestNoteFromStdin(t *testing.T) {
	fm, cfg := setupNoteFake(t)
	d := testDeps()
	d.stdin = strings.NewReader("from a pipe\n")
	if _, _, err := runCLIWith(t, d, "--config", cfg, "note", "33", "-"); err != nil {
		t.Fatal(err)
	}
	if got := noteBody(t, fm)["text"]; got != "from a pipe" {
		t.Errorf("text = %q", got)
	}
}

func TestNoteWithEditor(t *testing.T) {
	fm, cfg := setupNoteFake(t)
	script := filepath.Join(t.TempDir(), "ed.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nprintf 'from the editor\\n' >> \"$1\"\n"), 0o700)
	t.Setenv("VISUAL", script)

	if _, _, err := runCLI(t, "--config", cfg, "note", "33", "--edit"); err != nil {
		t.Fatal(err)
	}
	if got := noteBody(t, fm)["text"]; got != "from the editor" {
		t.Errorf("text = %q", got)
	}
}

func TestNoteEmptyEditorSendsNothing(t *testing.T) {
	fm, cfg := setupNoteFake(t)
	script := filepath.Join(t.TempDir(), "ed.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\ntrue\n"), 0o700)
	t.Setenv("VISUAL", script)

	_, stderr, err := runCLI(t, "--config", cfg, "note", "33", "--edit")
	if err == nil {
		t.Fatal("expected an error for an empty note")
	}
	if fm.lastRequest("POST", "/issues/33/notes") != nil {
		t.Error("empty note must not be sent")
	}
	_ = stderr
}

func TestNoteSubmitFailureKeepsEditorFile(t *testing.T) {
	fm, cfg := setupNoteFake(t)
	fm.on("POST", "/issues/33/notes", 500, []byte(`{"message":"db down"}`))
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	script := filepath.Join(t.TempDir(), "ed.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nprintf 'precious words\\n' >> \"$1\"\n"), 0o700)
	t.Setenv("VISUAL", script)

	_, stderr, err := runCLI(t, "--config", cfg, "note", "33", "--edit")
	if err == nil {
		t.Fatal("expected error")
	}
	files, _ := filepath.Glob(filepath.Join(tmp, "mantis-*"))
	if len(files) != 1 {
		t.Fatalf("editor file should be kept on failure, found %v", files)
	}
	if !strings.Contains(stderr, files[0]) {
		t.Errorf("stderr should name the saved file %s, got %q", files[0], stderr)
	}
}

func TestNoteUsageErrors(t *testing.T) {
	_, cfg := setupNoteFake(t)
	cases := [][]string{
		{"note", "33", "-m", "x", "--time", "30m"},
		{"note", "33", "-m", "x", "--time", "1:75"},
		{"note", "33", "-m", "x", "--edit"},
		{"note", "33", "-m", ""},
		{"note"},
		{"note", "33"}, // no text source and stdin is not a terminal
	}
	for _, args := range cases {
		_, _, err := runCLI(t, append([]string{"--config", cfg}, args...)...)
		if ExitCode(err) != 2 {
			t.Errorf("args %v: exit %d err %v, want 2", args, ExitCode(err), err)
		}
	}
}

func TestNoteDeleteRequiresYesWithoutTTY(t *testing.T) {
	fm, cfg := setupNoteFake(t)
	_, _, err := runCLI(t, "--config", cfg, "note", "delete", "33", "61")
	if ExitCode(err) != 2 || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("exit %d err %v, want 2 mentioning --yes", ExitCode(err), err)
	}
	if fm.lastRequest("DELETE", "/issues/33/notes/61") != nil {
		t.Error("nothing should be deleted")
	}

	if _, _, err := runCLI(t, "--config", cfg, "note", "delete", "33", "61", "--yes"); err != nil {
		t.Fatal(err)
	}
	if fm.lastRequest("DELETE", "/issues/33/notes/61") == nil {
		t.Error("--yes should delete")
	}
}

func TestNoteDeleteConfirmsOnTTY(t *testing.T) {
	for answer, wantDelete := range map[string]bool{"y\n": true, "n\n": false, "\n": false} {
		fm, cfg := setupNoteFake(t)
		d := testDeps()
		d.isTerminal = func() bool { return true }
		d.stdin = strings.NewReader(answer)

		_, stderr, err := runCLIWith(t, d, "--config", cfg, "note", "delete", "33", "61")
		deleted := fm.lastRequest("DELETE", "/issues/33/notes/61") != nil
		if deleted != wantDelete {
			t.Errorf("answer %q: deleted = %v (err %v)", answer, deleted, err)
		}
		if !strings.Contains(stderr, "Delete note 61") {
			t.Errorf("prompt missing, stderr = %q", stderr)
		}
		if !wantDelete && err == nil {
			t.Errorf("answer %q: declining should return an error", answer)
		}
	}
}
