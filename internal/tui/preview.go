package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

const (
	previewMinWidth = 140                    // narrower terminals get the full-width list
	previewDelay    = 150 * time.Millisecond // cursor rest time before fetching
)

// preview is the list's right-hand pane showing the issue under the cursor.
type preview struct {
	on    bool
	want  int // issue waiting for its debounce tick or fetch; 0 when none
	gen   int // current debounce; older ticks are ignored
	cache map[int]*mantis.Issue
	errs  map[int]error
	vp    viewport.Model

	// what the viewport currently holds, so it is only rebuilt on change
	shownID      int
	shownUpdated time.Time
	shownWidth   int
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

func newPreview(on bool) preview {
	return preview{on: on, cache: map[int]*mantis.Issue{}, errs: map[int]error{}, vp: viewport.New()}
}

// previewWidths splits width between the list and the preview; pw is 0 when
// the preview is not shown.
func (l *listModel) previewWidths(width int) (lw, pw int) {
	if !l.pv.on || width < previewMinWidth {
		return width, 0
	}
	pw = min(max(width*45/100, 50), 100)
	return width - pw - 1, pw // 1 for the rule
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
	_, pw := l.previewWidths(m.width)
	if pw == 0 || m.cur == nil || m.cur.list != l || m.cur.screen != screenList {
		return nil
	}
	is := m.currentIssue()
	if is == nil {
		return nil
	}
	if c := l.fresh(*is); c != nil {
		l.pv.show(c, max(pw-2, 10), max(m.height-1, 1)) // one column of padding each side
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

// show puts is in the viewport, rebuilding it only when the issue, its
// version or the size changed.
func (p *preview) show(is *mantis.Issue, w, h int) {
	p.vp.SetHeight(h)
	if is.ID == p.shownID && is.UpdatedAt.Equal(p.shownUpdated) && w == p.shownWidth {
		return
	}
	p.vp.SetWidth(w)
	offset := p.vp.YOffset()
	p.vp.SetContent(renderIssue(is, tabPreview, w))
	if is.ID == p.shownID {
		p.vp.SetYOffset(offset) // same issue refreshed: keep the reading position
	} else {
		p.vp.GotoTop()
	}
	p.shownID, p.shownUpdated, p.shownWidth = is.ID, is.UpdatedAt, w
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

func (l *listModel) onPreviewLoaded(_ *Model, msg previewLoadedMsg) tea.Cmd {
	if msg.err != nil {
		l.pv.errs[msg.id] = msg.err
		return nil // stays in want, so it is retried only when the cursor comes back
	}
	delete(l.pv.errs, msg.id)
	l.pv.cache[msg.id] = msg.issue
	if l.pv.want == msg.id {
		l.pv.want = 0
	}
	return nil
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
	if l.pv.on && m.width < previewMinWidth {
		return infoCmd(l.host(), fmt.Sprintf("the preview needs a terminal at least %d columns wide", previewMinWidth))
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
func withPreview(list, pane string, lw, pw, h int) string {
	left := lipgloss.NewStyle().Width(lw).Height(h).MaxHeight(h).Render(list)
	rule := styleMuted.Render(strings.TrimRight(strings.Repeat("│\n", h), "\n"))
	right := lipgloss.NewStyle().Width(pw).Height(h).MaxHeight(h).Render(pane)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, rule, right)
}
