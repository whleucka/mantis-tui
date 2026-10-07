package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func enum(name string) mantis.EnumValue { return mantis.EnumValue{Name: name, Label: name} }

// seed fills a fake with n issues, ids 1..n.
func seed(n int) func(string, *mantistest.Fake) {
	return func(_ string, f *mantistest.Fake) {
		prios := []string{"immediate", "urgent", "high", "normal", "low", "none"}
		for i := 1; i <= n; i++ {
			is := mantis.Issue{
				ID: i, Summary: fmt.Sprintf("Issue number %d summary", i),
				Project:   mantis.Ref{ID: 4, Name: "Soprano"},
				Category:  mantis.Ref{ID: 1, Name: "General"},
				Status:    enum("assigned"),
				Priority:  enum(prios[i%len(prios)]),
				Severity:  enum("minor"),
				UpdatedAt: time.Date(2026, 9, i%28+1, 12, 0, 0, 0, time.UTC),
			}
			if i == 3 {
				is.Monitors = []mantis.User{{ID: 2}} // the current user
			}
			f.Issues[i] = is
		}
	}
}

func listHarness(t *testing.T, n int, tweak func(*config.Config)) *harness {
	t.Helper()
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], seed(n), tweak)
	return h
}

func TestListLoadsWithConfigDefaults(t *testing.T) {
	h := listHarness(t, 5, func(c *config.Config) {
		c.List.DefaultFilter = "assigned"
		c.List.PageSize = 25
	})
	got := h.fakes["alpha"].LastList()
	if got.Filter != "assigned" || got.PageSize != 25 || got.Page != 1 { // 5 issues fit in one chunk
		t.Errorf("list request = %+v", got)
	}
	if len(got.Select) == 0 {
		t.Error("list should request only list fields")
	}
	if !strings.Contains(h.view(), "Issue number 5 summary") {
		t.Errorf("issues not rendered:\n%s", h.view())
	}
	if !strings.Contains(lastLine(h.view()), "assigned · 5 issues") {
		t.Errorf("status bar should show the filter and the count:\n%s", lastLine(h.view()))
	}
}

func TestFilterChangeReloadsFromTheStart(t *testing.T) {
	h := listHarness(t, 60, func(c *config.Config) { c.List.PageSize = 50 })
	f := h.fakes["alpha"]
	before := len(f.ListCalls)
	h.keys("f")
	if h.m.modal == nil {
		t.Fatal("f should open the filter picker")
	}
	h.keys("m", "o", "enter") // "monitored"
	calls := f.ListCalls[before:]
	if len(calls) != 2 || calls[0].Filter != "monitored" || calls[0].Page != 1 || calls[1].Page != 2 {
		t.Errorf("a new filter should load from chunk 1: %+v", calls)
	}
}

func TestLoadsTheWholeFilterInChunks(t *testing.T) {
	h := listHarness(t, 120, func(c *config.Config) { c.List.PageSize = 50 })
	f := h.fakes["alpha"]
	if got := len(f.ListCalls); got != 3 {
		t.Errorf("120 issues in chunks of 50 should take 3 requests, took %d", got)
	}
	if got := len(h.m.cur.list.issues); got != 120 {
		t.Fatalf("list holds %d issues, want 120", got)
	}
	if !strings.Contains(lastLine(h.view()), "120 issues") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	h.keys("G")
	if !strings.Contains(h.view(), "Issue number 1 summary") {
		t.Error("the last chunk's issues should be reachable")
	}
}

func TestExactChunkMultipleStopsOnEmptyChunk(t *testing.T) {
	h := listHarness(t, 100, func(c *config.Config) { c.List.PageSize = 50 })
	if got := len(h.fakes["alpha"].ListCalls); got != 3 || len(h.m.cur.list.issues) != 100 {
		t.Errorf("100 issues in chunks of 50: %d requests, %d issues", got, len(h.m.cur.list.issues))
	}
}

func TestLoadStopsAtMaxIssues(t *testing.T) {
	h := listHarness(t, 120, func(c *config.Config) {
		c.List.PageSize = 50
		c.List.MaxIssues = 70
	})
	if got := len(h.fakes["alpha"].ListCalls); got != 2 {
		t.Errorf("a cap of 70 should stop after 2 chunks, took %d", got)
	}
	if got := len(h.m.cur.list.issues); got != 70 {
		t.Errorf("list holds %d issues, want 70", got)
	}
	if !strings.Contains(lastLine(h.view()), "(limit 70)") {
		t.Errorf("status should say the cap was hit: %q", lastLine(h.view()))
	}
}

// chunk returns issues for a hand-delivered load response.
func chunk(from, to int) []mantis.Issue {
	var out []mantis.Issue
	for id := from; id >= to; id-- {
		out = append(out, mantis.Issue{ID: id, Summary: fmt.Sprintf("Issue number %d summary", id)})
	}
	return out
}

func TestFirstLoadShowsChunksAsTheyArrive(t *testing.T) {
	h := listHarness(t, 0, func(c *config.Config) { c.List.PageSize = 2 })
	l := h.m.cur.list
	_ = l.load(h.m, 0, true, false) // responses are delivered by hand below
	if next := l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: l.req, page: 1, issues: chunk(9, 8)}); next == nil {
		t.Fatal("a full chunk should ask for the next one")
	}
	if len(l.issues) != 2 || !strings.Contains(l.context(), "loading") {
		t.Errorf("first chunk should show at once with a loading note: %d issues, %q", len(l.issues), l.context())
	}
	l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: l.req, page: 2, issues: chunk(7, 7)})
	if len(l.issues) != 3 || l.inFlight || h.m.loading != 0 {
		t.Errorf("after the short chunk: %d issues, inFlight %v, spinner %d", len(l.issues), l.inFlight, h.m.loading)
	}
}

func TestRefreshSwapsInOnlyWhenComplete(t *testing.T) {
	h := listHarness(t, 3, func(c *config.Config) { c.List.PageSize = 2 })
	l := h.m.cur.list
	h.keys("j") // cursor on #2
	_ = l.load(h.m, 0, false, false)
	l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: l.req, page: 1, issues: chunk(9, 8)})
	if len(l.issues) != 3 || l.issues[0].ID != 3 {
		t.Fatalf("a refresh must keep the old list until the last chunk: %v", ids(l.issues))
	}
	l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: l.req, page: 2, issues: chunk(2, 2)}) // short chunk: the last
	if got := ids(l.issues); len(got) != 3 || got[0] != 9 || l.currentID() != 2 {
		t.Errorf("after the last chunk: %v, cursor #%d (want it kept on #2)", got, l.currentID())
	}
}

func TestFailedChunkKeepsTheOldList(t *testing.T) {
	h := listHarness(t, 3, func(c *config.Config) { c.List.PageSize = 2 })
	l := h.m.cur.list
	_ = l.load(h.m, 0, false, false)
	l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: l.req, page: 1, issues: chunk(9, 8)})
	cmd := l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: l.req, page: 2, err: errors.New("boom")})
	if got := ids(l.issues); len(got) != 3 || got[0] != 3 {
		t.Errorf("a failed chunk must keep the old list, got %v", got)
	}
	if cmd == nil || l.inFlight || h.m.loading != 0 {
		t.Errorf("the error should be reported and the load ended (inFlight %v, spinner %d)", l.inFlight, h.m.loading)
	}
}

func TestSupersededLoadIsIgnored(t *testing.T) {
	h := listHarness(t, 3, nil)
	l := h.m.cur.list
	_ = l.load(h.m, 0, true, false)
	old := l.req
	_ = l.load(h.m, 0, true, false)
	if h.m.loading != 1 {
		t.Errorf("replacing a load should not stack spinners, loading = %d", h.m.loading)
	}
	if l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: old, page: 1, issues: chunk(9, 9)}); len(l.issues) != 3 {
		t.Error("a response for a replaced load must be ignored")
	}
}

func TestLoadErrorGoesToStatusBar(t *testing.T) {
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(_ string, f *mantistest.Fake) {
		f.Errs = map[string]error{"ListIssues": errors.New("connection refused")}
	}, nil)
	if !strings.Contains(lastLine(h.view()), "connection refused") {
		t.Errorf("status bar = %q", lastLine(h.view()))
	}
	if h.m.loading != 0 {
		t.Error("spinner should stop after an error")
	}
}

func TestRowsShowIconsAndColumns(t *testing.T) {
	h := listHarness(t, 6, nil)
	out := h.view()
	for _, want := range []string{"ID", "SEVERITY", "STATUS", "CATEGORY", "UPDATED", "SUMMARY", "🔥", "🔺", "👁️", "General", "2026-09-"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

func TestColumnsFitTheWidth(t *testing.T) {
	for _, w := range []int{120, 80, 60} {
		h := listHarness(t, 3, nil)
		h.send(winSize(w, 24))
		for i, line := range strings.Split(h.view(), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("width %d: line %d is %d cells: %q", w, i, got, ansi.Strip(line))
			}
		}
		if !strings.Contains(h.view(), "Issue number 3") {
			t.Errorf("width %d: summary should stay visible", w)
		}
	}
}

func TestCursorMovement(t *testing.T) {
	h := listHarness(t, 5, nil)
	if id := h.m.cur.list.currentID(); id != 5 {
		t.Fatalf("cursor starts on first row, got #%d", id)
	}
	h.keys("j", "j")
	if id := h.m.cur.list.currentID(); id != 3 {
		t.Errorf("after jj: #%d", id)
	}
	h.keys("G")
	if id := h.m.cur.list.currentID(); id != 1 {
		t.Errorf("after G: #%d", id)
	}
	h.keys("g", "g")
	if id := h.m.cur.list.currentID(); id != 5 {
		t.Errorf("after gg: #%d", id)
	}
	h.keys("k")
	if id := h.m.cur.list.currentID(); id != 5 {
		t.Errorf("k at top should clamp: #%d", id)
	}
}

func TestRefreshKeepsCursorOnSameIssue(t *testing.T) {
	h := listHarness(t, 5, nil)
	h.keys("j", "j") // #3
	h.fakes["alpha"].Issues[9] = mantis.Issue{ID: 9, Summary: "new on top"}
	h.keys("R")
	if id := h.m.cur.list.currentID(); id != 3 {
		t.Errorf("after refresh cursor on #%d, want #3", id)
	}
}

func TestEmptyList(t *testing.T) {
	h := listHarness(t, 0, nil)
	if !strings.Contains(h.view(), "No issues") {
		t.Errorf("empty state missing:\n%s", h.view())
	}
	h.keys("j", "enter", "G") // must not panic
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return ansi.Strip(lines[len(lines)-1])
}

func TestFailedFirstLoadSaysSoInTheList(t *testing.T) {
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(_ string, f *mantistest.Fake) {
		f.Errs = map[string]error{"ListIssues": errors.New("connection refused")}
	}, nil)
	h.keys("j") // any key clears the status bar message
	out := h.view()
	if strings.Contains(out, "Loading issues") || !strings.Contains(out, "connection refused") || !strings.Contains(out, "R to retry") {
		t.Fatalf("a failed first load must not look like it is still loading:\n%s", out)
	}
	h.fakes["alpha"].Errs = nil
	h.fakes["alpha"].Issues[1] = mantis.Issue{ID: 1, Summary: "Back again"}
	h.keys("R")
	if !strings.Contains(h.view(), "Back again") {
		t.Errorf("R should retry:\n%s", h.view())
	}
}
