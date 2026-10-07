package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/config"
)

// bump makes issue id newer on the fake server, as if someone else changed it.
func bump(h *harness, id int) {
	f := h.fakes["alpha"]
	is := f.Issues[id]
	is.UpdatedAt = time.Now().Add(time.Hour)
	f.Issues[id] = is
}

// rowOf returns the stripped list line showing issue id's summary.
func rowOf(h *harness, id int) string {
	for _, line := range strings.Split(ansi.Strip(h.view()), "\n") {
		if strings.Contains(line, fmt.Sprintf("Issue number %d summary", id)) {
			return line
		}
	}
	return ""
}

func TestFirstRunHasNothingUnread(t *testing.T) {
	h := listHarness(t, 5, nil)
	if strings.Contains(ansi.Strip(h.view()), "•") || strings.Contains(lastLine(h.view()), "unread") {
		t.Errorf("issues older than the host's baseline must not be unread:\n%s", h.view())
	}
}

func TestUpdatedIssueIsUnreadUntilOpened(t *testing.T) {
	h := listHarness(t, 5, nil)
	bump(h, 3)
	h.keys("R")
	if row := rowOf(h, 3); !strings.Contains(row, "•") {
		t.Fatalf("#3 changed after the baseline and should be unread: %q", row)
	}
	if strings.Contains(rowOf(h, 4), "•") {
		t.Error("#4 did not change and should stay read")
	}
	if !strings.Contains(lastLine(h.view()), "1 unread") {
		t.Errorf("status bar should count unread: %q", lastLine(h.view()))
	}

	h.keys("n") // jumps to #3
	if id := h.m.cur.list.currentID(); id != 3 {
		t.Fatalf("n should move to the unread #3, got #%d", id)
	}
	h.keys("enter", "q")
	if strings.Contains(rowOf(h, 3), "•") {
		t.Errorf("opening #3 should mark it read: %q", rowOf(h, 3))
	}
	h.keys("n")
	if !strings.Contains(lastLine(h.view()), "no unread") {
		t.Errorf("n with nothing unread should say so: %q", lastLine(h.view()))
	}
}

func TestPreviewMarksIssueRead(t *testing.T) {
	h := previewHarness(t, 3, nil)
	bump(h, 2)
	h.keys("R")
	if !strings.Contains(rowOf(h, 2), "•") {
		t.Fatal("precondition: #2 unread")
	}
	h.keys("j") // cursor rests on #2; the preview loads it
	if strings.Contains(rowOf(h, 2), "•") {
		t.Errorf("a loaded preview should mark #2 read: %q", rowOf(h, 2))
	}
}

func TestToggleReadAndMarkUnreadFromIssueView(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys("u")
	if !strings.Contains(rowOf(h, 3), "•") {
		t.Fatalf("u on a read issue should mark it unread: %q", rowOf(h, 3))
	}
	h.keys("u")
	if strings.Contains(rowOf(h, 3), "•") {
		t.Fatalf("u again should mark it read: %q", rowOf(h, 3))
	}

	h.keys("j", "enter", "u")
	if h.m.cur.screen != screenList {
		t.Fatal("u in the issue view should go back to the list")
	}
	if !strings.Contains(rowOf(h, 2), "•") {
		t.Errorf("#2 should be unread after u in its view: %q", rowOf(h, 2))
	}
}

func TestToggleReadAppliesToSelection(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys("ctrl+a", "u")
	for id := 1; id <= 3; id++ {
		if !strings.Contains(rowOf(h, id), "•") {
			t.Errorf("#%d should be unread after u on the selection", id)
		}
	}
	if !strings.Contains(lastLine(h.view()), "3 issues unread") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}

func TestOwnChangesDoNotMarkUnread(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.fakes["alpha"].BumpOnWrite = true
	h.keys("s")
	h.keys("r", "e", "s", "enter") // status → resolved
	if len(h.fakes["alpha"].Patches) != 1 {
		t.Fatal("precondition: one patch")
	}
	h.keys("R")
	if strings.Contains(rowOf(h, 3), "•") {
		t.Errorf("your own status change must not leave #3 unread: %q", rowOf(h, 3))
	}
}

func TestOwnNoteDoesNotMarkUnread(t *testing.T) {
	h := notesHarness(t, false)
	h.fakes["alpha"].BumpOnWrite = true
	stubEditor(h, "mine\n")
	h.keys("r", "enter")
	h.keys("R")
	if strings.Contains(rowOf(h, 3), "•") {
		t.Errorf("your own note must not leave #3 unread: %q", rowOf(h, 3))
	}
}

func TestSeenStateIsSaved(t *testing.T) {
	path := t.TempDir() + "/seen.json"
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], seed(2), nil)
	h.m.seen = config.NewSeen(path)
	h.m.cur.list.seen = h.m.seen
	h.m.seen.Begin("alpha", time.Now().Add(-time.Hour))
	h.keys("u")
	if !config.LoadSeen(path).Unread("alpha", 2, time.Unix(1, 0)) {
		t.Error("u should save the unread mark to the seen file")
	}
}

func TestOwnMonitorDoesNotMarkUnread(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.fakes["alpha"].BumpOnWrite = true
	h.keys("j", "m") // #2 is not monitored yet
	if len(h.fakes["alpha"].Monitored) != 1 {
		t.Fatal("precondition: monitored")
	}
	h.keys("R")
	if strings.Contains(rowOf(h, 2), "•") {
		t.Errorf("monitoring must not leave #2 unread: %q", rowOf(h, 2))
	}
}
