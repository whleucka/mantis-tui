package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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

func TestLinkAt(t *testing.T) {
	line := styleMuted.Render("see") + " #12, (https://example.test/a?b=1). #3x é#7"
	for _, c := range []struct {
		x    int
		id   int
		url  string
		desc string
	}{
		{4, 12, "", "the #"},
		{6, 12, "", "the last digit"},
		{7, 0, "", "the comma after"},
		{9, 0, "", "the bracket before"},
		{10, 0, "https://example.test/a?b=1", "the scheme"},
		{35, 0, "https://example.test/a?b=1", "the URL's last character"},
		{36, 0, "", "the closing bracket"},
		{39, 0, "", "#3x is a word"},
		{44, 0, "", "glued to a letter"},
		{90, 0, "", "past the end"},
	} {
		id, url := linkAt(line, c.x)
		if id != c.id || url != c.url {
			t.Errorf("%s: linkAt(x=%d) = %d, %q; want %d, %q", c.desc, c.x, id, url, c.id, c.url)
		}
	}
}

// contentRow is the screen row of the first issue view line containing s.
func contentRow(t *testing.T, h *harness, s string) (x, y int) {
	t.Helper()
	for i, l := range strings.Split(h.view(), "\n") {
		if at := strings.Index(ansi.Strip(l), s); at >= 0 {
			return ansi.StringWidth(ansi.Strip(l)[:at]), i
		}
	}
	t.Fatalf("%q not on screen:\n%s", s, h.view())
	return 0, 0
}

func TestClickMentionOpensItAndBackReturns(t *testing.T) {
	h := relatedHarness(t, "Same cause as #2, see https://example.test/log.")
	var opened string
	h.m.opts.OpenURL = func(u string) error { opened = u; return nil }
	h.keys("enter")
	x, y := contentRow(t, h, "https://")
	h.send(click(x+3, y))
	if opened != "https://example.test/log" || h.shownIssue() != 3 {
		t.Fatalf("clicking the URL should open it in the browser, opened %q", opened)
	}
	x, y = contentRow(t, h, "#2")
	h.send(click(x+1, y))
	if got := h.shownIssue(); got != 2 {
		t.Fatalf("clicking #2 should open it, shown %d", got)
	}
	h.keys("h")
	if got := h.shownIssue(); got != 3 {
		t.Errorf("back should return to #3, shown %d", got)
	}
	x, y = contentRow(t, h, "#3")
	h.send(click(x, y))
	if got := h.shownIssue(); got != 3 || len(h.m.cur.issue.trail) != 0 {
		t.Errorf("clicking the issue's own number does nothing, shown %d", got)
	}
}
