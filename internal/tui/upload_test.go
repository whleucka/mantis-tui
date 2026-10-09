package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

// withClipboard makes the clipboard hold img (nil for none).
func withClipboard(h *harness, img *mantis.FileUpload) {
	h.m.opts.Clipboard = func(context.Context) *mantis.FileUpload { return img }
}

func shot(t *testing.T) *mantis.FileUpload {
	return &mantis.FileUpload{Name: "screenshot-20261009-074512.png", Content: pngBytes(t, 40, 20)}
}

func TestNoteWithClipboardImage(t *testing.T) {
	h := notesHarness(t, false)
	withClipboard(h, shot(t))
	stubEditor(h, "see screenshot\n")
	h.keys("r")
	if v := h.view(); !strings.Contains(v, "[x] clipboard image") || !strings.Contains(v, "40×20") {
		t.Fatalf("the clipboard image should be offered, checked:\n%s", v)
	}
	h.keys("enter")
	added := h.fakes["alpha"].NotesAdded
	if len(added) != 1 || added[0].Note.Text != "see screenshot" || len(added[0].Note.Files) != 1 ||
		added[0].Note.Files[0].Name != "screenshot-20261009-074512.png" {
		t.Fatalf("notes added = %+v", added)
	}
	if status := lastLine(h.view()); !strings.Contains(status, "added with 1 file") {
		t.Errorf("status = %q", status)
	}
}

func TestNoteClipboardImageCanBeUnchecked(t *testing.T) {
	h := notesHarness(t, false)
	withClipboard(h, shot(t))
	stubEditor(h, "text only\n")
	h.keys("r", "down", "space")
	if !strings.Contains(h.view(), "[ ] clipboard image") {
		t.Fatalf("space should uncheck it:\n%s", h.view())
	}
	h.keys("enter")
	if added := h.fakes["alpha"].NotesAdded; len(added) != 1 || len(added[0].Note.Files) != 0 {
		t.Errorf("notes added = %+v", added)
	}
}

func TestNoteFilesWithPathCompletion(t *testing.T) {
	h := notesHarness(t, false)
	withClipboard(h, nil)
	stubEditor(h, "logs attached\n")
	dir := t.TempDir()
	for name, body := range map[string]string{"app.log": "boom", "apple.txt": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h.keys("r", "down") // private, then Files: there is no clipboard row
	if strings.Contains(h.view(), "clipboard image") {
		t.Fatal("no clipboard row without an image")
	}
	h.typeText(dir + "/ap")
	h.keys("tab")
	if v := h.view(); !strings.Contains(v, "app.log  apple.txt") {
		t.Errorf("tab should list both matches:\n%s", v)
	}
	h.typeText(".")
	h.keys("tab", "enter")
	if v := ansi.Strip(h.view()); !strings.Contains(v, "app.log · 4 B") {
		t.Fatalf("app.log should be listed after enter:\n%s", v)
	}
	h.typeText(dir + "/nope")
	h.keys("enter")
	if !strings.Contains(h.view(), "no such file") {
		t.Errorf("a missing path is refused:\n%s", h.view())
	}
	for range len(dir + "/nope") {
		h.keys("backspace")
	}
	h.keys("enter") // empty input: open the editor
	added := h.fakes["alpha"].NotesAdded
	if len(added) != 1 || len(added[0].Note.Files) != 1 || string(added[0].Note.Files[0].Content) != "boom" {
		t.Fatalf("notes added = %+v", added)
	}
}

func TestEmptyNoteWithAttachmentAsksFirst(t *testing.T) {
	h := notesHarness(t, false)
	withClipboard(h, shot(t))
	stubEditor(h, "")
	h.keys("r", "enter")
	if !strings.Contains(h.view(), "Send 1 attachment without text?") {
		t.Fatalf("should ask:\n%s", h.view())
	}
	h.keys("n")
	if len(h.fakes["alpha"].NotesAdded) != 0 || !strings.Contains(lastLine(h.view()), "note discarded") {
		t.Fatalf("n discards: %+v, status %q", h.fakes["alpha"].NotesAdded, lastLine(h.view()))
	}

	h.keys("r", "enter", "y")
	added := h.fakes["alpha"].NotesAdded
	if len(added) != 1 || added[0].Note.Text != "" || len(added[0].Note.Files) != 1 {
		t.Errorf("y sends the attachment alone: %+v", added)
	}
	if files, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "mantis-*")); len(files) != 0 {
		t.Errorf("temp files left: %v", files)
	}
}

func TestNoteOverTheLimitStaysInTheForm(t *testing.T) {
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(3)(name, f)
		f.ConfigValues["max_file_size"] = json.RawMessage("10") // read with the enums at startup
	}, nil)
	withClipboard(h, shot(t)) // more than 10 bytes
	opened := false
	stubEditor(h, "x")
	inner := h.m.execEditor
	h.m.execEditor = func(s *editor.Session, done func(error) tea.Msg) tea.Cmd { opened = true; return inner(s, done) }
	h.keys("r", "enter")
	if opened || !strings.Contains(h.view(), "the server takes files up to 10 B") {
		t.Errorf("no editor before the limits pass:\n%s", h.view())
	}
}

func TestClipboardThumbnailInTheNoteForm(t *testing.T) {
	h := notesHarness(t, false)
	h.m.opts.FilesDir = t.TempDir()
	withClipboard(h, shot(t))
	h.send(graphicsOK)
	h.keys("r")
	if !strings.Contains(h.view(), placeholder) {
		t.Errorf("the form should draw the clipboard image:\n%s", h.view())
	}
}

func TestCreateWithClipboardImage(t *testing.T) {
	h := createHarness(t)
	withClipboard(h, shot(t))
	stubEditor(h, "desc\n")
	h.keys("C")
	if !strings.Contains(h.view(), "screenshot-20261009-074512.png") {
		t.Fatalf("the Attachments row should show the clipboard image:\n%s", h.view())
	}
	h.field("category")
	h.keys("enter", "g", "e", "n", "enter")
	h.field("summary")
	h.typeText("Button misplaced")
	h.field("description")
	h.keys("e")
	h.field("attachments")
	h.keys("enter")
	if !strings.Contains(h.view(), "Attachments for the new issue") || !strings.Contains(h.view(), "[x] clipboard image") {
		t.Fatalf("enter opens the attachments modal:\n%s", h.view())
	}
	h.keys("esc", "alt+enter")
	created := h.fakes["alpha"].Created
	if len(created) != 1 || len(created[0].Files) != 1 || created[0].Files[0].Name != "screenshot-20261009-074512.png" {
		t.Fatalf("created = %+v", created)
	}
	if status := lastLine(h.view()); !strings.Contains(status, "created with 1 file") {
		t.Errorf("status = %q", status)
	}
}
