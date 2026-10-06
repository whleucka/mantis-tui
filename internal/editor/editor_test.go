package editor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEditor writes a shell script that runs body with $1 = the file, and
// points $VISUAL at it.
func fakeEditor(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-editor.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", path)
	t.Setenv("EDITOR", "")
}

func newReq() Request {
	return Request{Host: "work/main", IssueID: 33, Kind: "note", Hints: []string{"Write a note for #33.", "Lines like this one are removed."}}
}

func TestCommandPrecedence(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		env  map[string]string
		want []string
	}{
		{map[string]string{"VISUAL": "nvim", "EDITOR": "nano"}, []string{"nvim"}},
		{map[string]string{"EDITOR": "code --wait"}, []string{"code", "--wait"}},
		{map[string]string{}, []string{"vi"}},
	}
	for _, tt := range tests {
		got := Command(env(tt.env))
		if strings.Join(got, " ") != strings.Join(tt.want, " ") {
			t.Errorf("env %v: got %v, want %v", tt.env, got, tt.want)
		}
	}
}

func TestRunReturnsTextWithoutHints(t *testing.T) {
	fakeEditor(t, `printf '# My heading\nFixed in abc123.\n' >> "$1"`)

	s, err := Prepare(newReq())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	text, err := s.Text()
	if err != nil {
		t.Fatal(err)
	}
	if text != "# My heading\nFixed in abc123." {
		t.Errorf("text = %q (user '#' lines kept, hint lines removed)", text)
	}
	s.Cleanup()
	if _, err := os.Stat(s.Path); !errors.Is(err, os.ErrNotExist) {
		t.Error("Cleanup should remove the temp file")
	}
}

func TestEmptyOrUnchangedAborts(t *testing.T) {
	for name, body := range map[string]string{
		"unchanged":       `true`,
		"emptied":         `: > "$1"`,
		"only whitespace": `printf '\n   \n' >> "$1"`,
		"same as initial": `true`,
	} {
		t.Run(name, func(t *testing.T) {
			fakeEditor(t, body)
			req := newReq()
			if name == "same as initial" {
				req.Initial = "existing description"
			}
			s, err := Prepare(req)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Cleanup()
			if err := s.Run(); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Text(); !errors.Is(err, ErrEmpty) {
				t.Errorf("err = %v, want ErrEmpty", err)
			}
		})
	}
}

func TestEditedInitialTextIsReturned(t *testing.T) {
	fakeEditor(t, `printf 'more\n' >> "$1"`)
	req := newReq()
	req.Initial = "existing"
	s, err := Prepare(req)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Cleanup()
	_ = s.Run()
	text, err := s.Text()
	if err != nil || text != "existing\nmore" {
		t.Errorf("text = %q, err %v", text, err)
	}
}

func TestTempFileIsPrivateAndNamed(t *testing.T) {
	out := filepath.Join(t.TempDir(), "mode")
	fakeEditor(t, `stat -c %a "$1" > `+out)

	s, err := Prepare(newReq())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Cleanup()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	mode, _ := os.ReadFile(out)
	if strings.TrimSpace(string(mode)) != "600" {
		t.Errorf("temp file mode = %q, want 600", mode)
	}
	base := filepath.Base(s.Path)
	if !strings.HasPrefix(base, "mantis-work-main-33-note-") || !strings.HasSuffix(base, ".md") {
		t.Errorf("temp file name = %q", base)
	}
}

func TestEditorFailureIsReported(t *testing.T) {
	fakeEditor(t, `exit 3`)
	s, err := Prepare(newReq())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Cleanup()
	if err := s.Run(); err == nil {
		t.Error("expected error when the editor exits non-zero")
	}
}

func TestCmdIsNotAttachedToTerminal(t *testing.T) {
	fakeEditor(t, `true`)
	s, err := Prepare(newReq())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Cleanup()
	c := s.Cmd()
	if c.Stdin != nil || c.Stdout != nil {
		t.Error("Cmd() is for tea.ExecProcess, which wires stdio itself")
	}
	if c.Args[len(c.Args)-1] != s.Path {
		t.Errorf("last arg = %q, want the temp file", c.Args[len(c.Args)-1])
	}
}
