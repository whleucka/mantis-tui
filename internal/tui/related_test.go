package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func TestMentions(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []int
	}{
		{"see #12 and #13.", []int{12, 13}},
		{"#7 at the start, (#8) in brackets\n#9 on a new line", []int{7, 8, 9}},
		{"&#123; is an entity", nil},
		{"abc#12 is glued to a word", nil},
		{"#12abc is glued to a word", nil},
		{"ü#5 counts as a letter", nil},
		{"##5 is a heading", nil},
		{"#12#13", []int{12}},
		{"#0 is no issue", nil},
	} {
		if got := mentions(tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("mentions(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestRelatedTargets(t *testing.T) {
	rel := mantis.Relationship{Type: mantis.EnumValue{Name: "related-to", Label: "related to"}}
	rel.Issue.ID, rel.Issue.Summary, rel.Issue.Status = 1, "Login broken", mantis.EnumValue{Name: "new"}
	is := &mantis.Issue{
		ID:            3,
		Relationships: []mantis.Relationship{rel},
		Description:   "Like #1 and #3, see #2.",
		Notes:         []mantis.Note{{ID: 45, Text: "Also #9 and #2.", Reporter: mantis.User{Name: "jane"}}},
	}
	got := relatedTargets(is)
	want := []relatedTarget{
		{1, "related to #1 Login broken", "new"},
		{2, "#2", "mentioned in description"},
		{9, "#9", "mentioned in note 45 by jane"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("targets =\n%+v\nwant\n%+v", got, want)
	}
}

func relatedHarness(t *testing.T, desc3 string) *harness {
	t.Helper()
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(3)(name, f)
		is := f.Issues[3]
		is.Description = desc3
		f.Issues[3] = is
	}, nil)
}

func (h *harness) shownIssue() int {
	h.t.Helper()
	if h.m.cur.screen != screenIssue || h.m.cur.issue == nil {
		return 0
	}
	return h.m.cur.issue.id
}

func TestGoRelatedOpensTheOnlyTargetAndBackReturns(t *testing.T) {
	h := relatedHarness(t, "Same cause as #2.")
	h.keys("enter", "g", "r")
	if got := h.shownIssue(); got != 2 {
		t.Fatalf("shown issue = %d, want 2", got)
	}
	if !strings.Contains(h.view(), "Issue number 2 summary") {
		t.Errorf("view:\n%s", h.view())
	}
	h.keys("h")
	if got := h.shownIssue(); got != 3 {
		t.Fatalf("back should return to #3, shown %d", got)
	}
	h.keys("h")
	if h.m.cur.screen != screenList {
		t.Fatal("second back should reach the list")
	}
}

func TestGoRelatedPicker(t *testing.T) {
	h := relatedHarness(t, "See #2 and #1.")
	h.keys("enter", "g", "r")
	if v := h.view(); !strings.Contains(v, "Go to which issue from #3?") || !strings.Contains(v, "mentioned in description") {
		t.Fatalf("picker missing:\n%s", v)
	}
	h.keys("down", "enter")
	if got := h.shownIssue(); got != 1 {
		t.Fatalf("shown issue = %d, want 1", got)
	}
	h.keys("esc")
	if got := h.shownIssue(); got != 3 {
		t.Fatalf("back should return to #3, shown %d", got)
	}
}

func TestStepIssueStartsAFreshTrail(t *testing.T) {
	h := relatedHarness(t, "Same cause as #2.")
	h.keys("enter", "g", "r", "]") // #3 → #2, then the next in the list, #1
	if got := h.shownIssue(); got != 1 {
		t.Fatalf("shown issue = %d, want 1", got)
	}
	h.keys("h")
	if h.m.cur.screen != screenList {
		t.Fatal("back from an issue opened with ] should reach the list")
	}
}

func TestGoRelatedWithNoTargets(t *testing.T) {
	h := relatedHarness(t, "Nothing related.")
	h.keys("enter", "g", "r")
	if h.m.modal != nil || h.shownIssue() != 3 {
		t.Fatal("no picker and no jump without targets")
	}
	if !strings.Contains(lastLine(h.view()), "#3 has no related or mentioned issues") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}
