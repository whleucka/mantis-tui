package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	doubleClick = 400 * time.Millisecond
	wheelStep   = 3 // rows or lines per wheel notch
)

// mouseModal is a modal that also takes the mouse. Modals without it (text
// prompts, confirmations) ignore the mouse.
type mouseModal interface {
	wheel(delta int)
	// click handles a click on line, counted from the first line inside the
	// border, in a terminal height rows tall.
	click(line, height int) (cmd tea.Cmd, done bool)
}

// modalOrigin is where overlay puts box on the screen.
func modalOrigin(box string, width, height int) (x, y int) {
	return max((width-lipgloss.Width(box))/2, 0), max((height-lipgloss.Height(box))/3, 0)
}

func wheelDelta(b tea.MouseButton) int {
	switch b {
	case tea.MouseWheelUp:
		return -1
	case tea.MouseWheelDown:
		return 1
	}
	return 0
}

func (m *Model) onMouse(msg tea.MouseMsg) tea.Cmd {
	mouse := msg.Mouse()
	if m.modal != nil {
		return m.mouseOnModal(msg, mouse)
	}
	if m.cur == nil || mouse.Y >= max(m.height-1, 1) { // the status bar
		return nil
	}
	_, wheel := msg.(tea.MouseWheelMsg)
	_, click := msg.(tea.MouseClickMsg)
	click = click && mouse.Button == tea.MouseLeft
	switch {
	case m.cur.screen == screenIssue && m.cur.issue != nil:
		iv := m.cur.issue
		if wheel {
			iv.wheel(wheelDelta(mouse.Button))
		} else if click {
			iv.clickTabs(m, mouse.X, mouse.Y)
		}
	case m.cur.screen == screenList:
		l := m.cur.list
		if wheel {
			l.wheel(m, mouse.X, wheelDelta(mouse.Button))
		} else if click {
			return l.click(m, mouse.X, mouse.Y)
		}
	}
	return nil
}

func (m *Model) mouseOnModal(msg tea.MouseMsg, mouse tea.Mouse) tea.Cmd {
	mm, ok := m.modal.(mouseModal)
	if !ok {
		return nil
	}
	switch msg.(type) {
	case tea.MouseWheelMsg:
		mm.wheel(wheelDelta(mouse.Button))
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return nil
		}
		box := m.modal.view(m.width, m.height)
		x, y := modalOrigin(box, m.width, m.height)
		if mouse.X < x || mouse.X >= x+lipgloss.Width(box) {
			return nil
		}
		cmd, done := mm.click(mouse.Y-y-1, m.height) // 1 for the top border
		if done {
			m.modal = nil
		}
		return cmd
	}
	return nil
}

// list

// rowsTop is the screen row of the first issue row.
func (l *listModel) rowsTop() int {
	if l.search != "" {
		return 2 // search line and column header
	}
	return 1
}

func (l *listModel) wheel(m *Model, x, delta int) {
	if lw, pw := l.previewWidths(m.width); pw > 0 && x > lw {
		if delta < 0 {
			l.pv.vp.ScrollUp(wheelStep)
		} else {
			l.pv.vp.ScrollDown(wheelStep)
		}
		return
	}
	l.move(m, delta*wheelStep)
}

// click moves the cursor to the clicked row, or opens it when the row was
// already clicked moments ago.
func (l *listModel) click(m *Model, x, y int) tea.Cmd {
	if lw, pw := l.previewWidths(m.width); pw > 0 && x >= lw {
		return nil
	}
	rs := l.rows()
	i := l.offset + y - l.rowsTop()
	if !l.loaded || y < l.rowsTop() || i < 0 || i >= len(rs) || i >= l.offset+bodyRows(m) || rs[i].idx < 0 {
		return nil
	}
	id := l.issues[rs[i].idx].ID
	now := m.now()
	double := l.cursor == i && l.lastClickID == id && now.Sub(l.lastClickAt) <= doubleClick
	l.cursor = i
	l.scrollToCursor(m)
	if double {
		l.lastClickID = 0
		return m.openIssue(id)
	}
	l.lastClickID, l.lastClickAt = id, now
	return nil
}

// issue view

func (iv *issueModel) wheel(delta int) {
	if delta < 0 {
		iv.vp.ScrollUp(wheelStep)
	} else {
		iv.vp.ScrollDown(wheelStep)
	}
}

// clickTabs switches tabs when the click lands on a tab label.
func (iv *issueModel) clickTabs(m *Model, x, y int) {
	if iv.issue == nil || iv.tabLine < 0 || y+iv.vp.YOffset() != iv.tabLine {
		return
	}
	notesW := len(fmt.Sprintf(" Notes (%d) ", len(iv.issue.Notes)))
	historyW := len(fmt.Sprintf(" History (%d) ", len(iv.issue.History)))
	tab := -1
	switch {
	case x < notesW:
		tab = tabNotes
	case x > notesW && x <= notesW+historyW:
		tab = tabHistory
	}
	if tab >= 0 && tab != iv.tab {
		iv.tab = tab
		offset := iv.vp.YOffset()
		iv.render(m)
		iv.vp.SetYOffset(offset)
	}
}

// modals

// window is the slice of options the picker shows: the first index and
// how many rows fit.
func (p *picker) window(height int) (start, rows int) {
	rows = max(height-6, 3)
	if p.cursor >= rows {
		start = p.cursor - rows + 1
	}
	return start, rows
}

func (p *picker) wheel(delta int) {
	p.cursor = min(max(p.cursor+delta, 0), max(len(p.visible())-1, 0))
}

func (p *picker) click(line, height int) (tea.Cmd, bool) {
	vis := p.visible()
	start, rows := p.window(height)
	row := line - 1 // the title takes the first line
	if row < 0 || row >= rows || start+row >= len(vis) {
		return nil, false
	}
	return p.onSelect(p.options[vis[start+row]]), true
}

func (p *palette) window(height int) (start, rows int) {
	rows = max(height-6, 3)
	return max(p.cursor-rows+1, 0), rows
}

func (p *palette) wheel(delta int) {
	p.cursor = min(max(p.cursor+delta, 0), max(len(p.visible())-1, 0))
}

func (p *palette) click(line, height int) (tea.Cmd, bool) {
	vis := p.visible()
	start, rows := p.window(height)
	row := line - 1 // the query takes the first line
	if row < 0 || row >= rows || start+row >= len(vis) {
		return nil, false
	}
	run := vis[start+row].run
	return func() tea.Msg { return paletteRunMsg{run: run} }, true
}

func (h *helpModal) wheel(delta int) {
	h.offset = min(max(h.offset+delta, 0), max(len(h.entries)-1, 0))
}

func (h *helpModal) click(int, int) (tea.Cmd, bool) { return nil, false }
