package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func actionsHarness(t *testing.T) *harness {
	t.Helper()
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(5)(name, f)
		f.ProjectList = []mantis.Project{{ID: 4, Name: "Soprano", Categories: []mantis.Category{{ID: 1, Name: "General"}, {ID: 2, Name: "Backend"}}}}
		f.UsersByProj = map[int][]mantis.User{4: {{ID: 2, Name: "me", RealName: "Me"}, {ID: 7, Name: "jane", RealName: "Jane Doe"}}}
	}, nil)
}

func lastPatch(t *testing.T, f *mantistest.Fake) mantistest.Patch {
	t.Helper()
	if len(f.Patches) == 0 {
		t.Fatal("no PATCH sent")
	}
	return f.Patches[len(f.Patches)-1]
}

func TestEnumActionsPatchTheCurrentIssue(t *testing.T) {
	for _, tt := range []struct {
		key, filter, want string
		field             func(mantis.IssuePatch) *mantis.Ref
	}{
		{"s", "resol", "resolved", func(p mantis.IssuePatch) *mantis.Ref { return p.Status }},
		{"p", "urg", "urgent", func(p mantis.IssuePatch) *mantis.Ref { return p.Priority }},
		{"v", "cra", "crash", func(p mantis.IssuePatch) *mantis.Ref { return p.Severity }},
	} {
		h := actionsHarness(t)
		h.keys("j") // #4
		h.keys(tt.key)
		if h.m.modal == nil {
			t.Fatalf("%s should open a picker", tt.key)
		}
		for _, r := range tt.filter {
			h.send(key(string(r)))
		}
		h.keys("enter")
		p := lastPatch(t, h.fakes["alpha"])
		if p.ID != 4 || tt.field(p.Patch) == nil || tt.field(p.Patch).Name != tt.want {
			t.Errorf("%s: patch = %+v", tt.key, p)
		}
		if !strings.Contains(h.view(), tt.want) {
			t.Errorf("%s: row should show %q after the update", tt.key, tt.want)
		}
	}
}

func TestStatusPickerMarksCurrentValue(t *testing.T) {
	h := actionsHarness(t)
	h.keys("s")
	if !strings.Contains(h.view(), "● assigned") {
		t.Errorf("current status should be marked:\n%s", h.view())
	}
}

func TestCategoryAndAssign(t *testing.T) {
	h := actionsHarness(t)
	h.keys("c", "b", "a", "c", "enter")
	if c := lastPatch(t, h.fakes["alpha"]).Patch.Category; c == nil || c.Name != "Backend" {
		t.Errorf("category patch = %+v", c)
	}
	h.keys("a", "j", "a", "n", "enter")
	if hd := lastPatch(t, h.fakes["alpha"]).Patch.Handler; hd == nil || hd.ID != 7 {
		t.Errorf("handler patch = %+v", hd)
	}
}

func TestSummaryPrompt(t *testing.T) {
	h := actionsHarness(t)
	h.keys("e", "enter")
	if !strings.Contains(h.view(), "Issue number 5 summary") {
		t.Error("summary prompt should be prefilled")
	}
	h.keys("ctrl+u") // clear the input
	for _, r := range "Better title" {
		h.send(key(string(r)))
	}
	h.keys("enter")
	if s := lastPatch(t, h.fakes["alpha"]).Patch.Summary; s == nil || *s != "Better title" {
		t.Errorf("summary patch = %v", s)
	}
}

func TestSummaryPromptRejectsEmpty(t *testing.T) {
	h := actionsHarness(t)
	h.keys("e", "enter", "ctrl+u", "enter")
	if h.m.modal == nil {
		t.Error("empty summary should keep the prompt open")
	}
	if len(h.fakes["alpha"].Patches) != 0 {
		t.Error("nothing should be sent")
	}
	h.keys("esc")
	if h.m.modal != nil {
		t.Error("esc closes the prompt")
	}
}

func TestMonitorToggle(t *testing.T) {
	h := actionsHarness(t)
	h.keys("m") // #5, not monitored
	if f := h.fakes["alpha"]; len(f.Monitored) != 1 || f.Monitored[0] != 5 {
		t.Errorf("monitored = %v", f.Monitored)
	}
	h.keys("j", "j", "m") // #3 is monitored by me (user 2) → unmonitor
	p := lastPatch(t, h.fakes["alpha"])
	if p.ID != 3 || p.Patch.Monitors == nil || len(*p.Patch.Monitors) != 0 {
		t.Errorf("unmonitor patch = %+v", p)
	}
}

func TestOpenInBrowser(t *testing.T) {
	h := actionsHarness(t)
	var opened string
	h.m.opts.OpenURL = func(u string) error { opened = u; return nil }
	h.keys("o")
	if opened != "https://alpha.example.test/view.php?id=5" {
		t.Errorf("opened %q", opened)
	}
}

func TestDeleteConfirm(t *testing.T) {
	h := actionsHarness(t)
	h.keys("j", "D") // #4
	if !strings.Contains(h.view(), "Delete #4") {
		t.Fatalf("confirm prompt missing:\n%s", h.view())
	}
	h.keys("n")
	if len(h.fakes["alpha"].Deleted) != 0 {
		t.Fatal("n must not delete")
	}
	h.keys("D", "y")
	if d := h.fakes["alpha"].Deleted; len(d) != 1 || d[0] != 4 {
		t.Errorf("deleted = %v", d)
	}
	if strings.Contains(h.view(), "Issue number 4 summary") {
		t.Error("deleted row should disappear")
	}
	if id := h.m.cur.list.currentID(); id != 3 {
		t.Errorf("cursor should move to the next row, got #%d", id)
	}
}

func TestFailedUpdateLeavesRowAndReports(t *testing.T) {
	h := actionsHarness(t)
	h.fakes["alpha"].ErrFor = map[string]map[int]error{"UpdateIssue": {5: errors.New("access denied")}}
	h.keys("s", "r", "e", "s", "o", "enter")
	if !strings.Contains(lastLine(h.view()), "access denied") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	if strings.Contains(h.view(), "resolved") {
		t.Error("row must not change when the server refuses")
	}
}

func TestActionsFromIssueView(t *testing.T) {
	h := actionsHarness(t)
	h.keys("enter") // #5
	gets := h.fakes["alpha"].Calls("GetIssue")
	h.keys("s", "c", "l", "o", "enter")
	if p := lastPatch(t, h.fakes["alpha"]); p.ID != 5 || p.Patch.Status.Name != "closed" {
		t.Errorf("patch = %+v", p)
	}
	if h.fakes["alpha"].Calls("GetIssue") <= gets {
		t.Error("issue view should reload after an update")
	}
	h.keys("D")
	if h.m.modal != nil {
		t.Error("D is not bound in the issue view")
	}
}

func TestAskRunsTheTemplateInAPane(t *testing.T) {
	h := actionsHarness(t)
	var got string
	var gotRight bool
	h.m.opts.RunInPane = func(_ context.Context, command string, right bool) error {
		got, gotRight = command, right
		return nil
	}
	h.m.opts.Config.Issue.Ask = "ask {id} on {host} at {url}"
	h.keys("A")
	if want := "ask 5 on alpha at https://alpha.example.test/view.php?id=5"; got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
	if !gotRight {
		t.Error("a 120x40 terminal should split to the right")
	}
	if !strings.Contains(h.view(), "asking about #5") {
		t.Errorf("status should confirm:\n%s", h.view())
	}

	h.send(winSize(80, 50))
	h.keys("A")
	if gotRight {
		t.Error("an 80x50 terminal should split below")
	}
}

func TestAskReportsWhyItCannot(t *testing.T) {
	for _, tt := range []struct {
		name, template string
		run            func(context.Context, string, bool) error
		want           string
	}{
		{"outside herdr", "claude", nil, "needs herdr"},
		{"disabled", "", func(context.Context, string, bool) error { return nil }, "issue.ask is empty"},
		{"pane failed", "claude", func(context.Context, string, bool) error { return errors.New("split broke") }, "split broke"},
	} {
		h := actionsHarness(t)
		h.m.opts.RunInPane = tt.run
		h.m.opts.Config.Issue.Ask = tt.template
		h.keys("A")
		if !strings.Contains(h.view(), tt.want) {
			t.Errorf("%s: status should say %q:\n%s", tt.name, tt.want, h.view())
		}
	}
}
