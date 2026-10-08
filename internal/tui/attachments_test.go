package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

type shown struct {
	path, title string
	cols, rows  int
	editing     bool // was refreshing paused when the viewer started?
}

// attachHarness opens #3, which has a PDF on the issue and two images on a
// note; #2 has a shell script; #1 has nothing.
func attachHarness(t *testing.T) (*harness, *[]string, *[]shown) {
	t.Helper()
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(3)(name, f)
		is := f.Issues[3]
		is.Attachments = []mantis.Attachment{{ID: 1, Filename: "guide.pdf", Size: 3, ContentType: "application/pdf"}}
		is.Notes = []mantis.Note{{ID: 41, Text: "screens", Reporter: mantis.User{Name: "bob"}, Attachments: []mantis.Attachment{
			{ID: 2, Filename: "image.png", Size: 4, ContentType: "image/png; charset=binary"},
			{ID: 3, Filename: "image-2.png", Size: 4, ContentType: "image/png; charset=binary"},
		}}}
		f.Issues[3] = is
		is2 := f.Issues[2]
		is2.Attachments = []mantis.Attachment{{ID: 4, Filename: "fix.sh", Size: 2}}
		f.Issues[2] = is2
		f.FileContent = map[int][]byte{1: []byte("pdf"), 2: []byte("\x89PNG"), 3: []byte("\x89PNG"), 4: []byte("rm")}
	}, nil)
	opened, images := &[]string{}, &[]shown{}
	h.m.opts.FilesDir = t.TempDir()
	h.m.opts.OpenFile = func(path string) error { *opened = append(*opened, path); return nil }
	h.m.opts.ShowImage = func(path, title string, cols, rows int, done func(error) tea.Msg) tea.Cmd {
		*images = append(*images, shown{path, title, cols, rows, h.m.editing})
		return func() tea.Msg { return done(nil) }
	}
	return h, opened, images
}

func TestAttachmentPickerListsIssueAndNoteFiles(t *testing.T) {
	h, _, _ := attachHarness(t)
	h.keys("enter", "g", "a")
	v := h.view()
	for _, want := range []string{"Open which attachment of #3?", "guide.pdf", "3 B · issue", "image-2.png", "4 B · note 41 by bob"} {
		if !strings.Contains(v, want) {
			t.Errorf("picker missing %q:\n%s", want, v)
		}
	}
}

func TestOpenImageShowsItInTheTerminal(t *testing.T) {
	h, opened, images := attachHarness(t)
	h.keys("enter", "g", "a", "down", "enter")
	if len(*images) != 1 {
		t.Fatalf("images shown = %+v", *images)
	}
	img := (*images)[0]
	want := filepath.Join(h.m.opts.FilesDir, "alpha", "3", "2-image.png")
	if img.path != want || img.title != "image.png · note 41 by bob" || img.cols != 120 || img.rows != 40 || !img.editing {
		t.Errorf("shown = %+v, want path %s", img, want)
	}
	if b, err := os.ReadFile(want); err != nil || string(b) != "\x89PNG" {
		t.Errorf("cached file = %q, %v", b, err)
	}
	if h.m.editing {
		t.Error("refreshing must resume after the viewer returns")
	}
	if len(*opened) != 0 {
		t.Errorf("an image shown in the terminal is not also opened: %v", *opened)
	}
}

func TestOpenPDFUsesTheDesktopOpener(t *testing.T) {
	h, opened, images := attachHarness(t)
	h.keys("enter", "g", "a", "enter")
	if len(*images) != 0 || len(*opened) != 1 || filepath.Base((*opened)[0]) != "1-guide.pdf" {
		t.Fatalf("opened = %v, images = %v", *opened, *images)
	}
	if !strings.Contains(lastLine(h.view()), "opened guide.pdf") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}

func TestImageFallsBackToOpenerWithoutGraphics(t *testing.T) {
	h, opened, _ := attachHarness(t)
	h.m.opts.ShowImage = func(_, _ string, _, _ int, done func(error) tea.Msg) tea.Cmd {
		return func() tea.Msg { return done(ErrNoGraphics) }
	}
	h.keys("enter", "g", "a", "down", "enter")
	if len(*opened) != 1 || filepath.Base((*opened)[0]) != "2-image.png" {
		t.Errorf("opened = %v", *opened)
	}

	h.m.opts.ShowImage = nil // no kitten at all
	h.keys("g", "a", "down", "down", "enter")
	if len(*opened) != 2 || filepath.Base((*opened)[1]) != "3-image-2.png" {
		t.Errorf("opened = %v", *opened)
	}
}

func TestUnsafeFileIsSavedNotOpened(t *testing.T) {
	h, opened, _ := attachHarness(t)
	h.keys("j", "enter", "g", "a") // #2's only file opens at once
	if len(*opened) != 0 {
		t.Errorf("a shell script must not be opened: %v", *opened)
	}
	if status := lastLine(h.view()); !strings.Contains(status, "saved") || !strings.Contains(status, "4-fix.sh") {
		t.Errorf("status = %q", status)
	}
}

func TestCachedAttachmentIsNotFetchedAgain(t *testing.T) {
	h, opened, _ := attachHarness(t)
	h.keys("enter", "g", "a", "enter")
	h.fakes["alpha"].Errs = map[string]error{"GetFile": errors.New("offline")}
	h.keys("g", "a", "enter")
	if len(*opened) != 2 {
		t.Errorf("the cached copy should open again: %v, status %q", *opened, lastLine(h.view()))
	}
}

func TestNoAttachments(t *testing.T) {
	h, _, _ := attachHarness(t)
	h.keys("j", "j", "enter", "g", "a")
	if h.m.modal != nil || !strings.Contains(lastLine(h.view()), "#1 has no attachments") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}
