package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

// fixtureIssue loads the scrubbed 2.27.0 issue and adds a private,
// time-tracked note and the optional collections the fixture lacks.
func fixtureIssue(t *testing.T) mantis.Issue {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "mantis", "testdata", "issue.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Issues []mantis.Issue }
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	is := env.Issues[0]
	is.Notes = append(is.Notes, mantis.Note{
		ID: 99, Reporter: mantis.User{ID: 2, Name: "user2", RealName: "User 2"},
		Text: "Private follow-up.\nSecond line.", ViewState: enum("private"),
		TimeTracking: &mantis.TimeTracking{Duration: "00:30"},
		CreatedAt:    time.Date(2026, 9, 5, 9, 15, 0, 0, time.UTC),
	})
	is.Tags = []mantis.Ref{{ID: 1, Name: "ui"}, {ID: 2, Name: "regression"}}
	is.CustomFields = []mantis.CustomFieldValue{{Field: mantis.Ref{Name: "Version"}, Value: "1.4.2"}}
	is.StepsToReproduce = "1. Open a generated playlist\n2. Press delete"
	return is
}

func issueHarness(t *testing.T, is mantis.Issue) *harness {
	t.Helper()
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(_ string, f *mantistest.Fake) {
		f.Issues[is.ID] = is
		f.Issues[is.ID+1] = mantis.Issue{ID: is.ID + 1, Summary: "neighbour"}
	}, nil)
}

func TestOpenIssueShowsDetailsAndNotes(t *testing.T) {
	is := fixtureIssue(t)
	h := issueHarness(t, is)
	h.keys("j") // second row is the fixture issue (ids sort descending)
	h.keys("enter")
	if h.m.cur.screen != screenIssue {
		t.Fatal("enter should open the issue view")
	}
	if h.fakes["alpha"].Calls("GetIssue") != 1 {
		t.Error("issue view should fetch the full issue")
	}
	out := h.view()
	for _, want := range []string{
		"#33", "Sample summary", "closed", "fixed", "Sample description", "Steps to reproduce",
		"ui, regression", "Version", "1.4.2", "file-6.txt", "Notes (4)", "private", "0:30", "Second line.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("issue view missing %q", want)
		}
	}
	h.send(winSize(100, 60))
	golden(t, "issue_view_notes", h.view())
}

func TestIssueHistoryTab(t *testing.T) {
	h := issueHarness(t, fixtureIssue(t))
	h.keys("j", "enter", "tab")
	out := h.view()
	if !strings.Contains(out, "History (") || !strings.Contains(out, "New Issue") || !strings.Contains(out, "old value => new value") {
		t.Errorf("history tab:\n%s", out)
	}
	h.keys("tab")
	if !strings.Contains(h.view(), "Second line.") {
		t.Error("tab should switch back to notes")
	}
}

func TestIssueViewScrolling(t *testing.T) {
	h := issueHarness(t, fixtureIssue(t))
	h.send(winSize(80, 12)) // tiny: content must scroll
	h.keys("j", "enter")
	iv := h.m.cur.issue
	if iv.vp.YOffset() != 0 {
		t.Fatal("starts at the top")
	}
	h.keys("j", "j")
	if iv.vp.YOffset() != 2 {
		t.Errorf("jj → offset %d", iv.vp.YOffset())
	}
	h.keys("ctrl+d")
	if iv.vp.YOffset() <= 2 {
		t.Error("ctrl+d should scroll half a page")
	}
	h.keys("G")
	if !iv.vp.AtBottom() {
		t.Error("G should go to the bottom")
	}
	h.keys("g", "g")
	if iv.vp.YOffset() != 0 {
		t.Error("gg should go to the top")
	}
}

func TestIssueViewBackKeepsListCursor(t *testing.T) {
	h := issueHarness(t, fixtureIssue(t))
	h.keys("j", "enter", "q")
	if h.m.cur.screen != screenList {
		t.Fatal("q should go back to the list")
	}
	if h.m.cur.list.currentID() != 33 {
		t.Errorf("cursor on #%d, want #33", h.m.cur.list.currentID())
	}
	h.keys("enter", "esc")
	if h.m.cur.screen != screenList {
		t.Error("esc should also go back")
	}
}

func TestIssueViewMinimalIssue(t *testing.T) {
	bare := mantis.Issue{ID: 7, Summary: "bare"} // no handler, notes, tags, dates
	h := issueHarness(t, bare)
	h.keys("j", "enter", "tab", "G")
	out := h.view()
	if !strings.Contains(out, "#7") || !strings.Contains(out, "Notes (0)") && !strings.Contains(out, "History (0)") {
		t.Errorf("bare issue view:\n%s", out)
	}
	if strings.Contains(out, "Handler") {
		t.Error("empty fields should be omitted")
	}
}

func TestIssueLoadErrorShowsInStatusAndView(t *testing.T) {
	h := issueHarness(t, fixtureIssue(t))
	h.fakes["alpha"].Errs = map[string]error{"GetIssue": errBoomf("issue gone")}
	h.keys("j", "enter")
	if !strings.Contains(lastLine(h.view()), "issue gone") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	h.keys("q")
	if h.m.cur.screen != screenList {
		t.Error("q should still go back after an error")
	}
}

func TestIssueRefresh(t *testing.T) {
	is := fixtureIssue(t)
	h := issueHarness(t, is)
	h.keys("j", "enter")
	is.Summary = "Edited elsewhere"
	h.fakes["alpha"].Issues[33] = is
	h.keys("r")
	if !strings.Contains(h.view(), "Edited elsewhere") {
		t.Error("r should reload the issue")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func errBoomf(s string) error { return errString(s) }
