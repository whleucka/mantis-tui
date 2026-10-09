package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/whleucka/mantis-tui/internal/graphics"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

const placeholder = string(kitty.Placeholder)

// pngBytes is a w×h PNG.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// graphicsOK is the terminal's reply to graphics.Query.
var graphicsOK = uv.KittyGraphicsEvent{Options: kitty.Options{ID: graphics.QueryID}, Payload: []byte("OK")}

// thumbHarness is #3 with a 40×20 PNG on note 41, a broken "PNG" on note 42
// and a PDF on the issue, with attachments cached in a temp dir.
func thumbHarness(t *testing.T, tweak func(f *mantistest.Fake)) *harness {
	t.Helper()
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(3)(name, f)
		is := f.Issues[3]
		is.Attachments = []mantis.Attachment{{ID: 1, Filename: "guide.pdf", Size: 3, ContentType: "application/pdf"}}
		is.Notes = []mantis.Note{
			{ID: 41, Text: "screens", Reporter: mantis.User{Name: "bob"}, Attachments: []mantis.Attachment{
				{ID: 2, Filename: "image.png", ContentType: "image/png"},
			}},
			{ID: 42, Text: "broken", Reporter: mantis.User{Name: "bob"}, Attachments: []mantis.Attachment{
				{ID: 3, Filename: "bad.png", Size: 4, ContentType: "image/png"},
			}},
		}
		img := pngBytes(t, 40, 20)
		is.Notes[0].Attachments[0].Size = int64(len(img))
		f.Issues[3] = is
		f.FileContent = map[int][]byte{1: []byte("pdf"), 2: img, 3: []byte("nope")}
		if tweak != nil {
			tweak(f)
		}
	}, nil)
	h.m.opts.FilesDir = t.TempDir()
	return h
}

func TestNoThumbnailsWithoutGraphics(t *testing.T) {
	h := thumbHarness(t, nil)
	h.keys("enter")
	if strings.Contains(h.view(), placeholder) || h.fakes["alpha"].Calls("GetFile") != 0 {
		t.Errorf("nothing is fetched or drawn before the terminal says OK:\n%s", h.view())
	}
	if h.m.ImageCleanup() != "" {
		t.Errorf("cleanup = %q, want nothing", h.m.ImageCleanup())
	}
}

func TestThumbnailDrawnUnderItsNote(t *testing.T) {
	h := thumbHarness(t, nil)
	h.send(graphicsOK)
	h.keys("enter")
	v := h.view()
	lines := strings.Split(v, "\n")
	at := -1
	for i, l := range lines {
		if strings.Contains(l, "attachment: image.png") {
			at = i
		}
	}
	if at < 0 || at+1 >= len(lines) || !strings.Contains(lines[at+1], placeholder) {
		t.Fatalf("the thumbnail should follow its attachment line:\n%s", v)
	}
	// 40×20 pixels in 10×20 cells: 4 columns, 1 row, then the next note.
	if got := strings.Count(lines[at+1], placeholder); got != 4 {
		t.Errorf("thumbnail is %d cells wide, want 4", got)
	}
	if strings.Contains(lines[at+2], placeholder) {
		t.Errorf("thumbnail should be one row tall:\n%s", v)
	}
	if !strings.Contains(v, "attachment: bad.png") || strings.Count(v, placeholder) != 4 {
		t.Errorf("the broken image keeps just its line:\n%s", v)
	}
	if strings.Contains(lastLine(v), "nope") || strings.Contains(lastLine(v), "decode") {
		t.Errorf("a broken thumbnail is not an error: %q", lastLine(v))
	}
	if got := h.fakes["alpha"].Calls("GetFile"); got != 2 {
		t.Errorf("GetFile calls = %d, want the two images only", got)
	}
}

// The thumbnail appears only once Bubble Tea has written its transmit
// sequence, which it hands back to Update as a RawMsg.
func TestThumbnailWaitsForTheTransmit(t *testing.T) {
	h := thumbHarness(t, nil)
	h.send(graphicsOK)
	h.m.img.on = false // open the issue without loading thumbnails
	h.keys("enter")
	h.m.img.on = true
	cmd := h.m.loadThumbs(h.m.cur.issue)
	var ready thumbReadyMsg
	for _, msg := range run(cmd) {
		for _, c := range msg.(tea.BatchMsg) {
			if r, ok := run(c)[0].(thumbReadyMsg); ok && r.err == nil {
				ready = r
			}
		}
	}
	if ready.seq == "" {
		t.Fatal("no thumbnail was made")
	}
	_, raw := h.m.Update(ready)
	if strings.Contains(h.view(), placeholder) {
		t.Fatal("drawn before the terminal has the image")
	}
	msgs := run(raw)
	rm, ok := msgs[0].(tea.RawMsg)
	if !ok || !strings.HasPrefix(fmt.Sprint(rm.Msg), "\x1b_G") {
		t.Fatalf("want the transmit as raw output, got %#v", msgs)
	}
	h.send(rm)
	if !strings.Contains(h.view(), placeholder) {
		t.Errorf("drawn once written:\n%s", h.view())
	}
}

func TestGraphicsReplyLoadsTheOpenIssue(t *testing.T) {
	h := thumbHarness(t, nil)
	h.keys("enter")
	h.send(uv.KittyGraphicsEvent{Options: kitty.Options{ID: 99}, Payload: []byte("OK")})
	h.send(uv.KittyGraphicsEvent{Options: kitty.Options{ID: graphics.QueryID}, Payload: []byte("ENOTSUPPORTED")})
	if h.m.img.on {
		t.Fatal("only an OK to our query turns thumbnails on")
	}
	h.send(graphicsOK)
	if !strings.Contains(h.view(), placeholder) {
		t.Errorf("the open issue should get its thumbnails:\n%s", h.view())
	}
	if c := h.m.ImageCleanup(); c != graphics.Delete(16)+graphics.Delete(17) {
		t.Errorf("cleanup = %q, want ids 16 and 17 deleted", c)
	}
}

func TestPreviewPaneHasNoThumbnails(t *testing.T) {
	h := thumbHarness(t, nil)
	h.m.previewDelay = 0
	h.send(graphicsOK)
	h.keys("enter", "q") // load the thumbnails, then back to the list
	h.send(winSize(160, 40))
	v := h.view()
	if !strings.Contains(v, "attachment: image.png") {
		t.Fatalf("the preview should show #3's notes:\n%s", v)
	}
	if strings.Contains(v, placeholder) {
		t.Errorf("the preview pane draws no thumbnails:\n%s", v)
	}
}

func TestThumbIDsAreReusedOldestFirst(t *testing.T) {
	im := newInlineImages()
	for i := range lastThumbID - firstThumbID + 1 {
		key := thumbKey("alpha", i)
		if id := im.allocate(key); id != firstThumbID+i {
			t.Fatalf("id %d for file %d", id, i)
		}
		im.thumbs[key] = &thumb{}
	}
	if id := im.allocate("alpha/new"); id != firstThumbID {
		t.Errorf("id = %d, want the oldest, %d", id, firstThumbID)
	}
	if im.thumbs[thumbKey("alpha", 0)] != nil {
		t.Error("the evicted thumbnail must be forgotten so it loads again")
	}
}

func TestCellSizeFromTheTerminal(t *testing.T) {
	h := thumbHarness(t, nil)
	h.send(uv.PixelSizeEvent{Width: 1200, Height: 1200}) // 120×40 cells
	if h.m.img.cell != (graphics.Cell{W: 10, H: 30}) {
		t.Errorf("cell from window pixels = %+v", h.m.img.cell)
	}
	h.send(uv.CellSizeEvent{Width: 9, Height: 18})
	h.send(uv.PixelSizeEvent{Width: 2400, Height: 2400})
	if h.m.img.cell != (graphics.Cell{W: 9, H: 18}) {
		t.Errorf("a reported cell size wins: %+v", h.m.img.cell)
	}
}
