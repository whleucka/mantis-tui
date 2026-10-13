package tui

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func refreshHarness(t *testing.T, n int) *harness {
	t.Helper()
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], seed(n), func(c *config.Config) {
		c.List.AutoRefresh.Duration = time.Hour // never fires in tests; ticks are sent by hand
		c.Issue.AutoRefresh.Duration = time.Hour
	})
}

func (h *harness) listTick() {
	l := h.m.cur.list
	h.send(refreshTickMsg{host: l.host(), target: "list", gen: l.tickGen})
}

func TestAutoRefreshFollowsIssueWhenRowsReorder(t *testing.T) {
	h := refreshHarness(t, 5)
	h.keys("j", "j") // #3
	calls := len(h.fakes["alpha"].ListCalls)

	// #3 moves to the top (fake orders by id desc, so give it the highest id
	// by adding newer issues above and removing one below).
	h.fakes["alpha"].Issues[10] = mantis.Issue{ID: 10, Summary: "brand new"}
	delete(h.fakes["alpha"].Issues, 5)
	h.listTick()

	if len(h.fakes["alpha"].ListCalls) != calls+1 {
		t.Fatal("tick should re-fetch the list")
	}
	if id := h.m.cur.list.currentID(); id != 3 {
		t.Errorf("cursor on #%d after refresh, want #3", id)
	}
	if h.m.loading != 0 {
		t.Error("auto-refresh should be silent (no spinner)")
	}
	if !strings.Contains(h.view(), "brand new") {
		t.Error("refreshed rows should be shown")
	}
}

// sweepDue moves the clock to when the next auto-refresh reloads the
// whole filter.
func (h *harness) sweepDue() {
	later := h.m.now().Add(fullSweepEvery)
	h.m.now = func() time.Time { return later }
}

func TestAutoRefreshCurrentIssueDisappears(t *testing.T) {
	h := refreshHarness(t, 5)
	h.keys("G") // #1, the last row
	delete(h.fakes["alpha"].Issues, 1)
	h.sweepDue() // only a full sweep sees deletions
	h.listTick()
	if id := h.m.cur.list.currentID(); id == 0 || id == 1 {
		t.Errorf("cursor should clamp to a remaining issue, got #%d", id)
	}
}

func TestAutoRefreshDisabledSchedulesNothing(t *testing.T) {
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], seed(3), nil) // harness default: interval 0
	if h.m.cur.list.tickGen != 0 {
		t.Error("interval 0 must not schedule ticks")
	}
	h.keys("enter")
	if h.m.cur.issue.tickGen != 0 {
		t.Error("issue interval 0 must not schedule ticks")
	}
}

func TestAutoRefreshSkippedWhileModalOpenOrLoading(t *testing.T) {
	h := refreshHarness(t, 3)
	calls := func() int { return len(h.fakes["alpha"].ListCalls) }
	before := calls()

	h.keys("f") // filter picker open
	h.listTick()
	if calls() != before {
		t.Error("no refresh while a modal is open")
	}
	h.keys("esc")

	h.m.cur.list.inFlight = true
	h.listTick()
	if calls() != before {
		t.Error("no refresh while a request is in flight")
	}
	h.m.cur.list.inFlight = false

	h.listTick()
	if calls() != before+1 {
		t.Error("refresh should resume afterwards")
	}
}

func TestStaleTickIgnored(t *testing.T) {
	h := refreshHarness(t, 3)
	before := len(h.fakes["alpha"].ListCalls)
	h.send(refreshTickMsg{host: "alpha", target: "list", gen: h.m.cur.list.tickGen - 1})
	if len(h.fakes["alpha"].ListCalls) != before {
		t.Error("a tick from an old chain must be ignored")
	}
}

func TestAutoRefreshIssueKeepsScroll(t *testing.T) {
	is := fixtureIssue(t)
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(_ string, f *mantistest.Fake) { f.Issues[33] = is }, func(c *config.Config) {
		c.Issue.AutoRefresh.Duration = time.Hour
	})
	h.send(winSize(80, 12))
	h.keys("enter", "j", "j", "j")
	iv := h.m.cur.issue
	gets := h.fakes["alpha"].Calls("GetIssue")

	h.send(refreshTickMsg{host: "alpha", target: "issue", gen: iv.tickGen})
	if h.fakes["alpha"].Calls("GetIssue") != gets+1 {
		t.Fatal("tick should reload the issue")
	}
	if iv.vp.YOffset() != 3 {
		t.Errorf("scroll offset %d after refresh, want 3", iv.vp.YOffset())
	}

	h.keys("q")
	gets = h.fakes["alpha"].Calls("GetIssue")
	h.send(refreshTickMsg{host: "alpha", target: "issue", gen: iv.tickGen})
	if h.fakes["alpha"].Calls("GetIssue") != gets {
		t.Error("a closed issue view must not refresh")
	}
}

// changesHarness serves n issues, #n the most recently updated, three to a
// page, with writes that move updated_at forward like Mantis.
func changesHarness(t *testing.T, n int) *harness {
	t.Helper()
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(_ string, f *mantistest.Fake) {
		f.BumpOnWrite = true
		for i := 1; i <= n; i++ {
			f.Issues[i] = mantis.Issue{ID: i, Summary: fmt.Sprintf("Issue %d", i), Status: enum("new"), UpdatedAt: testNow.Add(time.Duration(i-n) * time.Hour)}
		}
	}, func(c *config.Config) {
		c.List.AutoRefresh.Duration = time.Hour // ticks are sent by hand
		c.List.PageSize = 3
	})
}

// touchIssue makes issue id the most recently updated, with a new summary.
func (h *harness) touchIssue(id int, summary string) {
	f := h.fakes["alpha"]
	is := f.Issues[id]
	is.Summary, is.UpdatedAt = summary, h.m.now().Add(time.Minute)
	f.Issues[id] = is
}

func (h *harness) listCalls() []mantis.ListOptions { return h.fakes["alpha"].ListCalls }

func TestIncrementalRefreshFetchesOnlyTheChanges(t *testing.T) {
	h := changesHarness(t, 10)
	before := len(h.listCalls())
	if before != 4 {
		t.Fatalf("the first load should read all 4 pages, read %d", before)
	}
	h.touchIssue(2, "Changed by someone")
	h.listTick()

	calls := h.listCalls()[before:]
	if len(calls) != 1 || calls[0].Page != 1 {
		t.Fatalf("one change should cost one request for page 1, got %+v", calls)
	}
	l := h.m.cur.list
	if len(l.issues) != 10 || !strings.Contains(h.view(), "Changed by someone") {
		t.Errorf("the change should be merged into all 10 issues, got %v", ids(l.issues))
	}
}

func TestIncrementalRefreshPagesUntilItReachesOlderIssues(t *testing.T) {
	h := changesHarness(t, 10)
	before := len(h.listCalls())
	for _, id := range []int{1, 2, 3, 4} { // a whole page and one more
		h.touchIssue(id, fmt.Sprintf("Changed %d", id))
	}
	h.listTick()
	if n := len(h.listCalls()) - before; n != 2 {
		t.Errorf("four changes at three a page should take 2 requests, took %d", n)
	}
	for _, id := range []int{1, 2, 3, 4} {
		if !strings.Contains(h.view(), fmt.Sprintf("Changed %d", id)) {
			t.Errorf("change to #%d is missing", id)
		}
	}
}

func TestIncrementalRefreshWithNoChangesKeepsTheList(t *testing.T) {
	h := changesHarness(t, 10)
	l := h.m.cur.list
	shown := &l.issues[0]
	h.keys("j", "j")
	h.listTick()
	if &l.issues[0] != shown || l.currentID() != 8 {
		t.Errorf("an empty refresh should leave the list alone (cursor #%d)", l.currentID())
	}
}

func TestStickyIssuesDontEndAnIncrementalRefresh(t *testing.T) {
	h := changesHarness(t, 10)
	f := h.fakes["alpha"]
	old := f.Issues[1]
	old.Sticky = true // first on every page 1, and older than everything
	f.Issues[1] = old
	before := len(h.listCalls())
	h.touchIssue(5, "Changed 5")
	h.touchIssue(6, "Changed 6")
	h.listTick()
	// page 1 is sticky #1, #6, #5: nothing older apart from the sticky one
	if n := len(h.listCalls()) - before; n != 2 {
		t.Errorf("a sticky issue must not count as reaching older issues: %d requests", n)
	}
}

func TestIncrementalRefreshAnnouncesNewIssues(t *testing.T) {
	h := changesHarness(t, 10)
	f := h.fakes["alpha"]
	f.Issues[11] = mantis.Issue{ID: 11, Summary: "Fresh one", Reporter: mantis.User{ID: 9}, UpdatedAt: h.m.now().Add(time.Minute)}
	h.listTick()
	if !strings.Contains(lastLine(h.view()), "1 new: #11 Fresh one") {
		t.Errorf("status bar = %q", lastLine(h.view()))
	}
}

func TestFullSweepDropsWhatLeftTheFilter(t *testing.T) {
	h := changesHarness(t, 10)
	delete(h.fakes["alpha"].Issues, 5)
	h.listTick()
	if len(h.m.cur.list.issues) != 10 {
		t.Fatal("an incremental refresh can't know #5 is gone")
	}
	before := len(h.listCalls())
	h.sweepDue()
	h.listTick()
	if n := len(h.listCalls()) - before; n != 4 { // 3 full pages of 9, then an empty one
		t.Errorf("the sweep should read the whole filter, read %d pages", n)
	}
	if got := ids(h.m.cur.list.issues); len(got) != 9 {
		t.Errorf("after the sweep: %v", got)
	}
	before = len(h.listCalls())
	h.listTick()
	if n := len(h.listCalls()) - before; n != 1 {
		t.Errorf("the tick after a sweep is incremental again, read %d pages", n)
	}
}

func TestManualRefreshReadsTheWholeFilter(t *testing.T) {
	h := changesHarness(t, 10)
	before := len(h.listCalls())
	h.keys("R")
	if n := len(h.listCalls()) - before; n != 4 {
		t.Errorf("R should reload all 4 pages, read %d", n)
	}
}

func TestIncrementalRefreshKeepsTheCap(t *testing.T) {
	h := changesHarness(t, 0)
	l := h.m.cur.list
	l.maxIssues = 4
	f := h.fakes["alpha"]
	for i := 1; i <= 6; i++ {
		f.Issues[i] = mantis.Issue{ID: i, UpdatedAt: testNow.Add(time.Duration(i-6) * time.Hour)}
	}
	h.keys("R")
	if got := ids(l.issues); len(got) != 4 || !l.capped {
		t.Fatalf("the cap should hold 4, got %v", got)
	}
	h.touchIssue(1, "Changed 1") // the oldest comes back to the top
	h.listTick()
	got := ids(l.issues)
	sort.Ints(got)
	if want := []int{1, 4, 5, 6}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the merge should keep the 4 most recently updated, got %v", got)
	}
}
