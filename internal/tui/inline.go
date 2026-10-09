package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/whleucka/mantis-tui/internal/graphics"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

// Thumbnails are at most thumbRows tall and thumbMaxCols wide. Their image
// ids ride in an indexed colour, so they stay within 16–255.
const (
	thumbRows    = 12
	thumbMaxCols = 80
	firstThumbID = 16
	lastThumbID  = 255
)

// thumb is one image attachment drawn inline.
type thumb struct {
	id    int
	lines []string // the placeholder text, once the terminal has the image
	ready bool
}

// inlineImages tracks the thumbnails sent to the terminal.
type inlineImages struct {
	on        bool // the terminal answered the graphics query
	cell      graphics.Cell
	cellKnown bool
	thumbs    map[string]*thumb // by thumbKey; failed loads stay as not ready
	holder    [lastThumbID + 1]string
	next      int
}

func newInlineImages() inlineImages {
	return inlineImages{cell: graphics.DefaultCell, thumbs: map[string]*thumb{}, next: firstThumbID}
}

func thumbKey(host string, fileID int) string { return host + "/" + strconv.Itoa(fileID) }

// allocate gives key an image id, taking the oldest one back once all are
// in use.
func (im *inlineImages) allocate(key string) int {
	id := im.next
	im.next++
	if im.next > lastThumbID {
		im.next = firstThumbID
	}
	if old := im.holder[id]; old != "" {
		delete(im.thumbs, old)
	}
	im.holder[id] = key
	return id
}

type (
	// thumbReadyMsg carries a thumbnail's transmit sequence.
	thumbReadyMsg struct {
		key        string
		id         int
		seq        string
		cols, rows int
		err        error
	}
	// thumbSent is a transmit sequence. Bubble Tea writes it to the
	// terminal before handing it to Update, so a thumbnail is drawn only
	// once the terminal has its image.
	thumbSent struct {
		key, seq   string
		id         int
		cols, rows int
	}
)

func (s thumbSent) String() string { return s.seq }

// onTerminalReply handles the replies to graphics.Query.
func (m *Model) onTerminalReply(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		if msg.Options.ID != graphics.QueryID || string(msg.Payload) != "OK" || m.img.on {
			return nil
		}
		m.img.on = true
		if m.cur != nil && m.cur.issue != nil {
			return m.loadThumbs(m.cur.issue)
		}
	case uv.CellSizeEvent:
		if msg.Width > 0 && msg.Height > 0 {
			m.img.cell, m.img.cellKnown = graphics.Cell{W: msg.Width, H: msg.Height}, true
		}
	case uv.PixelSizeEvent:
		if !m.img.cellKnown && m.width > 0 && m.height > 0 && msg.Width >= m.width && msg.Height >= m.height {
			m.img.cell = graphics.Cell{W: msg.Width / m.width, H: msg.Height / m.height}
		}
	}
	return nil
}

// loadThumbs fetches and sends the issue's images that have no thumbnail
// yet, in the background.
func (m *Model) loadThumbs(iv *issueModel) tea.Cmd {
	maxCols := min(thumbMaxCols, m.width-4)
	if !m.img.on || iv.issue == nil || m.opts.FilesDir == "" || maxCols < 1 {
		return nil
	}
	host, api, issueID, cell := iv.sess.Host.Name, iv.sess.API, iv.id, m.img.cell
	dir := m.filesDir(host, issueID)
	var cmds []tea.Cmd
	for _, f := range service.Files(iv.issue) {
		key := thumbKey(host, f.ID)
		if !service.IsImage(f.Attachment) || m.img.thumbs[key] != nil {
			continue
		}
		id := m.img.allocate(key)
		m.img.thumbs[key] = &thumb{id: id}
		a := f.Attachment
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
			defer cancel()
			msg := thumbReadyMsg{key: key, id: id}
			path, err := service.Download(ctx, api, issueID, a, dir, true)
			if err == nil {
				msg.seq, msg.cols, msg.rows, err = makeThumb(path, id, cell, maxCols)
			}
			msg.err = err
			return msg
		})
	}
	return tea.Batch(cmds...)
}

// makeThumb loads the image at path and returns the sequence that sends it
// as image id, and the cells it covers.
func makeThumb(path string, id int, cell graphics.Cell, maxCols int) (seq string, cols, rows int, err error) {
	img, err := graphics.Load(path)
	if err != nil {
		return "", 0, 0, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	b := img.Bounds()
	w, h, cols, rows := graphics.Fit(b.Dx(), b.Dy(), cell, maxCols, thumbRows)
	seq, err = graphics.Transmit(id, graphics.Thumbnail(img, w, h, cols, rows, cell), cols, rows)
	return seq, cols, rows, err
}

func (m *Model) onThumbReady(msg thumbReadyMsg) tea.Cmd {
	if t := m.img.thumbs[msg.key]; t == nil || t.id != msg.id || msg.err != nil {
		return nil // taken back meanwhile, or no thumbnail: the attachment line stays
	}
	return tea.Raw(thumbSent{key: msg.key, seq: msg.seq, id: msg.id, cols: msg.cols, rows: msg.rows})
}

func (m *Model) onThumbSent(s thumbSent) {
	t := m.img.thumbs[s.key]
	if t == nil || t.id != s.id {
		return
	}
	t.lines, t.ready = graphics.Placeholder(s.id, s.cols, s.rows), true
	if m.cur != nil && m.cur.issue != nil && strings.HasPrefix(s.key, m.cur.sess.Host.Name+"/") {
		iv := m.cur.issue
		offset := iv.vp.YOffset()
		iv.render(m)
		iv.vp.SetYOffset(offset)
	}
}

// thumbLines is the placeholder text of the issue's ready thumbnails, by
// attachment id.
func (m *Model) thumbLines(host string, is *mantis.Issue) map[int][]string {
	if !m.img.on || is == nil {
		return nil
	}
	out := map[int][]string{}
	for _, f := range service.Files(is) {
		if t := m.img.thumbs[thumbKey(host, f.ID)]; t != nil && t.ready {
			out[f.ID] = t.lines
		}
	}
	return out
}

// ImageCleanup is what deletes every image the TUI sent from the terminal,
// for writing after the program exits.
func (m *Model) ImageCleanup() string {
	var b strings.Builder
	for id, key := range m.img.holder {
		if key != "" {
			b.WriteString(graphics.Delete(id))
		}
	}
	return b.String()
}
