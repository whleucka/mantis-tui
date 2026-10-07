package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// Notifier announces new issues outside the TUI. Mode is "herdr",
// "terminal" or "off" (the CLI resolves "auto").
type Notifier struct {
	Mode  string
	Herdr func(ctx context.Context, title, body string) error
}

const notifyTimeout = 5 * time.Second

type (
	// newIssuesMsg reports issues that appeared in a host's list.
	newIssuesMsg struct {
		host   string
		issues []mantis.Issue
	}
	notifyFailedMsg struct{ err error }
)

// detectNew returns the issues in all that the list has not shown before.
// The first complete load (and the first after a filter change) only
// records what is there. Issues the user reported are never new.
func (l *listModel) detectNew(all []mantis.Issue) []mantis.Issue {
	first := !l.baselined
	l.baselined = true
	var news []mantis.Issue
	for _, is := range all {
		if l.known[is.ID] {
			continue
		}
		l.known[is.ID] = true
		if !first && (l.meID == 0 || is.Reporter.ID != l.meID) {
			news = append(news, is)
		}
	}
	return news
}

// announce reports new issues found by a completed load.
func (l *listModel) announce(all []mantis.Issue) tea.Cmd {
	news := l.detectNew(all)
	if len(news) == 0 {
		return nil
	}
	msg := newIssuesMsg{host: l.host(), issues: news}
	return func() tea.Msg { return msg }
}

func (m *Model) onNewIssues(msg newIssuesMsg) tea.Cmd {
	var parts []string
	for i, is := range msg.issues {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more", len(msg.issues)-3))
			break
		}
		parts = append(parts, fmt.Sprintf("#%d %s", is.ID, is.Summary))
	}
	where := ""
	if !m.isCurrent(msg.host) {
		where = " on " + msg.host
	}
	m.status.info(clean(fmt.Sprintf("%d new%s: %s", len(msg.issues), where, strings.Join(parts, ", "))))
	return m.notify(fmt.Sprintf("mantis-tui: %d new on %s", len(msg.issues), msg.host), strings.Join(parts, " · "))
}

// notify sends a notification the configured way. Issue summaries are
// untrusted, so control characters never reach herdr or the terminal.
func (m *Model) notify(title, body string) tea.Cmd {
	title, body = clean(title), clean(body)
	switch m.opts.Notify.Mode {
	case "herdr":
		send := m.opts.Notify.Herdr
		if send == nil {
			return nil
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
			defer cancel()
			if err := send(ctx, title, body); err != nil {
				return notifyFailedMsg{err}
			}
			return nil
		}
	case "terminal":
		return tea.Raw(osc9(title + ": " + body))
	}
	return nil
}

// osc9 is the desktop-notification escape sequence most modern terminals
// understand; others ignore it.
func osc9(text string) string { return "\x1b]9;" + clean(text) + "\x07" }

// clean removes control characters (C0, DEL, C1), which could otherwise
// smuggle escape sequences into the terminal.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// unreadTotal counts unread issues across every host, for the window title.
func (m *Model) unreadTotal() int {
	n := 0
	for _, hv := range m.hosts {
		n += hv.list.unreadCount()
	}
	return n
}
