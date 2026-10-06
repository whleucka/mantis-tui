package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/config"
)

// listModel is the issue list for one host (filled in by Task 11).
type listModel struct {
	filter string
}

func newListModel(cfg *config.Config) *listModel {
	return &listModel{filter: cfg.List.DefaultFilter}
}

func (l *listModel) init(_ *Model) tea.Cmd { return nil }

func (l *listModel) context() string { return "filter: " + l.filter }

func (l *listModel) handleAction(_ *Model, a action) tea.Cmd {
	if a == actQuit {
		return tea.Quit
	}
	return nil
}

func (l *listModel) handleMsg(_ *Model, _ tea.Msg) tea.Cmd { return nil }

func (l *listModel) view(_ *Model, _, _ int) string { return "" }
