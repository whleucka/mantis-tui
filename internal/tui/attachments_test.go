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
	editing     bool     // was refreshing paused when the viewer started?
	titles      []string // every image the viewer can step to
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
	h.m.opts.OpenFolder = h.m.opts.OpenFile
	h.m.opts.ShowImage = func(imgs []ViewerImage, start, cols, rows int, done func(error) tea.Msg) tea.Cmd {
		path, err := imgs[start].Fetch()
		if err != nil {
			t.Fatal(err)
		}
		titles := make([]string, len(imgs))
		for i, img := range imgs {
			titles[i] = img.Title
		}
		*images = append(*images, shown{path, imgs[start].Title, cols, rows, h.m.editing, titles})
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
	if got := strings.Join(img.titles, ", "); got != "image.png · note 41 by bob, image-2.png · note 41 by bob" {
		t.Errorf("the viewer should step through the issue's images only, got %s", got)
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
	h.m.opts.ShowImage = func(_ []ViewerImage, _, _, _ int, done func(error) tea.Msg) tea.Cmd {
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

// fileLine is the screen row showing attachment id in the issue view.
func fileLine(t *testing.T, h *harness, id int) int {
	t.Helper()
	iv := h.m.cur.issue
	for line, sp := range iv.fileLines {
		if sp.id == id {
			if y := line - iv.vp.YOffset(); y >= 0 && y < iv.vp.Height() {
				return y
			}
		}
	}
	t.Fatalf("attachment %d not on screen: %v", id, iv.fileLines)
	return 0
}

func TestClickAttachmentOpensIt(t *testing.T) {
	h, opened, images := attachHarness(t)
	h.keys("enter")
	y := fileLine(t, h, 1)
	if !strings.Contains(strings.Split(h.view(), "\n")[y], "guide.pdf") {
		t.Fatalf("row %d does not show guide.pdf:\n%s", y, h.view())
	}
	h.send(click(20, y))
	if len(*opened) != 1 || filepath.Base((*opened)[0]) != "1-guide.pdf" {
		t.Fatalf("clicking the PDF should open it, opened = %v", *opened)
	}
	h.send(click(20, fileLine(t, h, 3)))
	if len(*images) != 1 || filepath.Base((*images)[0].path) != "3-image-2.png" {
		t.Errorf("clicking a note's image should show it, images = %+v", *images)
	}
}

func TestClickOnlyTheNameOpensAnAttachment(t *testing.T) {
	h, opened, _ := attachHarness(t)
	h.keys("enter")
	y := fileLine(t, h, 1)
	h.send(click(5, y)) // the "Attachment" label
	h.send(click(60, y))
	if len(*opened) != 0 {
		t.Fatalf("clicks beside the name should not open it, opened = %v", *opened)
	}
	h.send(click(16, y)) // the name's first column
	if len(*opened) != 1 {
		t.Errorf("a click on the name should open it, opened = %v", *opened)
	}
}

func TestDownloadAllThenOpenFolder(t *testing.T) {
	h, opened, _ := attachHarness(t)
	h.keys("enter", "g", "f")
	if len(*opened) != 0 || !strings.Contains(lastLine(h.view()), "nothing saved for #3 yet") {
		t.Fatalf("gf before gA: opened %v, status %q", *opened, lastLine(h.view()))
	}
	h.keys("g", "A")
	dir := filepath.Join(h.m.opts.FilesDir, "alpha", "3")
	for _, name := range []string{"1-guide.pdf", "2-image.png", "3-image-2.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("gA should save %s: %v", name, err)
		}
	}
	if !strings.Contains(lastLine(h.view()), "saved 3 attachments") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	h.keys("g", "f")
	if len(*opened) != 1 || (*opened)[0] != dir {
		t.Errorf("gf should open %s, opened = %v", dir, *opened)
	}
}

func TestDownloadAllWithNoAttachments(t *testing.T) {
	h, _, _ := attachHarness(t)
	h.keys("j", "j", "enter", "g", "A")
	if !strings.Contains(lastLine(h.view()), "#1 has no attachments") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}
