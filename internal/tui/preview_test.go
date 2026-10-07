package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

// previewHarness is a list of n issues (with descriptions) on a terminal
// wide enough for the preview, with the debounce removed.
func previewHarness(t *testing.T, n int, tweak func(*config.Config)) *harness {
	t.Helper()
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(host string, f *mantistest.Fake) {
		seed(n)(host, f)
		for id, is := range f.Issues {
			is.Description = fmt.Sprint("Body of issue ", id)
			f.Issues[id] = is
		}
	}, tweak)
	h.m.previewDelay = 0
	h.send(winSize(160, 40))
	return h
}

func TestPreviewShowsIssueUnderCursorWhenWide(t *testing.T) {
	h := previewHarness(t, 5, nil)
	out := h.view()
	if !strings.Contains(out, "│") || !strings.Contains(out, "Body of issue 5") {
		t.Fatalf("wide terminal should show the preview of #5:\n%s", out)
	}
	if got := h.fakes["alpha"].Calls("GetIssue"); got != 1 {
		t.Errorf("GetIssue calls = %d, want 1", got)
	}
	h.keys("j")
	if !strings.Contains(h.view(), "Body of issue 4") {
		t.Errorf("moving the cursor should preview #4:\n%s", h.view())
	}
	for i, line := range strings.Split(h.view(), "\n") {
		if w := ansi.StringWidth(line); w > 160 {
			t.Errorf("line %d is %d cells wide", i, w)
		}
	}
}

func TestPreviewNeedsAWideTerminal(t *testing.T) {
	h := listHarness(t, 5, nil)
	h.m.previewDelay = 0
	h.keys("j")
	if strings.Contains(h.view(), "│") {
		t.Errorf("120 columns should show the full-width list:\n%s", h.view())
	}
	if got := h.fakes["alpha"].Calls("GetIssue"); got != 0 {
		t.Errorf("no preview means no fetch, got %d GetIssue calls", got)
	}
	h.keys("P", "P") // toggling off and on again on a narrow terminal
	if !strings.Contains(h.view(), "at least 140 columns") {
		t.Errorf("turning the preview on when narrow should explain why it is hidden:\n%s", lastLine(h.view()))
	}
}

func TestPreviewCachesUntilTheRowIsNewer(t *testing.T) {
	h := previewHarness(t, 5, nil)
	h.keys("j", "k")
	f := h.fakes["alpha"]
	if got := f.Calls("GetIssue"); got != 2 {
		t.Fatalf("returning to #5 should use the cache: %d GetIssue calls, want 2", got)
	}

	is := f.Issues[5]
	is.UpdatedAt = is.UpdatedAt.Add(time.Hour)
	is.Description = "Edited body"
	f.Issues[5] = is
	h.keys("R")
	if got := f.Calls("GetIssue"); got != 3 {
		t.Errorf("a newer updated_at should refetch: %d GetIssue calls, want 3", got)
	}
	if !strings.Contains(h.view(), "Edited body") {
		t.Errorf("preview should show the new body:\n%s", h.view())
	}
}

func TestPreviewDebounceIgnoresStaleTicks(t *testing.T) {
	h := previewHarness(t, 5, nil)
	h.m.previewDelay = time.Hour // ticks never fire on their own; the test sends them
	l := h.m.cur.list
	h.keys("j")
	stale := previewTickMsg{host: "alpha", id: 4, gen: l.pv.gen}
	h.keys("j")
	h.send(stale)
	f := h.fakes["alpha"]
	if got := f.Calls("GetIssue"); got != 1 {
		t.Fatalf("a tick for a row the cursor left must not fetch: %d calls", got)
	}
	h.send(previewTickMsg{host: "alpha", id: 3, gen: l.pv.gen})
	if got := f.Calls("GetIssue"); got != 2 || !strings.Contains(h.view(), "Body of issue 3") {
		t.Errorf("the current tick should fetch #3: %d calls\n%s", got, h.view())
	}
}

func TestPreviewErrorIsShownInThePane(t *testing.T) {
	h := previewHarness(t, 3, nil)
	f := h.fakes["alpha"]
	f.Errs = map[string]error{"GetIssue": &mantis.APIError{Status: 500, Message: "boom"}}
	h.keys("j")
	if !strings.Contains(h.view(), "Could not load") {
		t.Errorf("a failed preview should say so:\n%s", h.view())
	}
}

func TestTogglePreview(t *testing.T) {
	h := previewHarness(t, 3, nil)
	h.keys("P")
	if strings.Contains(h.view(), "│") {
		t.Errorf("P should hide the preview:\n%s", h.view())
	}
	h.keys("P")
	if !strings.Contains(h.view(), "Body of issue 3") {
		t.Errorf("P again should show it:\n%s", h.view())
	}

	off := previewHarness(t, 3, func(c *config.Config) { c.List.Preview = false })
	if strings.Contains(off.view(), "│") || off.fakes["alpha"].Calls("GetIssue") != 0 {
		t.Errorf("list.preview = false should start hidden:\n%s", off.view())
	}
}

func TestPreviewScrollKeys(t *testing.T) {
	h := previewHarness(t, 1, nil)
	f := h.fakes["alpha"]
	is := f.Issues[1]
	is.UpdatedAt = is.UpdatedAt.Add(time.Minute)
	is.Description = strings.Repeat("long line\n", 100)
	f.Issues[1] = is
	h.keys("R")
	h.keys("J")
	if h.m.cur.list.pv.vp.YOffset() == 0 {
		t.Fatal("ctrl+d should scroll the preview")
	}
	h.keys("K")
	if h.m.cur.list.pv.vp.YOffset() != 0 {
		t.Error("ctrl+u should scroll back")
	}
}

func TestPreviewGolden(t *testing.T) {
	h := previewHarness(t, 4, nil)
	golden(t, "list_preview", h.view())
}

func TestVimStyleOpenAndBack(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys("l")
	if h.m.cur.screen != screenIssue || h.m.cur.issue.id != 3 {
		t.Fatal("l should open the issue under the cursor")
	}
	h.keys("h")
	if h.m.cur.screen != screenList {
		t.Error("h should go back to the list")
	}
}

func TestNoteFromListRefreshesPreview(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	h := previewHarness(t, 2, nil)
	stubEditor(h, "a note from the list\n")
	h.keys("r")
	h.keys("enter") // not private; the fake server has no time tracking
	if !strings.Contains(h.view(), "a note from the list") {
		t.Errorf("the preview should show the new note:\n%s", h.view())
	}
}
