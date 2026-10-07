package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func wheel(x, y int, down bool) tea.MouseWheelMsg {
	b := tea.MouseWheelUp
	if down {
		b = tea.MouseWheelDown
	}
	return tea.MouseWheelMsg{X: x, Y: y, Button: b}
}

// fakeClock replaces the model's clock; advance moves it forward.
func fakeClock(h *harness) (advance func(time.Duration)) {
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	h.m.now = func() time.Time { return now }
	return func(d time.Duration) { now = now.Add(d) }
}

func TestMouseEnabledByConfig(t *testing.T) {
	h := listHarness(t, 1, nil)
	if h.m.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("mouse should be on by default")
	}
	off := listHarness(t, 1, func(c *config.Config) { c.UI.Mouse = false })
	if off.m.View().MouseMode != tea.MouseModeNone {
		t.Error("ui.mouse = false should leave mouse reporting off")
	}
}

func TestClickMovesCursorAndDoubleClickOpens(t *testing.T) {
	h := listHarness(t, 5, nil) // rows #5..#1 start at screen row 1
	advance := fakeClock(h)
	h.send(click(20, 3))
	if id := h.m.cur.list.currentID(); id != 3 {
		t.Fatalf("click on the third row should select #3, got #%d", id)
	}
	advance(time.Second)
	h.send(click(20, 3))
	if h.m.cur.screen != screenList {
		t.Fatal("a slow second click must not open the issue")
	}
	advance(100 * time.Millisecond)
	h.send(click(20, 3))
	if h.m.cur.screen != screenIssue || h.m.cur.issue.id != 3 {
		t.Fatal("a quick second click should open #3")
	}
}

func TestClickOutsideRowsDoesNothing(t *testing.T) {
	h := listHarness(t, 2, nil)
	for _, y := range []int{0, 10, 39} { // header, empty space, status bar
		h.send(click(5, y))
		if id := h.m.cur.list.currentID(); id != 2 {
			t.Errorf("click at row %d moved the cursor to #%d", y, id)
		}
	}
}

func TestClickRespectsScrollAndSearch(t *testing.T) {
	h := listHarness(t, 60, nil)
	h.send(winSize(120, 12)) // 10 issue rows fit
	h.keys("G")              // scroll to the bottom
	l := h.m.cur.list
	h.send(click(20, 1))
	if id := l.currentID(); id != l.issues[l.offset].ID {
		t.Errorf("click on the first visible row should pick the row at the scroll offset, got #%d", id)
	}

	h.keys("/", "5", "0", "enter") // one match, shown under the search line
	h.send(click(20, 2))
	if id := l.currentID(); id != 50 {
		t.Errorf("with a search line, row 2 is the first match; got #%d", id)
	}
}

func TestWheelMovesListCursor(t *testing.T) {
	h := listHarness(t, 10, nil)
	h.send(wheel(10, 5, true))
	if id := h.m.cur.list.currentID(); id != 7 {
		t.Errorf("wheel down should move three rows to #7, got #%d", id)
	}
	h.send(wheel(10, 5, false))
	if id := h.m.cur.list.currentID(); id != 10 {
		t.Errorf("wheel up should move back to #10, got #%d", id)
	}
}

func TestWheelOverPreviewScrollsIt(t *testing.T) {
	h := previewHarness(t, 1, nil)
	f := h.fakes["alpha"]
	is := f.Issues[1]
	is.UpdatedAt = is.UpdatedAt.Add(time.Minute)
	is.Description = strings.Repeat("long line\n", 100)
	f.Issues[1] = is
	h.keys("r")
	h.send(wheel(150, 10, true))
	if h.m.cur.list.pv.vp.YOffset() != wheelStep {
		t.Errorf("wheel over the preview should scroll it, offset = %d", h.m.cur.list.pv.vp.YOffset())
	}
	if id := h.m.cur.list.currentID(); id != 1 {
		t.Error("wheel over the preview must not move the list cursor")
	}
	h.send(click(150, 3))
	if h.m.cur.screen != screenList {
		t.Error("a click in the preview should not open anything")
	}
}

func TestIssueViewWheelAndTabClick(t *testing.T) {
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(1)(name, f)
		is := f.Issues[1]
		is.Description = strings.Repeat("line\n", 60)
		is.History = []mantis.HistoryEntry{{Message: "Status changed"}}
		f.Issues[1] = is
	}, nil)
	h.keys("enter")
	iv := h.m.cur.issue
	h.send(wheel(10, 5, true))
	if iv.vp.YOffset() != wheelStep {
		t.Fatalf("wheel should scroll the issue view, offset = %d", iv.vp.YOffset())
	}
	h.keys("G")
	y := iv.tabLine - iv.vp.YOffset()
	if y < 0 || y >= 39 {
		t.Fatalf("tab line %d not on screen (offset %d)", iv.tabLine, iv.vp.YOffset())
	}
	h.send(click(len(" Notes (0) ")+3, y))
	if iv.tab != tabHistory {
		t.Fatal("clicking the History label should switch tabs")
	}
	h.send(click(2, iv.tabLine-iv.vp.YOffset()))
	if iv.tab != tabNotes {
		t.Error("clicking the Notes label should switch back")
	}
}

// modalLine returns the screen position of line n inside the open modal.
func modalLine(h *harness, n int) (x, y int) {
	box := h.m.modal.view(h.m.width, h.m.height)
	x, y = modalOrigin(box, h.m.width, h.m.height)
	return x + 3, y + 1 + n
}

func TestPickerWheelAndClick(t *testing.T) {
	h := actionsHarness(t)
	h.keys("s")
	p, ok := h.m.modal.(*picker)
	if !ok {
		t.Fatal("s should open the status picker")
	}
	before := p.cursor
	h.send(wheel(60, 10, true))
	if p.cursor != before+1 {
		t.Errorf("wheel should move the picker cursor, %d → %d", before, p.cursor)
	}
	want := p.options[p.visible()[1]].label
	h.send(click(modalLine(h, 2))) // line 0 is the title, 1 the first option
	if h.m.modal != nil {
		t.Fatal("clicking an option should choose it and close the picker")
	}
	if got := lastPatch(t, h.fakes["alpha"]); got.Patch.Status == nil || !strings.EqualFold(got.Patch.Status.Name, want) {
		t.Errorf("patch = %+v, want status %q", got, want)
	}
}

func TestPaletteClick(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys(":", "s", "t", "a", "t")
	h.send(click(modalLine(h, 1))) // first command: Change status
	if p, ok := h.m.modal.(*picker); !ok || !strings.Contains(p.title, "tatus") {
		t.Errorf("clicking Change status should open its picker, got %T", h.m.modal)
	}
}

func TestHelpWheelAndModalsWithoutMouse(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.send(winSize(80, 12))
	h.keys("?")
	hm := h.m.modal.(*helpModal)
	h.send(wheel(40, 5, true))
	if hm.offset != 1 {
		t.Errorf("wheel should scroll help, offset = %d", hm.offset)
	}
	h.keys("esc", "S") // summary prompt: a text modal that ignores the mouse
	h.send(click(1, 1), wheel(1, 1, true))
	if h.m.modal == nil {
		t.Error("the mouse must not close a text prompt")
	}
}
