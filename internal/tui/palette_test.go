package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

func TestPaletteListsCommandsWithKeys(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys(":")
	if _, ok := h.m.modal.(*palette); !ok {
		t.Fatalf(": should open the palette, got %T", h.m.modal)
	}
	if out := ansi.Strip(h.view()); !strings.Contains(out, "Toggle help") {
		t.Errorf("palette should render its commands:\n%s", out)
	}
	var labels []string
	for _, e := range h.m.modal.(*palette).entries {
		labels = append(labels, e.label)
	}
	all := strings.Join(labels, "\n")
	for _, want := range []string{"Change status", "Filter: assigned", "Filter: all (current)", "Host: beta", "Mark all read"} {
		if !strings.Contains(all, want) {
			t.Errorf("palette missing %q:\n%s", want, all)
		}
	}
	h.keys("s", "t", "a", "t")
	vis := h.m.modal.(*palette).visible()
	if len(vis) == 0 || vis[0].label != "Change status" || vis[0].keys != "s" {
		t.Errorf("\"stat\" should put Change status (s) first, got %+v", vis)
	}
	for _, e := range h.m.modal.(*palette).entries {
		if e.label == "Up" || e.label == "Command palette" {
			t.Errorf("palette should not list %q", e.label)
		}
	}
	h.keys("esc")
	if h.m.modal != nil {
		t.Error("esc should close the palette")
	}
	h.keys("ctrl+p")
	if _, ok := h.m.modal.(*palette); !ok {
		t.Error("ctrl+p should open the palette too")
	}
}

func TestPaletteRunsActionThatOpensAModal(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys(":", "s", "t", "a", "t", "enter")
	p, ok := h.m.modal.(*picker)
	if !ok || !strings.Contains(p.title, "tatus") {
		t.Fatalf("running Change status should leave its picker open, got %T", h.m.modal)
	}
}

func TestPaletteOpensIssueByNumber(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.fakes["alpha"].Issues[123] = mantis.Issue{ID: 123, Summary: "Far away issue"}
	h.keys(":", "#", "1", "2", "3")
	if vis := h.m.modal.(*palette).visible(); vis[0].label != "Open issue #123" {
		t.Fatalf("a number should offer Open issue first, got %q", vis[0].label)
	}
	h.keys("enter")
	if h.m.cur.screen != screenIssue || h.m.cur.issue.id != 123 {
		t.Fatal("enter should open #123")
	}
	if !strings.Contains(h.view(), "Far away issue") {
		t.Errorf("issue #123 should be shown:\n%s", h.view())
	}
}

func TestPaletteFilterAndHostCommands(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys(":", "f", "i", "l", " ", "a", "s", "s", "enter")
	if got := h.fakes["alpha"].LastList().Filter; got != "assigned" {
		t.Errorf("filter command should reload with assigned, got %q", got)
	}
	h.keys(":", "h", "o", "s", "t", ":", " ", "b", "enter")
	if h.m.cur.sess.Host.Name != "beta" {
		t.Errorf("host command should switch to beta, on %q", h.m.cur.sess.Host.Name)
	}
}

func TestPaletteMarkAllRead(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys("ctrl+a", "u", "ctrl+x") // everything unread
	h.keys(":", "m", "a", "r", "k", " ", "a", "l", "l", "enter")
	if strings.Contains(ansi.Strip(h.view()), "•") {
		t.Errorf("Mark all read should clear every unread marker:\n%s", h.view())
	}
}

func TestPaletteInIssueView(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys("enter", ":")
	out := ansi.Strip(h.view())
	if !strings.Contains(out, "Back to list") || strings.Contains(out, "Filter:") {
		t.Errorf("issue view palette should list its own commands only:\n%s", out)
	}
	h.keys("b", "a", "c", "k", "enter")
	if h.m.cur.screen != screenList {
		t.Error("Back to list from the palette should return to the list")
	}
}

func TestPaletteGolden(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys(":", "s", "t")
	golden(t, "palette", h.view())
}
