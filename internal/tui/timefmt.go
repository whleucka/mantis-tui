package tui

import (
	"fmt"
	"time"
)

const recent = 7 * 24 * time.Hour // newer times show as relative

// ago is a short relative time ("now", "12m ago", "3h ago", "6d ago") for
// times within the last week, and "" for older or zero times. Times a little
// in the future (clock skew) count as "now".
func ago(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < recent:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
	return ""
}

// dateOf is the list's short time: relative within a week, else the date.
func dateOf(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	if a := ago(t, now); a != "" {
		return a
	}
	return t.Local().Format("2006-01-02")
}

// timeOf is the full time, with the relative time after it when recent.
func timeOf(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	s := t.Local().Format("2006-01-02 15:04")
	if a := ago(t, now); a != "" {
		s += " (" + a + ")"
	}
	return s
}

// stamp is the full time alone, for aligned columns such as history.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}
