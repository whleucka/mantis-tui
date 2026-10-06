package tui

import (
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

func TestAutoRefreshCurrentIssueDisappears(t *testing.T) {
	h := refreshHarness(t, 5)
	h.keys("G") // #1, the last row
	delete(h.fakes["alpha"].Issues, 1)
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

	h.keys("F") // filter picker open
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
