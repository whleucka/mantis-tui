package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

type seenSaveFailedMsg struct{ err error }

// saveSeen writes the read state in the background.
func (m *Model) saveSeen() tea.Cmd {
	s := m.seen
	return func() tea.Msg {
		if err := s.Save(); err != nil {
			return seenSaveFailedMsg{err}
		}
		return nil
	}
}

// markSeen records that the user has seen is as it is now.
func (m *Model) markSeen(host string, is mantis.Issue) tea.Cmd {
	if !m.seen.Mark(host, is.ID, is.UpdatedAt) {
		return nil
	}
	return m.saveSeen()
}

// seenFetchedMsg carries an issue re-read after the user's own write.
type seenFetchedMsg struct {
	host  string
	issue mantis.Issue
}

// markSeenFromServer re-reads issue id and marks it seen, for writes whose
// response does not carry the issue's new updated_at (adding a note).
// A failure only means the issue may show as unread, so it is not reported.
func (m *Model) markSeenFromServer(host string, id int) tea.Cmd {
	api := m.cur.sess.API
	if hv := m.hosts[host]; hv != nil {
		api = hv.sess.API
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		res, err := api.GetIssue(ctx, id)
		if err != nil {
			return nil
		}
		return seenFetchedMsg{host: host, issue: res.Issue}
	}
}

// markPatchedSeen counts the user's own changes as seen, so editing an
// issue never leaves it unread.
func (m *Model) markPatchedSeen(host string, updated map[int]*mantis.Issue) tea.Cmd {
	changed := false
	for _, is := range updated {
		if is != nil && is.ID != 0 && m.seen.Mark(host, is.ID, is.UpdatedAt) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return m.saveSeen()
}

func (l *listModel) unread(is mantis.Issue) bool {
	return l.seen.Unread(l.host(), is.ID, is.UpdatedAt)
}

func (l *listModel) unreadCount() int {
	n := 0
	for _, is := range l.issues {
		if l.unread(is) {
			n++
		}
	}
	return n
}

// nextUnread moves the cursor to the next unread row, wrapping around.
func (l *listModel) nextUnread(m *Model) tea.Cmd {
	rs := l.rows()
	for step := 1; step <= len(rs); step++ {
		i := (l.cursor + step) % len(rs)
		if rs[i].idx >= 0 && l.unread(l.issues[rs[i].idx]) {
			l.cursor = i
			l.scrollToCursor(m)
			return nil
		}
	}
	return infoCmd(l.host(), "no unread issues on this page")
}

// toggleRead marks the targets read, or unread when the issue under the
// cursor (or the first target) is already read.
func (l *listModel) toggleRead(m *Model) tea.Cmd {
	targets := l.targets(m)
	if len(targets) == 0 {
		return nil
	}
	lead := targets[0]
	if is := m.currentIssue(); is != nil {
		lead = *is
	}
	read := l.unread(lead)
	for _, is := range targets {
		if read {
			l.seen.Mark(l.host(), is.ID, is.UpdatedAt)
		} else {
			l.seen.MarkUnread(l.host(), is.ID)
		}
	}
	word := "unread"
	if read {
		word = "read"
	}
	return tea.Batch(m.saveSeen(), infoCmd(l.host(), fmt.Sprintf("marked %s %s", targetLabel(targets), word)))
}

// markPageRead marks every issue on the loaded page read.
func (l *listModel) markPageRead(m *Model) tea.Cmd {
	for _, is := range l.issues {
		l.seen.Mark(l.host(), is.ID, is.UpdatedAt)
	}
	return tea.Batch(m.saveSeen(), infoCmd(l.host(), "marked the page read"))
}
