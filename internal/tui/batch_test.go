package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func batchHarness(t *testing.T, n int) *harness {
	t.Helper()
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(n)(name, f)
		f.ProjectList = []mantis.Project{
			{ID: 4, Name: "Soprano", Categories: []mantis.Category{{ID: 1, Name: "General"}, {ID: 2, Name: "Backend"}}},
			{ID: 8, Name: "Other", Categories: []mantis.Category{{ID: 9, Name: "Backend"}}},
		}
		f.UsersByProj = map[int][]mantis.User{4: {{ID: 7, Name: "jane", RealName: "Jane Doe"}}}
	}, nil)
}

func selectedIDs(h *harness) []int {
	var out []int
	for _, is := range h.m.cur.list.issues {
		if _, ok := h.m.cur.list.selected[is.ID]; ok {
			out = append(out, is.ID)
		}
	}
	return out
}

func TestToggleSelectionAdvancesCursor(t *testing.T) {
	h := batchHarness(t, 5)
	h.keys("space", "space")
	if got := selectedIDs(h); !equalInts(got, []int{5, 4}) {
		t.Errorf("selected = %v", got)
	}
	if id := h.m.cur.list.currentID(); id != 3 {
		t.Errorf("cursor should advance after selecting, on #%d", id)
	}
	if !strings.Contains(lastLine(h.view()), "2 selected") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	h.keys("k", "space") // unselect #4
	if got := selectedIDs(h); !equalInts(got, []int{5}) {
		t.Errorf("selected = %v", got)
	}
}

func TestSelectAllAndClear(t *testing.T) {
	h := batchHarness(t, 4)
	h.keys("ctrl+a")
	if got := selectedIDs(h); len(got) != 4 {
		t.Errorf("ctrl+a selected %v", got)
	}
	h.keys("esc")
	if got := selectedIDs(h); len(got) != 0 {
		t.Errorf("esc left %v", got)
	}
}

func TestSelectAllRespectsSearch(t *testing.T) {
	h := batchHarness(t, 12)
	h.keys("/")
	h.send(key("1"))
	h.keys("enter", "ctrl+a")
	if got := selectedIDs(h); !equalInts(got, []int{12, 11, 10, 1}) {
		t.Errorf("ctrl+a with search '1' selected %v, want only the shown rows", got)
	}
}

func TestBatchStatusOnSelection(t *testing.T) {
	h := batchHarness(t, 5)
	h.keys("space", "space", "space") // 5,4,3
	h.keys("s")
	if !strings.Contains(h.view(), "for 3 issues") {
		t.Fatalf("picker should name the count:\n%s", h.view())
	}
	h.keys("c", "l", "o", "enter")
	f := h.fakes["alpha"]
	if len(f.Patches) != 3 {
		t.Fatalf("patches = %+v", f.Patches)
	}
	for _, p := range f.Patches {
		if p.Patch.Status == nil || p.Patch.Status.Name != "closed" {
			t.Errorf("patch %+v", p)
		}
	}
	if len(selectedIDs(h)) != 0 {
		t.Error("selection should clear after a fully successful batch")
	}
	if !strings.Contains(lastLine(h.view()), "3 issues") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}

func TestBatchWithoutSelectionUsesCursorRow(t *testing.T) {
	h := batchHarness(t, 5)
	h.keys("j", "p", "h", "i", "enter")
	f := h.fakes["alpha"]
	if len(f.Patches) != 1 || f.Patches[0].ID != 4 || f.Patches[0].Patch.Priority.Name != "high" {
		t.Errorf("patches = %+v", f.Patches)
	}
}

func TestBatchPartialFailureKeepsOnlyFailedSelected(t *testing.T) {
	h := batchHarness(t, 10)
	h.fakes["alpha"].ErrFor = map[string]map[int]error{"UpdateIssue": {7: errors.New("access denied")}}
	h.keys("ctrl+a", "v", "m", "a", "j", "enter") // severity major
	if got := selectedIDs(h); !equalInts(got, []int{7}) {
		t.Errorf("selected after partial failure = %v, want [7]", got)
	}
	status := lastLine(h.view())
	if !strings.Contains(status, "1 of 10 failed") || !strings.Contains(status, "9 ok") {
		t.Errorf("status = %q", status)
	}
}

func TestBatchDeleteConfirmsCount(t *testing.T) {
	h := batchHarness(t, 5)
	h.keys("space", "space", "D")
	if !strings.Contains(h.view(), "Delete 2 issues?") {
		t.Fatalf("confirm should show the count:\n%s", h.view())
	}
	h.keys("y")
	if d := h.fakes["alpha"].Deleted; len(d) != 2 {
		t.Errorf("deleted = %v", d)
	}
	if len(selectedIDs(h)) != 0 || len(h.m.cur.list.selected) != 0 {
		t.Error("deleted issues leave the selection")
	}
	if strings.Contains(h.view(), "Issue number 5 summary") {
		t.Error("deleted rows should be gone")
	}
}

func TestSelectionSurvivesPagingAndRefresh(t *testing.T) {
	h := batchHarness(t, 60)
	h.keys("space") // #60
	h.keys("]")
	if _, ok := h.m.cur.list.selected[60]; !ok {
		t.Fatal("selection lost after paging")
	}
	if !strings.Contains(lastLine(h.view()), "1 selected") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	h.keys("space", "R") // select #10 on page 2, then refresh
	if len(h.m.cur.list.selected) != 2 {
		t.Errorf("selection after refresh = %v", h.m.cur.list.selected)
	}
	h.keys("s", "n", "e", "w", "enter")
	got := map[int]bool{}
	for _, p := range h.fakes["alpha"].Patches {
		got[p.ID] = true
	}
	if !got[60] || !got[10] || len(got) != 2 {
		t.Errorf("batch should include the off-page issue: patched %v", got)
	}
}

func TestBatchCategoryAcrossProjects(t *testing.T) {
	h := batchHarness(t, 3)
	other := h.fakes["alpha"].Issues[2]
	other.Project = mantis.Ref{ID: 8, Name: "Other"}
	h.fakes["alpha"].Issues[2] = other
	h.keys("R", "ctrl+a", "c", "b", "a", "c", "enter")
	for _, p := range h.fakes["alpha"].Patches {
		c := p.Patch.Category
		switch p.ID {
		case 2:
			if c == nil || c.Name != "Backend" || c.ID != 0 {
				t.Errorf("issue in another project should get the category by name only: %+v", c)
			}
		default:
			if c == nil || c.ID != 2 {
				t.Errorf("#%d category = %+v", p.ID, c)
			}
		}
	}
}

func TestBatchProgressInStatusBar(t *testing.T) {
	h := batchHarness(t, 3)
	h.m.batchID = 4
	h.send(batchProgressMsg{host: "alpha", batch: 4, label: "status → closed", done: 2, total: 5})
	if !strings.Contains(lastLine(h.view()), "2/5") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	h.send(batchProgressMsg{host: "alpha", batch: 3, label: "old batch", done: 1, total: 5})
	if strings.Contains(lastLine(h.view()), "old batch") {
		t.Error("progress from a finished batch must be ignored")
	}
}
