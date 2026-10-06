package tui

import (
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func mixedProjects(_ string, f *mantistest.Fake) {
	add := func(id int, project, summary, category, handler string) {
		is := mantis.Issue{ID: id, Summary: summary, Project: mantis.Ref{ID: len(project), Name: project},
			Category: mantis.Ref{Name: category}, Status: enum("new"), Priority: enum("normal")}
		if handler != "" {
			is.Handler = &mantis.User{ID: 9, Name: handler, RealName: strings.ToUpper(handler)}
		}
		f.Issues[id] = is
	}
	add(6, "Zeta", "zeta login bug", "Backend", "bob")
	add(5, "Alpha", "alpha crash on save", "Frontend", "")
	add(4, "Zeta", "zeta slow search", "Backend", "alice")
	add(3, "Alpha", "alpha typo", "Docs", "bob")
}

func extrasHarness(t *testing.T, tweak func(*config.Config)) *harness {
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], mixedProjects, tweak)
}

func TestGroupByProjectKeepsCursorAndSkipsHeaders(t *testing.T) {
	h := extrasHarness(t, nil)
	h.keys("j", "j") // #4 (zeta slow search)
	h.keys("ctrl+g")
	if id := h.m.cur.list.currentID(); id != 4 {
		t.Fatalf("after grouping cursor on #%d, want #4", id)
	}
	out := h.view()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Zeta (2)") {
		t.Errorf("group headers missing:\n%s", out)
	}
	if strings.Index(out, "alpha crash") > strings.Index(out, "zeta login") {
		t.Error("groups should be ordered by project name")
	}

	// Grouped order: Alpha[5,3] Zeta[6,4]. From #4 moving up visits 6, 3, 5 —
	// never a header.
	var seen []int
	for range 3 {
		h.keys("k")
		seen = append(seen, h.m.cur.list.currentID())
	}
	if want := []int{6, 3, 5}; !equalInts(seen, want) {
		t.Errorf("cursor visited %v, want %v (headers skipped)", seen, want)
	}
	h.keys("k")
	if id := h.m.cur.list.currentID(); id != 5 {
		t.Errorf("k at the first issue should stay, got #%d", id)
	}

	h.keys("ctrl+g")
	if id := h.m.cur.list.currentID(); id != 5 || strings.Contains(h.view(), "Zeta (2)") {
		t.Errorf("ungrouping should keep #5 and drop headers, cursor #%d", id)
	}
}

func TestGroupByProjectFromConfig(t *testing.T) {
	h := extrasHarness(t, func(c *config.Config) { c.List.GroupByProject = true })
	if !strings.Contains(h.view(), "Zeta (2)") {
		t.Error("group_by_project = true should start grouped")
	}
	if id := h.m.cur.list.currentID(); id != 5 {
		t.Errorf("cursor should start on the first issue row, got #%d", id)
	}
}

func TestSearchNarrowsAndEscRestores(t *testing.T) {
	h := extrasHarness(t, nil)
	h.keys("j") // #5
	h.keys("/")
	h.send(key("z"), key("e"), key("t"))
	out := h.view()
	if strings.Contains(out, "alpha crash") || !strings.Contains(out, "zeta login") || !strings.Contains(out, "zeta slow") {
		t.Errorf("search 'zet' should show only zeta issues:\n%s", out)
	}
	h.keys("enter")
	if h.m.modal != nil {
		t.Error("enter should close the search input")
	}
	if !strings.Contains(h.view(), "/zet") {
		t.Error("active search should stay visible")
	}
	h.keys("j")
	if id := h.m.cur.list.currentID(); id != 4 {
		t.Errorf("moving within results: #%d", id)
	}
	h.keys("esc")
	if !strings.Contains(h.view(), "alpha crash") {
		t.Error("esc should clear the search")
	}
	if id := h.m.cur.list.currentID(); id != 4 {
		t.Errorf("after clearing, cursor on #%d, want #4", id)
	}
}

func TestSearchMatchesIDCategoryAndHandler(t *testing.T) {
	for q, want := range map[string][]int{
		"3":        {3},
		"docs":     {3},
		"alice":    {4},
		"BOB":      {6, 3},
		"zt srch":  {4}, // fuzzy: each term is a subsequence
		"nothing!": {},
	} {
		h := extrasHarness(t, nil)
		h.keys("/")
		for _, r := range q {
			h.send(key(string(r)))
		}
		var got []int
		for _, r := range h.m.cur.list.rows() {
			if r.idx >= 0 {
				got = append(got, h.m.cur.list.issues[r.idx].ID)
			}
		}
		if !equalInts(got, want) {
			t.Errorf("search %q = %v, want %v", q, got, want)
		}
	}
}

func TestSearchEscWhileTypingClears(t *testing.T) {
	h := extrasHarness(t, nil)
	h.keys("/")
	h.send(key("z"))
	h.keys("esc")
	if h.m.modal != nil || h.m.cur.list.search != "" {
		t.Error("esc while typing should cancel the search")
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
