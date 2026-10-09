package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

const (
	previewDelay = 150 * time.Millisecond // cursor rest time before fetching

	// "auto" puts the preview on the right from this width, and below the
	// list on a portrait-shaped terminal (rows×2 ≥ columns, since a cell is
	// about twice as tall as wide) with at least this many rows.
	autoRightWidth = 140
	autoBottomRows = 40
	// A forced layout only needs room for both parts.
	minRightWidth = 100
	minBottomRows = 24
)

// preview is the list's right-hand pane showing the issue under the cursor.
type preview struct {
	on     bool
	layout string // auto | right | bottom
	want   int    // issue waiting for its debounce tick or fetch; 0 when none
	gen    int    // current debounce; older ticks are ignored
	cache  map[int]*mantis.Issue
	errs   map[int]error
	vp     viewport.Model

	// what the viewport currently holds, so it is only rebuilt on change
	shown       *mantis.Issue // the cached copy; a refetch replaces it
	shownID     int
	shownWidth  int
	shownColors bool // status colours had arrived
	shownCode   *chroma.Style
}

type (
	previewTickMsg struct {
		host string
		id   int
		gen  int
	}
	previewLoadedMsg struct {
		host  string
		id    int
		issue *mantis.Issue
		err   error
	}
)

func (msg previewTickMsg) hostName() string   { return msg.host }
func (msg previewLoadedMsg) hostName() string { return msg.host }

func newPreview(on bool, layout string) preview {
	return preview{on: on, layout: layout, cache: map[int]*mantis.Issue{}, errs: map[int]error{}, vp: viewport.New()}
}

// paneLayout is how the list body splits between the list and the preview.
type paneLayout struct {
	right, below bool // where the preview is; neither when it is hidden
	lw, lh       int  // the list's block
	pw, ph       int  // the preview's block
}

func (p paneLayout) shown() bool { return p.right || p.below }

// layout splits a width×height body. A rule (1 column or row) separates
// the parts.
func (l *listModel) layout(width, height int) paneLayout {
	full := paneLayout{lw: width, lh: height}
	if !l.pv.on {
		return full
	}
	right := width >= minRightWidth
	below := height >= minBottomRows
	switch l.pv.layout {
	case "right":
		below = false
	case "bottom":
		right = false
	default:
		right = width >= autoRightWidth
		below = !right && height >= autoBottomRows && height*2 >= width
	}
	switch {
	case right:
		pw := min(max(width*45/100, 40), 100)
		return paneLayout{right: true, lw: width - pw - 1, lh: height, pw: pw, ph: height}
	case below:
		lh := max(height*40/100, 8)
		return paneLayout{below: true, lw: width, lh: lh, pw: width, ph: height - lh - 1}
	}
	return full
}

// bodyLayout is the layout of the list screen for the current terminal.
func (l *listModel) bodyLayout(m *Model) paneLayout {
	return l.layout(m.width, max(m.height-1, 1)) // the status bar takes a row
}

// fresh returns the cached issue if it is at least as new as the list row.
func (l *listModel) fresh(row mantis.Issue) *mantis.Issue {
	if c := l.pv.cache[row.ID]; c != nil && !row.UpdatedAt.After(c.UpdatedAt) {
		return c
	}
	return nil
}

// syncPreview runs after every update. It lays out the issue under the
// cursor when a current copy is cached, and otherwise schedules a fetch.
func (l *listModel) syncPreview(m *Model) tea.Cmd {
	lay := l.bodyLayout(m)
	if !lay.shown() || m.cur == nil || m.cur.list != l || m.cur.screen != screenList {
		return nil
	}
	is := m.currentIssue()
	if is == nil {
		return nil
	}
	if c := l.fresh(*is); c != nil {
		l.pv.show(c, max(lay.pw-2, 10), max(lay.ph, 1), m.look(l.host())) // one column of padding each side
		return nil
	}
	if l.pv.want == is.ID {
		return nil
	}
	l.pv.want = is.ID
	l.pv.gen++
	msg := previewTickMsg{host: l.host(), id: is.ID, gen: l.pv.gen}
	return tea.Tick(m.previewDelay, func(time.Time) tea.Msg { return msg })
}

// show puts is in the viewport, rebuilding it only when the cached copy or
// the size changed.
func (p *preview) show(is *mantis.Issue, w, h int, lk issueLook) {
	p.vp.SetHeight(h)
	if is == p.shown && w == p.shownWidth && (lk.colors != nil) == p.shownColors && lk.code == p.shownCode {
		return
	}
	p.vp.SetWidth(w)
	offset := p.vp.YOffset()
	p.vp.SetContent(renderIssue(is, tabPreview, w, lk))
	if is.ID == p.shownID {
		p.vp.SetYOffset(offset) // same issue refreshed: keep the reading position
	} else {
		p.vp.GotoTop()
	}
	p.shown, p.shownID, p.shownWidth, p.shownColors, p.shownCode = is, is.ID, w, lk.colors != nil, lk.code
}

// onPreviewTick fetches the issue if the cursor is still resting on it.
func (l *listModel) onPreviewTick(msg previewTickMsg) tea.Cmd {
	if msg.gen != l.pv.gen || msg.id != l.pv.want {
		return nil
	}
	host, api, id := l.host(), l.sess.API, msg.id
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		res, err := api.GetIssue(ctx, id)
		out := previewLoadedMsg{host: host, id: id, err: err}
		if err == nil {
			out.issue = &res.Issue
		}
		return out
	}
}

func (l *listModel) onPreviewLoaded(m *Model, msg previewLoadedMsg) tea.Cmd {
	if msg.err != nil {
		l.pv.errs[msg.id] = msg.err
		return nil // stays in want, so it is retried only when the cursor comes back
	}
	delete(l.pv.errs, msg.id)
	l.pv.cache[msg.id] = msg.issue
	if l.pv.want == msg.id {
		l.pv.want = 0
	}
	return m.markSeen(l.host(), *msg.issue)
}

// forget drops the cached copy of id so the next sync fetches it again.
func (p *preview) forget(id int) {
	delete(p.cache, id)
	if p.want == id {
		p.want = 0
	}
}

func (l *listModel) togglePreview(m *Model) tea.Cmd {
	l.pv.on = !l.pv.on
	if l.pv.on && !l.bodyLayout(m).shown() {
		need := map[string]string{
			"right":  fmt.Sprintf("at least %d columns", minRightWidth),
			"bottom": fmt.Sprintf("at least %d rows", minBottomRows),
		}[l.pv.layout]
		if need == "" {
			need = fmt.Sprintf("%d columns, or %d rows on a portrait screen", autoRightWidth, autoBottomRows)
		}
		return infoCmd(l.host(), "the preview needs a bigger terminal: "+need)
	}
	return nil
}

// previewView renders the pane for row in w columns.
func (l *listModel) previewView(row *mantis.Issue, w int) string {
	pad := lipgloss.NewStyle().Padding(0, 1)
	if row == nil {
		return pad.Render(styleMuted.Render("No issue selected."))
	}
	if l.fresh(*row) != nil && l.pv.shownID == row.ID {
		return pad.Render(l.pv.vp.View())
	}
	head := styleTitle.Render(fmt.Sprintf("#%d %s", row.ID, row.Summary))
	note := "Loading…"
	if err := l.pv.errs[row.ID]; err != nil {
		note = "Could not load: " + err.Error()
	}
	return pad.Width(w).Render(head + "\n\n" + styleMuted.Render(note))
}

// withPreview joins the list block and the preview pane with a rule.
func withPreview(list, pane string, lay paneLayout) string {
	block := func(s string, w, h int) string {
		return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(s)
	}
	if lay.below {
		rule := styleMuted.Render(strings.Repeat("─", lay.lw))
		return lipgloss.JoinVertical(lipgloss.Left, block(list, lay.lw, lay.lh), rule, block(pane, lay.pw, lay.ph))
	}
	rule := styleMuted.Render(strings.TrimRight(strings.Repeat("│\n", lay.lh), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, block(list, lay.lw, lay.lh), rule, block(pane, lay.pw, lay.ph))
}
