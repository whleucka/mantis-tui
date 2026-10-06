package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

// stubEditor makes the "editor" append text to the temp file synchronously.
func stubEditor(h *harness, text string) {
	h.m.execEditor = func(s *editor.Session, done func(error) tea.Msg) tea.Cmd {
		f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_WRONLY, 0)
		if err == nil {
			_, _ = f.WriteString(text)
			_ = f.Close()
		}
		return func() tea.Msg { return done(err) }
	}
}

func notesHarness(t *testing.T, timeTracking bool) *harness {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(3)(name, f)
		is := f.Issues[3]
		is.Notes = []mantis.Note{
			{ID: 60, Text: "first note\nmore", Reporter: mantis.User{Name: "jane"}},
			{ID: 61, Text: "second note", Reporter: mantis.User{Name: "bob"}},
		}
		f.Issues[3] = is
		if timeTracking {
			f.ConfigValues["time_tracking_enabled"] = json.RawMessage("1")
		}
	}, nil)
}

func TestAddNoteFromList(t *testing.T) {
	h := notesHarness(t, false)
	stubEditor(h, "Fixed in abc123.\n")
	h.keys("N")
	if !strings.Contains(h.view(), "Add note to #3") {
		t.Fatalf("note form missing:\n%s", h.view())
	}
	if strings.Contains(h.view(), "Time spent") {
		t.Error("time field must be hidden when time tracking is disabled")
	}
	h.keys("enter")
	added := h.fakes["alpha"].NotesAdded
	if len(added) != 1 || added[0].IssueID != 3 || added[0].Note.Text != "Fixed in abc123." || added[0].Note.Private || added[0].Note.TimeTracking != "" {
		t.Fatalf("notes added = %+v", added)
	}
	if !strings.Contains(lastLine(h.view()), "note") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	if files, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "mantis-*")); len(files) != 0 {
		t.Errorf("temp file should be removed after sending: %v", files)
	}
}

func TestAddPrivateTimedNote(t *testing.T) {
	h := notesHarness(t, true)
	stubEditor(h, "Spent a while.\n")
	h.keys("N")
	if !strings.Contains(h.view(), "Time spent") {
		t.Fatal("time field should show when time tracking is enabled")
	}
	h.keys("space", "tab")
	for _, r := range "0:45" {
		h.send(key(string(r)))
	}
	h.keys("enter")
	added := h.fakes["alpha"].NotesAdded
	if len(added) != 1 || !added[0].Note.Private || added[0].Note.TimeTracking != "0:45" {
		t.Fatalf("notes added = %+v", added)
	}
}

func TestNoteFormRejectsBadTime(t *testing.T) {
	h := notesHarness(t, true)
	stubEditor(h, "x")
	h.keys("N", "tab", "9", "9", "enter")
	if h.m.modal == nil || !strings.Contains(h.view(), "H:MM") {
		t.Errorf("bad time should keep the form open with an error:\n%s", h.view())
	}
	if len(h.fakes["alpha"].NotesAdded) != 0 {
		t.Error("nothing should be sent")
	}
}

func TestEmptyEditorDiscardsNote(t *testing.T) {
	h := notesHarness(t, false)
	stubEditor(h, "")
	h.keys("N", "enter")
	if len(h.fakes["alpha"].NotesAdded) != 0 {
		t.Error("empty note must not be sent")
	}
	if !strings.Contains(lastLine(h.view()), "note discarded") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}

func TestFailedNoteKeepsTextFile(t *testing.T) {
	h := notesHarness(t, false)
	h.fakes["alpha"].Errs = map[string]error{"AddNote": errors.New("db down")}
	stubEditor(h, "precious words\n")
	h.keys("N", "enter")
	files, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "mantis-*"))
	if len(files) != 1 {
		t.Fatalf("text file should be kept, found %v", files)
	}
	if !strings.Contains(h.m.status.msg, files[0]) || !strings.Contains(h.m.status.msg, "db down") {
		t.Errorf("status should name the saved file and the error: %q", h.m.status.msg)
	}
}

func TestEditingBlocksAutoRefresh(t *testing.T) {
	h := notesHarness(t, false)
	var finish func(error) tea.Msg
	h.m.execEditor = func(_ *editor.Session, done func(error) tea.Msg) tea.Cmd {
		finish = done
		return nil // the editor is "still open"
	}
	h.keys("N", "enter")
	if !h.m.editing {
		t.Fatal("model should know an editor is open")
	}
	if !h.m.busy() {
		t.Error("an open editor should count as busy for auto-refresh")
	}
	h.send(finish(nil))
	if h.m.editing {
		t.Error("editing should end when the editor exits")
	}
}

func TestAddNoteFromIssueViewReloadsIt(t *testing.T) {
	h := notesHarness(t, false)
	stubEditor(h, "From the issue view.\n")
	h.keys("enter") // #3
	gets := h.fakes["alpha"].Calls("GetIssue")
	h.keys("N", "enter")
	if len(h.fakes["alpha"].NotesAdded) != 1 {
		t.Fatal("note not added")
	}
	if h.fakes["alpha"].Calls("GetIssue") <= gets {
		t.Error("issue view should reload to show the new note")
	}
}

func TestDeleteNoteFromIssueView(t *testing.T) {
	h := notesHarness(t, false)
	h.keys("enter", "d", "n")
	if !strings.Contains(h.view(), "first note") || !strings.Contains(h.view(), "second note") {
		t.Fatalf("note picker should list notes:\n%s", h.view())
	}
	h.keys("down", "enter")
	if !strings.Contains(h.view(), "Delete note 61") {
		t.Fatalf("confirm missing:\n%s", h.view())
	}
	h.keys("y")
	if d := h.fakes["alpha"].NotesDeleted; len(d) != 1 || d[0] != [2]int{3, 61} {
		t.Errorf("deleted = %v", d)
	}
	if strings.Contains(h.view(), "second note") {
		t.Error("issue view should reload without the deleted note")
	}
}

func TestDeleteNoteWithNoNotes(t *testing.T) {
	h := notesHarness(t, false)
	h.keys("j", "enter", "d", "n") // #2 has no notes
	if h.m.modal != nil {
		t.Error("no picker when there are no notes")
	}
	if !strings.Contains(lastLine(h.view()), "no notes") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}
