package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

// rewriteEditor makes the "editor" replace the temp file with edit(old).
func rewriteEditor(h *harness, edit func(old string) string) {
	h.m.execEditor = func(s *editor.Session, done func(error) tea.Msg) tea.Cmd {
		b, err := os.ReadFile(s.Path)
		if err == nil {
			err = os.WriteFile(s.Path, []byte(edit(string(b))), 0o600)
		}
		return func() tea.Msg { return done(err) }
	}
}

func fieldsHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(3)(name, f)
		is := f.Issues[3]
		is.Description = "It crashes."
		is.StepsToReproduce = "1. open it"
		f.Issues[3] = is
	}, nil)
}

func tempFiles(t *testing.T) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "mantis-*"))
	return files
}

func TestEditDescriptionStartsFromServerText(t *testing.T) {
	h := fieldsHarness(t)
	f := h.fakes["alpha"]
	f.BumpOnWrite = true // so marking it seen is observable
	is := f.Issues[3]
	is.Description = "# Heading\nServer text." // newer than the list row
	f.Issues[3] = is

	var started string
	rewriteEditor(h, func(old string) string {
		started = old
		return strings.Replace(old, "Server text.", "Server text, edited.", 1)
	})
	h.keys("e")
	if v := h.view(); !strings.Contains(v, "Edit which field of #3?") || !strings.Contains(v, "1. open it") || !strings.Contains(v, "(empty)") {
		t.Fatalf("field picker missing or without first lines:\n%s", v)
	}
	h.keys("down", "enter")
	if !strings.HasPrefix(started, "# Heading\nServer text.\n") || !strings.Contains(started, "# Description of issue #3 on alpha.") {
		t.Fatalf("editor started from %q", started)
	}
	if len(f.Patches) != 1 {
		t.Fatalf("patches = %+v", f.Patches)
	}
	p := f.Patches[0]
	if p.ID != 3 || p.Patch.Description == nil || *p.Patch.Description != "# Heading\nServer text, edited." ||
		p.Patch.StepsToReproduce != nil || p.Patch.AdditionalInformation != nil || p.Patch.Summary != nil {
		t.Fatalf("patch = %+v", p)
	}
	if !strings.Contains(lastLine(h.view()), "#3 description updated") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	if files := tempFiles(t); len(files) != 0 {
		t.Errorf("temp file should be removed after sending: %v", files)
	}
	if h.m.seen.Unread("alpha", 3, f.Issues[3].UpdatedAt) {
		t.Error("your own edit must not leave the issue unread")
	}
}

func TestEditStepsFromIssueView(t *testing.T) {
	h := fieldsHarness(t)
	rewriteEditor(h, func(old string) string { return strings.Replace(old, "1. open it", "1. open it\n2. click save", 1) })
	h.keys("enter", "e", "down", "down", "enter")
	f := h.fakes["alpha"]
	if len(f.Patches) != 1 || f.Patches[0].Patch.StepsToReproduce == nil || *f.Patches[0].Patch.StepsToReproduce != "1. open it\n2. click save" || f.Patches[0].Patch.Description != nil {
		t.Fatalf("patches = %+v", f.Patches)
	}
	if !strings.Contains(h.view(), "2. click save") {
		t.Errorf("issue view should reload with the new steps:\n%s", h.view())
	}
}

func TestEditFieldUnchangedSendsNothing(t *testing.T) {
	h := fieldsHarness(t)
	rewriteEditor(h, func(old string) string { return old })
	h.keys("e", "down", "enter")
	if p := h.fakes["alpha"].Patches; len(p) != 0 {
		t.Fatalf("patches = %+v", p)
	}
	if !strings.Contains(lastLine(h.view()), "#3 description unchanged") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	if files := tempFiles(t); len(files) != 0 {
		t.Errorf("temp file should be removed: %v", files)
	}
}

func TestEditFieldChangedOnServerKeepsText(t *testing.T) {
	h := fieldsHarness(t)
	f := h.fakes["alpha"]
	rewriteEditor(h, func(old string) string {
		is := f.Issues[3]
		is.Description = "Someone else's text."
		f.Issues[3] = is
		return strings.Replace(old, "It crashes.", "Mine.", 1)
	})
	h.keys("e", "down", "enter")
	if len(f.Patches) != 0 {
		t.Fatalf("nothing should be sent: %+v", f.Patches)
	}
	status := lastLine(h.view())
	if !strings.Contains(status, "changed on the server") {
		t.Errorf("status = %q", status)
	}
	files := tempFiles(t)
	if len(files) != 1 {
		t.Fatalf("temp file should be kept: %v", files)
	}
	if b, _ := os.ReadFile(files[0]); !strings.Contains(string(b), "Mine.") {
		t.Errorf("kept file = %q", b)
	}
}

func TestEditFieldFailedPatchKeepsText(t *testing.T) {
	h := fieldsHarness(t)
	h.fakes["alpha"].Errs = map[string]error{"UpdateIssue": errBoomf("server said no")}
	rewriteEditor(h, func(old string) string { return strings.Replace(old, "It crashes.", "Mine.", 1) })
	h.keys("e", "down", "enter")
	if status := lastLine(h.view()); !strings.Contains(status, "not saved") {
		t.Errorf("status = %q", status)
	}
	if files := tempFiles(t); len(files) != 1 {
		t.Errorf("temp file should be kept: %v", files)
	}
}
