package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFieldKeysFollowTheSelection(t *testing.T) {
	h := batchHarness(t, 5)
	h.keys("space", "space", "space") // #5, #4, #3
	h.keys("s")
	p, ok := h.m.modal.(*picker)
	if !ok || p.title != "Status for 3 issues" {
		t.Fatalf("s with a selection should name it, got %T %+v", h.m.modal, p)
	}
	h.keys("c", "l", "o", "enter")
	if got := len(h.fakes["alpha"].Patches); got != 3 {
		t.Errorf("s should patch the 3 selected issues, got %d patches", got)
	}

	h.keys("s") // the successes left the selection, so this is the cursor issue
	if p, ok := h.m.modal.(*picker); !ok || !strings.HasPrefix(p.title, "Status for #") {
		t.Errorf("without a selection s acts on the cursor issue, got %+v", h.m.modal)
	}
}

func TestBatchChordsAreGone(t *testing.T) {
	h := batchHarness(t, 3)
	h.keys("b")
	if len(h.m.chord.pending) != 0 {
		t.Error("b should no longer start a chord")
	}
}

func TestHostKeys(t *testing.T) {
	hosts := testHosts()
	h := newHarness(t, &hosts[0], nil)
	h.keys("ctrl+h")
	if h.m.modal != nil {
		t.Error("ctrl+h (Backspace in many terminals) must not open the host picker")
	}
	h.keys("H")
	if _, ok := h.m.modal.(*picker); !ok {
		t.Fatal("H should open the host picker")
	}
	h.keys("esc", "2")
	if h.m.cur.sess.Host.Name != "beta" {
		t.Errorf("2 should switch to the second host, on %q", h.m.cur.sess.Host.Name)
	}
	h.keys("1")
	if h.m.cur.sess.Host.Name != "alpha" {
		t.Errorf("1 should switch back to the first host, on %q", h.m.cur.sess.Host.Name)
	}
	h.keys("9")
	if !strings.Contains(lastLine(h.view()), "no host 9") {
		t.Errorf("9 with two hosts should say so: %q", lastLine(h.view()))
	}
}

func TestIssueViewStepsThroughTheList(t *testing.T) {
	h := listHarness(t, 3, nil) // #3, #2, #1
	h.keys("enter", "]")
	if h.m.cur.issue == nil || h.m.cur.issue.id != 2 {
		t.Fatal("] should open the next issue, #2")
	}
	h.keys("]", "]")
	if h.m.cur.issue.id != 1 || !strings.Contains(lastLine(h.view()), "last issue") {
		t.Errorf("] past the end should stay on #1 and say so: #%d %q", h.m.cur.issue.id, lastLine(h.view()))
	}
	h.keys("[")
	if h.m.cur.issue.id != 2 {
		t.Errorf("[ should go back to #2, got #%d", h.m.cur.issue.id)
	}
	h.keys("h")
	if id := h.m.cur.list.currentID(); id != 2 {
		t.Errorf("back in the list the cursor should be on #2, got #%d", id)
	}
}

func TestCopyURL(t *testing.T) {
	h := listHarness(t, 2, nil)
	var copied []string
	h.m.clipboard = func(s string) tea.Cmd {
		copied = append(copied, s)
		return nil
	}
	h.keys("y")
	h.keys("j", "enter", "y")
	want := []string{"https://alpha.example.test/view.php?id=2", "https://alpha.example.test/view.php?id=1"}
	if strings.Join(copied, " ") != strings.Join(want, " ") {
		t.Errorf("copied %v, want %v", copied, want)
	}
	if !strings.Contains(lastLine(h.view()), "copied") {
		t.Errorf("status should confirm the copy: %q", lastLine(h.view()))
	}
}

func TestPreviousUnread(t *testing.T) {
	h := listHarness(t, 5, nil)
	bump(h, 4)
	bump(h, 2)
	h.keys("R", "N") // from #5, backwards wraps to the last unread: #2
	if id := h.m.cur.list.currentID(); id != 2 {
		t.Fatalf("N should wrap backwards to #2, got #%d", id)
	}
	h.keys("N")
	if id := h.m.cur.list.currentID(); id != 4 {
		t.Errorf("N again should reach #4, got #%d", id)
	}
}

func TestHalfPageInList(t *testing.T) {
	h := listHarness(t, 60, nil)
	h.send(winSize(120, 22)) // 20 body rows → half page is 10
	h.keys("ctrl+d")
	if id := h.m.cur.list.currentID(); id != 50 {
		t.Errorf("ctrl+d should move 10 rows to #50, got #%d", id)
	}
	h.keys("pgup")
	if id := h.m.cur.list.currentID(); id != 60 {
		t.Errorf("pgup should move back to #60, got #%d", id)
	}
}

func TestEscClearsSearchThenSelection(t *testing.T) {
	h := batchHarness(t, 5)
	h.keys("space")
	h.keys("/", "2", "enter")
	h.keys("esc")
	if h.m.cur.list.search != "" || len(selectedIDs(h)) != 1 {
		t.Fatalf("first esc clears only the search: search %q, selected %v", h.m.cur.list.search, selectedIDs(h))
	}
	h.keys("esc")
	if len(selectedIDs(h)) != 0 {
		t.Error("second esc clears the selection")
	}
}

func TestHintsNameTheCurrentKeys(t *testing.T) {
	h := listHarness(t, 1, nil)
	h.keys("enter")
	if out := h.view(); !strings.Contains(out, "No notes. r adds one.") {
		t.Errorf("the empty-notes hint should name r:\n%s", out)
	}
	if keyOf(actRefresh) != "R" || keyOf(actFilter) != "f" || keyOf(actEscape) != "esc" {
		t.Errorf("keyOf = %q %q %q", keyOf(actRefresh), keyOf(actFilter), keyOf(actEscape))
	}
}
