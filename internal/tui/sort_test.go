package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

var day0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// sortHarness serves four issues whose fields disagree on the order.
func sortHarness(t *testing.T, sort string) *harness {
	t.Helper()
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(_ string, f *mantistest.Fake) {
		add := func(id int, summary string, prio, sev, status int, updatedDay int, project string) {
			f.Issues[id] = mantis.Issue{
				ID: id, Summary: summary,
				Priority:  mantis.EnumValue{ID: prio, Name: "p"},
				Severity:  mantis.EnumValue{ID: sev, Name: "s"},
				Status:    mantis.EnumValue{ID: status, Name: "st"},
				UpdatedAt: day0.AddDate(0, 0, updatedDay),
				Project:   mantis.Ref{ID: 1, Name: project},
			}
		}
		add(1, "delta", 60, 10, 80, 3, "Beta")
		add(2, "alpha", 30, 80, 10, 1, "Alpha")
		add(3, "Charlie", 30, 50, 50, 4, "Beta")
		add(4, "bravo", 10, 50, 20, 2, "Alpha")
	}, func(c *config.Config) { c.List.Sort = sort })
}

func order(h *harness) []int {
	var out []int
	for _, r := range h.m.cur.list.rows() {
		if r.idx >= 0 {
			out = append(out, h.m.cur.list.issues[r.idx].ID)
		}
	}
	return out
}

func sameOrder(a, b []int) bool {
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

func TestSortFieldsNaturalOrder(t *testing.T) {
	for _, tt := range []struct {
		field string
		want  []int
	}{
		{"updated", []int{3, 1, 4, 2}},  // newest first
		{"priority", []int{1, 3, 2, 4}}, // highest first; 3 and 2 tie → newer #3 first
		{"severity", []int{2, 3, 4, 1}}, // worst first; 3 and 4 tie → newer #3 first
		{"status", []int{2, 4, 3, 1}},   // workflow order
		{"id", []int{4, 3, 2, 1}},
		{"summary", []int{2, 4, 3, 1}}, // case-insensitive A–Z
	} {
		h := sortHarness(t, tt.field)
		if got := order(h); !sameOrder(got, tt.want) {
			t.Errorf("sort %s: got %v, want %v", tt.field, got, tt.want)
		}
	}
}

func TestSortPickerAndReverse(t *testing.T) {
	h := sortHarness(t, "updated")
	h.keys("S")
	p, ok := h.m.modal.(*picker)
	if !ok || p.title != "Sort by" {
		t.Fatalf("S should open the sort picker, got %T", h.m.modal)
	}
	h.keys("p", "r", "i", "enter")
	if got := order(h); !sameOrder(got, []int{1, 3, 2, 4}) {
		t.Fatalf("priority: %v", got)
	}
	if !strings.Contains(ansi.Strip(h.view()), "↓") || !strings.Contains(lastLine(h.view()), "by priority ↓") {
		t.Errorf("header and status should show the sort:\n%s", h.view())
	}
	h.keys("S", "p", "r", "i", "enter") // again → reversed; ties keep newest first
	if got := order(h); !sameOrder(got, []int{4, 3, 2, 1}) {
		t.Errorf("reversed priority: %v", got)
	}
	if !strings.Contains(lastLine(h.view()), "by priority ↑") {
		t.Errorf("status should show the reversed arrow: %q", lastLine(h.view()))
	}
}

func TestSortKeepsCursorAndGroups(t *testing.T) {
	h := sortHarness(t, "id")
	h.keys("j") // #3
	h.keys(":", "s", "o", "r", "t", ":", " ", "s", "u", "m", "enter")
	if id := h.m.cur.list.currentID(); id != 3 {
		t.Errorf("sorting should keep the cursor on #3, got #%d", id)
	}
	h.keys("ctrl+g")
	if got := order(h); !sameOrder(got, []int{2, 4, 3, 1}) { // Alpha: alpha, bravo; Beta: Charlie, delta
		t.Errorf("grouped by project, sorted within: %v", got)
	}
}

func TestEditedRowMovesToItsSortPlace(t *testing.T) {
	h := sortHarness(t, "priority")
	h.keys("G")                         // #4, lowest priority
	h.keys("p", "i", "m", "m", "enter") // immediate: ties with #1, which is newer
	if p := lastPatch(t, h.fakes["alpha"]); p.ID != 4 {
		t.Fatalf("patch = %+v", p)
	}
	if got := order(h); !sameOrder(got, []int{1, 4, 3, 2}) || h.m.cur.list.currentID() != 4 {
		t.Errorf("#4 should move up to second with the cursor on it: %v, cursor #%d", got, h.m.cur.list.currentID())
	}
}
