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
	if got.Filter != "assigned" || got.PageSize != 25 || got.Page != 1 {
		t.Errorf("list request = %+v", got)
	}
	if len(got.Select) == 0 {
		t.Error("list should request only list fields")
	}
	if !strings.Contains(h.view(), "Issue number 5 summary") {
		t.Errorf("issues not rendered:\n%s", h.view())
	}
	if !strings.Contains(h.view(), "assigned") || !strings.Contains(h.view(), "page 1") {
		t.Errorf("status bar should show filter and page:\n%s", lastLine(h.view()))
	}
}

func TestFilterChangeReloadsPageOne(t *testing.T) {
	h := listHarness(t, 60, nil)
	h.keys("]") // to page 2
	if h.fakes["alpha"].LastList().Page != 2 {
		t.Fatal("precondition: on page 2")
	}
	h.keys("f")
	if h.m.modal == nil {
		t.Fatal("F should open the filter picker")
	}
	h.keys("m", "o", "enter") // "monitored"
	got := h.fakes["alpha"].LastList()
	if got.Filter != "monitored" || got.Page != 1 {
		t.Errorf("after filter change: %+v", got)
	}
}

func TestPaging(t *testing.T) {
	h := listHarness(t, 60, nil) // page size 50 → 2 pages
	calls := func() int { return len(h.fakes["alpha"].ListCalls) }

	h.keys("[")
	if calls() != 1 {
		t.Error("H on page 1 must not request anything")
	}
	h.keys("]")
	if l := h.fakes["alpha"].LastList(); l.Page != 2 || calls() != 2 {
		t.Fatalf("L: %+v (%d calls)", l, calls())
	}
	if !strings.Contains(h.view(), "Issue number 10 summary") || strings.Contains(h.view(), "Issue number 60 summary") {
		t.Error("page 2 should show the last 10 issues")
	}
	h.keys("]") // page 2 is short → last page
	if calls() != 2 {
		t.Error("L past the last page must be a no-op")
	}
	h.keys("[")
	if l := h.fakes["alpha"].LastList(); l.Page != 1 {
		t.Errorf("H: %+v", l)
	}
}

func TestPagingOntoEmptyPageStays(t *testing.T) {
	h := listHarness(t, 50, nil) // exactly one full page
	h.keys("]")
	if h.m.cur.list.page != 1 {
		t.Errorf("an empty next page should keep page 1, got %d", h.m.cur.list.page)
	}
	if !strings.Contains(h.view(), "Issue number 50 summary") {
		t.Error("issues of page 1 should still be shown")
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
