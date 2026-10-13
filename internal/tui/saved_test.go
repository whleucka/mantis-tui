package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

// savedHarness starts with lists saved by an earlier run in a temp dir,
// serving n issues from the fake (optionally failing).
func savedHarness(t *testing.T, n int, failing bool, saved map[string][]mantis.Issue) (*harness, *config.Lists) {
	t.Helper()
	lists := config.NewLists(t.TempDir())
	alpha := testHosts()[0]
	for filter, issues := range saved {
		if err := lists.Save(alpha, filter, issues, testNow.Add(-3*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarnessOpts(t, &alpha, func(host string, f *mantistest.Fake) {
		seed(n)(host, f)
		if failing {
			f.Errs = map[string]error{"ListIssues": errors.New("connection refused")}
		}
	}, func(c *config.Config) { c.List.DefaultFilter = "assigned" }, func(o *Options) { o.Lists = lists })
	return h, lists
}

func TestSavedListShowsUntilTheLoadIsIn(t *testing.T) {
	h, _ := savedHarness(t, 3, false, nil)
	l := h.m.cur.list
	// Start over by hand to see the moment before the network answers.
	_ = l.load(h.m, 0, true, false)
	l.onSaved(h.m, savedListMsg{host: "alpha", filter: "assigned", list: config.SavedList{Issues: chunk(42, 41)}})
	if got := ids(l.issues); len(got) != 2 || got[0] != 42 || !strings.Contains(l.context(), "saved list, refreshing") {
		t.Fatalf("the saved list should show at once: %v, %q", got, l.context())
	}
	l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: l.req, page: 1, issues: chunk(9, 9)})
	if got := ids(l.issues); len(got) != 1 || got[0] != 9 || strings.Contains(l.context(), "saved") {
		t.Errorf("the loaded list should replace the saved one: %v, %q", got, l.context())
	}
}

func TestSavedListWaitsForTheWholeLoad(t *testing.T) {
	h, _ := savedHarness(t, 0, false, nil)
	l := h.m.cur.list
	l.pageSize = 2
	_ = l.load(h.m, 0, true, false)
	l.onSaved(h.m, savedListMsg{host: "alpha", filter: "assigned", list: config.SavedList{Issues: chunk(42, 40)}})
	l.onChunk(h.m, issuesLoadedMsg{host: "alpha", req: l.req, page: 1, issues: chunk(9, 8)})
	if got := ids(l.issues); len(got) != 3 || got[0] != 42 {
		t.Errorf("a partial load must not replace the saved list (it would shrink): %v", got)
	}
}

func TestLateSavedListIsIgnored(t *testing.T) {
	h, _ := savedHarness(t, 3, false, nil)
	l := h.m.cur.list
	l.onSaved(h.m, savedListMsg{host: "alpha", filter: "assigned", list: config.SavedList{Issues: chunk(42, 41)}})
	if got := ids(l.issues); len(got) != 3 || got[0] != 3 {
		t.Errorf("a saved list after the real one must be dropped: %v", got)
	}
}

func TestSavedListServesWhenTheServerIsDown(t *testing.T) {
	h, _ := savedHarness(t, 0, true, map[string][]mantis.Issue{"assigned": chunk(42, 41)})
	l := h.m.cur.list
	if got := ids(l.issues); len(got) != 2 || !strings.Contains(l.context(), "saved 3h ago") {
		t.Errorf("offline, the saved list should stay with its age: %v, %q", got, l.context())
	}
	if !strings.Contains(lastLine(h.view()), "connection refused") {
		t.Errorf("the load error should still be reported: %q", lastLine(h.view()))
	}
}

func TestCompleteLoadIsSavedForNextTime(t *testing.T) {
	h, lists := savedHarness(t, 3, false, nil)
	saved, ok := lists.Load(h.m.cur.list.sess.Host, "assigned")
	if !ok || len(saved.Issues) != 3 || !saved.SavedAt.Equal(testNow) {
		t.Errorf("the first load should be saved: %v %+v", ok, saved)
	}
}

func TestFilterChangeShowsThatFiltersSavedList(t *testing.T) {
	h, _ := savedHarness(t, 0, true, map[string][]mantis.Issue{"assigned": chunk(42, 41), "monitored": chunk(77, 77)})
	h.keys("f")
	h.m.modal.(*picker).cursor = 3 // monitored
	h.keys("enter")
	l := h.m.cur.list
	if got := ids(l.issues); l.filter != "monitored" || len(got) != 1 || got[0] != 77 {
		t.Errorf("filter %q shows %v, want the saved monitored list", l.filter, got)
	}
}

func TestSavedListIsNotNews(t *testing.T) {
	h, _ := savedHarness(t, 3, false, map[string][]mantis.Issue{"assigned": chunk(1, 1)})
	if strings.Contains(lastLine(h.view()), "new") {
		t.Errorf("issues missing from the saved list are not new: %q", lastLine(h.view()))
	}
}

func TestSavedAge(t *testing.T) {
	for _, c := range []struct {
		at   time.Time
		want string
	}{
		{time.Time{}, "saved"},
		{testNow.Add(-10 * time.Second), "saved just now"},
		{testNow.Add(-3 * time.Hour), "saved 3h ago"},
	} {
		if got := savedAge(c.at, testNow); got != c.want {
			t.Errorf("savedAge(%v) = %q, want %q", c.at, got, c.want)
		}
	}
}
