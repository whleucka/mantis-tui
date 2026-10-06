package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// status is the bottom line: host, context, spinner and the last message.
type status struct {
	msg   string
	isErr bool
}

func (s *status) info(msg string) { s.msg, s.isErr = msg, false }
func (s *status) err(msg string)  { s.msg, s.isErr = msg, true }
func (s *status) clear()          { s.msg, s.isErr = "", false }

func (m *Model) statusView() string {
	var left []string
	if m.cur != nil {
		left = append(left, styleBarHost.Render(m.cur.sess.Host.Name))
		if ctx := m.cur.context(); ctx != "" {
			left = append(left, styleBar.Render(" "+ctx+" "))
		}
	}
	if m.loading > 0 {
		left = append(left, styleBar.Render(" "+m.spin.View()+" "))
	}
	if len(m.chord.pending) > 0 {
		left = append(left, styleBar.Render(" "+strings.Join(m.chord.pending, " ")+"- "))
	}
	l := lipgloss.JoinHorizontal(lipgloss.Top, left...)

	msg := m.status.msg
	style := styleBar
	if m.status.isErr {
		style = styleBar.Foreground(colorError)
	}
	room := m.width - lipgloss.Width(l) - 1
	if room < 0 {
		room = 0
	}
	if r := []rune(msg); len(r) > room {
		if room > 1 {
			msg = string(r[:room-1]) + "…"
		} else {
			msg = ""
		}
	}
	right := style.Width(room + 1).Align(lipgloss.Right).Render(msg + " ")
	return l + right
}
