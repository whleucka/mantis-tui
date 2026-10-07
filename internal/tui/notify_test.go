package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

type sent struct{ title, body string }

// recordNotifications switches the model to a herdr notifier that records.
func recordNotifications(h *harness) func() []sent {
	var mu sync.Mutex
	var got []sent
	h.m.opts.Notify = Notifier{Mode: "herdr", Herdr: func(_ context.Context, title, body string) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, sent{title, body})
		return nil
	}}
	return func() []sent {
		mu.Lock()
		defer mu.Unlock()
		return append([]sent(nil), got...)
	}
}

func addIssue(h *harness, host string, id int, summary string, reporter int) {
	h.fakes[host].Issues[id] = mantis.Issue{ID: id, Summary: summary, Reporter: mantis.User{ID: reporter}, UpdatedAt: testNow}
}

func TestNewIssueNotifiesOnceAfterTheFirstLoad(t *testing.T) {
	h := refreshHarness(t, 3)
	notes := recordNotifications(h)
	if len(notes()) != 0 {
		t.Fatal("the first load must not notify")
	}
	addIssue(h, "alpha", 9, "Login broken", 7)
	h.listTick()
	got := notes()
	if len(got) != 1 || got[0].title != "mantis-tui: 1 new on alpha" || got[0].body != "#9 Login broken" {
		t.Fatalf("notifications = %+v", got)
	}
	if !strings.Contains(lastLine(h.view()), "1 new: #9 Login broken") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
	h.listTick()
	if len(notes()) != 1 {
		t.Error("an issue is only new once")
	}
}

func TestNotificationListsThreeAndCountsTheRest(t *testing.T) {
	h := refreshHarness(t, 1)
	notes := recordNotifications(h)
	for id := 10; id <= 14; id++ {
		addIssue(h, "alpha", id, "x", 7)
	}
	h.keys("R") // a manual refresh finds new issues too
	got := notes()
	if len(got) != 1 || got[0].title != "mantis-tui: 5 new on alpha" || !strings.HasSuffix(got[0].body, "· and 2 more") {
		t.Errorf("notifications = %+v", got)
	}
}

func TestOwnIssuesAreNotNews(t *testing.T) {
	h := refreshHarness(t, 2)
	notes := recordNotifications(h)
	addIssue(h, "alpha", 9, "mine", 2) // the harness user is id 2
	h.listTick()
	if len(notes()) != 0 {
		t.Errorf("issues you reported are not news: %+v", notes())
	}
}

func TestCreatedIssueIsNotNews(t *testing.T) {
	h := createHarness(t)
	notes := recordNotifications(h)
	stubEditor(h, "body\n")
	h.keys("C")
	h.field("category")
	h.keys("enter", "b", "a", "c", "enter")
	h.field("summary")
	h.typeText("Made here")
	h.field("description")
	h.keys("e", "alt+enter")
	if h.m.cur.screen != screenIssue {
		t.Fatalf("precondition: the issue was created:\n%s", h.view())
	}
	h.keys("q", "R")
	if len(notes()) != 0 {
		t.Errorf("an issue you created is not news: %+v", notes())
	}
}

func TestFilterChangeIsNotNews(t *testing.T) {
	h := refreshHarness(t, 3)
	notes := recordNotifications(h)
	addIssue(h, "alpha", 9, "appears with the new filter", 7)
	h.keys("f", "a", "s", "s", "enter")
	if len(notes()) != 0 {
		t.Errorf("a new filter's first load is not news: %+v", notes())
	}
}

func TestBackgroundHostIsWatched(t *testing.T) {
	h := refreshHarness(t, 2)
	notes := recordNotifications(h)
	beta := h.m.hosts["beta"]
	if beta == nil || len(h.fakes["beta"].ListCalls) == 0 {
		t.Fatal("other hosts should load in the background")
	}
	if h.m.cur.sess.Host.Name != "alpha" || h.m.loading != 0 {
		t.Fatalf("background loads must not switch hosts or spin (loading %d)", h.m.loading)
	}
	h.keys("enter") // on alpha's issue view: lists still refresh
	addIssue(h, "beta", 50, "Over there", 7)
	h.send(refreshTickMsg{host: "beta", target: "list", gen: beta.list.tickGen})
	got := notes()
	if len(got) != 1 || got[0].title != "mantis-tui: 1 new on beta" {
		t.Fatalf("notifications = %+v", got)
	}
	if !strings.Contains(lastLine(h.view()), "1 new on beta: #50 Over there") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}

func TestNoWatchingWithoutAutoRefresh(t *testing.T) {
	h := listHarness(t, 1, nil) // harness default: auto-refresh off
	if _, ok := h.m.hosts["beta"]; ok {
		t.Error("with auto-refresh off, other hosts are not loaded")
	}
}

func TestListRefreshesOutsideTheListScreen(t *testing.T) {
	h := refreshHarness(t, 2)
	h.keys("enter")
	before := len(h.fakes["alpha"].ListCalls)
	h.listTick()
	if len(h.fakes["alpha"].ListCalls) != before+1 {
		t.Error("the list should refresh while the issue view is open")
	}
}

func TestTerminalAndOffModes(t *testing.T) {
	h := refreshHarness(t, 1)
	h.m.opts.Notify = Notifier{Mode: "terminal"}
	msg := h.m.notify("mantis-tui: 1 new on alpha", "#9 evil\x1b]0;pwned\x07 \u009bsummary")()
	raw, ok := msg.(tea.RawMsg)
	if !ok {
		t.Fatalf("terminal mode should write a raw sequence, got %T", msg)
	}
	seq := raw.Msg.(string)
	if !strings.HasPrefix(seq, "\x1b]9;mantis-tui: 1 new on alpha: #9 evil]0;pwned summary") || !strings.HasSuffix(seq, "\x07") {
		t.Errorf("OSC 9 sequence = %q", seq)
	}
	if strings.Count(seq, "\x1b") != 1 || strings.Count(seq, "\x07") != 1 || strings.ContainsRune(seq, '\u009b') {
		t.Errorf("control characters from the summary must be removed: %q", seq)
	}

	h.m.opts.Notify = Notifier{Mode: "off"}
	if h.m.notify("t", "b") != nil {
		t.Error("off sends nothing")
	}
	addIssue(h, "alpha", 9, "still shown", 7)
	h.listTick()
	if !strings.Contains(lastLine(h.view()), "1 new: #9 still shown") {
		t.Errorf("off still shows the status message: %q", lastLine(h.view()))
	}
}

func TestHerdrFailureIsReported(t *testing.T) {
	h := refreshHarness(t, 1)
	h.m.opts.Notify = Notifier{Mode: "herdr", Herdr: func(context.Context, string, string) error {
		return context.DeadlineExceeded
	}}
	addIssue(h, "alpha", 9, "x", 7)
	h.listTick()
	if !strings.Contains(lastLine(h.view()), "notification failed") {
		t.Errorf("status = %q", lastLine(h.view()))
	}
}

func TestWindowTitleCountsUnreadAcrossHosts(t *testing.T) {
	h := refreshHarness(t, 2)
	if got := h.m.View().WindowTitle; got != "mantis-tui" {
		t.Errorf("title = %q", got)
	}
	is := h.fakes["beta"].Issues[1]
	is.UpdatedAt = time.Now().Add(time.Hour)
	h.fakes["beta"].Issues[1] = is
	h.send(refreshTickMsg{host: "beta", target: "list", gen: h.m.hosts["beta"].list.tickGen})
	if got := h.m.View().WindowTitle; got != "(1) mantis-tui" {
		t.Errorf("title = %q, want the unread count from beta", got)
	}
}
