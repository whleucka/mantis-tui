package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	colorAccent = lipgloss.Color("12")
	colorMuted  = lipgloss.Color("8")
	colorError  = lipgloss.Color("9")

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(colorMuted)
	styleModal    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 1)
	styleBar      = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("236"))
	styleBarHost  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(colorAccent).Padding(0, 1)
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }
