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

func TestAgo(t *testing.T) {
	now := testNow
	for _, tt := range []struct {
		before time.Duration
		want   string
	}{
		{-time.Minute, "now"}, // a little in the future: clock skew
		{0, "now"},
		{59 * time.Second, "now"},
		{time.Minute, "1m ago"},
		{59 * time.Minute, "59m ago"},
		{time.Hour, "1h ago"},
		{23*time.Hour + 59*time.Minute, "23h ago"},
		{24 * time.Hour, "1d ago"},
		{7*24*time.Hour - time.Second, "6d ago"},
		{7 * 24 * time.Hour, ""}, // a week or more: no relative time
	} {
		if got := ago(now.Add(-tt.before), now); got != tt.want {
			t.Errorf("ago(-%v) = %q, want %q", tt.before, got, tt.want)
		}
	}
	if ago(time.Time{}, now) != "" || dateOf(time.Time{}, now) != "" || timeOf(time.Time{}, now) != "" {
		t.Error("zero times render as empty")
	}
	old := now.AddDate(0, 0, -30)
	if got := dateOf(old, now); got != "2026-09-07" {
		t.Errorf("dateOf(30d ago) = %q, want the date", got)
	}
	if got := timeOf(now.Add(-3*time.Hour), now); got != "2026-10-07 06:00 (3h ago)" {
		t.Errorf("timeOf(3h ago) = %q", got)
	}
	if got := timeOf(old, now); got != "2026-09-07 09:00" {
		t.Errorf("timeOf(30d ago) = %q", got)
	}
}

func TestListShowsRelativeTimes(t *testing.T) {
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(2)(name, f)
		is := f.Issues[2]
		is.UpdatedAt = testNow.Add(-3 * time.Hour)
		f.Issues[2] = is
	}, nil)
	out := ansi.Strip(h.view())
	if !strings.Contains(out, "3h ago") || !strings.Contains(out, "2026-09-02") {
		t.Errorf("recent rows show a relative time, older rows the date:\n%s", out)
	}
	h.keys("enter")
	if !strings.Contains(ansi.Strip(h.view()), "(3h ago)") {
		t.Errorf("the issue view should add the relative time:\n%s", h.view())
	}
}

func TestIssueViewStatusUsesServerColour(t *testing.T) {
	h := listHarness(t, 1, nil)
	h.keys("enter")
	if !strings.Contains(h.view(), "38;2;194;223;255") { // "assigned" → #c2dfff
		t.Errorf("status should use the server's colour:\n%q", h.view())
	}
}

func TestStatusColourFallsBackToTheIssue(t *testing.T) {
	is := mantis.Issue{Status: mantis.EnumValue{Name: "odd", Color: "#123456"}}
	if got := statusColor(is, map[string]string{"new": "#fff"}); got != "#123456" {
		t.Errorf("statusColor = %q, want the issue's own colour", got)
	}
}

func TestHandlerColumnNeedsAWideList(t *testing.T) {
	hosts := testHosts()
	h := newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(1)(name, f)
		is := f.Issues[1]
		is.Handler = &mantis.User{ID: 7, Name: "jane"}
		f.Issues[1] = is
	}, func(c *config.Config) { c.List.Preview = false })
	if strings.Contains(h.view(), "HANDLER") {
		t.Error("120 columns should not show HANDLER")
	}
	h.send(winSize(160, 40))
	if out := h.view(); !strings.Contains(out, "HANDLER") || !strings.Contains(out, "jane") {
		t.Errorf("160 columns without the preview should show HANDLER:\n%s", out)
	}
}
