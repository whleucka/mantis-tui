package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// confirmModal asks a yes/no question; only "y" confirms.
type confirmModal struct {
	prompt string
	onYes  func() tea.Cmd
}

func (c *confirmModal) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "y", "Y":
		return c.onYes(), true
	case "n", "N", "esc", "q":
		return nil, true
	}
	return nil, false
}

func (c *confirmModal) view(width, _ int) string {
	body := styleTitle.Render(c.prompt) + "\n\n" + styleMuted.Render("y confirm · n/esc cancel")
	return styleModal.Width(min(max(width/2, 40), width-2)).Render(body)
}

// textPrompt edits one line of text.
type textPrompt struct {
	title    string
	input    textinput.Model
	errText  string
	onSubmit func(string) (tea.Cmd, string) // returns an error text to stay open
}

func newTextPrompt(title, value string, width int, onSubmit func(string) (tea.Cmd, string)) *textPrompt {
	in := textinput.New()
	in.SetValue(value)
	in.CursorEnd()
	in.SetWidth(max(min(width-10, 100), 20))
	in.Focus()
	return &textPrompt{title: title, input: in, onSubmit: onSubmit}
}

func (p *textPrompt) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		return nil, true
	case "enter":
		cmd, errText := p.onSubmit(strings.TrimSpace(p.input.Value()))
		if errText != "" {
			p.errText = errText
			return nil, false
		}
		return cmd, true
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.errText = ""
	return cmd, false
}

func (p *textPrompt) view(width, _ int) string {
	body := styleTitle.Render(p.title) + "\n" + p.input.View()
	if p.errText != "" {
		body += "\n" + styleError.Render(p.errText)
	}
	body += "\n" + styleMuted.Render("enter save · esc cancel")
	return styleModal.Width(min(max(width*2/3, 40), width-2)).Render(body)
}
