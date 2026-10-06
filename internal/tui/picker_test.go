package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "alt+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}
	}
	if len(s) > 5 && s[:5] == "ctrl+" {
		return tea.KeyPressMsg{Code: rune(s[5]), Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

type pickedMsg struct{ value any }

func newTestPicker() *picker {
	return newPicker("Status", []pickerOption{
		{label: "new", value: 10},
		{label: "assigned", value: 50, current: true},
		{label: "resolved", value: 80},
		{label: "closed", value: 90},
	}, func(o pickerOption) tea.Cmd {
		return func() tea.Msg { return pickedMsg{o.value} }
	})
}

func pick(t *testing.T, p *picker, keys ...string) (tea.Msg, bool) {
	t.Helper()
	var cmd tea.Cmd
	var done bool
	for _, k := range keys {
		cmd, done = p.update(key(k))
	}
	if cmd == nil {
		return nil, done
	}
	return cmd(), done
}

func TestPickerStartsOnCurrentValue(t *testing.T) {
	msg, done := pick(t, newTestPicker(), "enter")
	if !done || msg.(pickedMsg).value != 50 {
		t.Errorf("got %v done=%v, want current value 50", msg, done)
	}
}

func TestPickerNavigatesAndSelects(t *testing.T) {
	msg, _ := pick(t, newTestPicker(), "down", "enter")
	if msg.(pickedMsg).value != 80 {
		t.Errorf("got %v", msg)
	}
	msg, _ = pick(t, newTestPicker(), "up", "up", "up", "enter") // clamps at top
	if msg.(pickedMsg).value != 10 {
		t.Errorf("got %v", msg)
	}
}

func TestPickerFilterNarrowsOptions(t *testing.T) {
	p := newTestPicker()
	msg, _ := pick(t, p, "c", "l", "enter")
	if msg.(pickedMsg).value != 90 {
		t.Errorf("typing 'cl' should select closed, got %v", msg)
	}

	p = newTestPicker()
	pick(t, p, "z", "z")
	if len(p.visible()) != 0 {
		t.Fatalf("visible = %v", p.visible())
	}
	if msg, done := pick(t, p, "enter"); msg != nil || done {
		t.Error("enter with no matches should do nothing")
	}
	pick(t, p, "backspace", "backspace")
	if len(p.visible()) != 4 {
		t.Errorf("backspace should widen the filter again, visible = %d", len(p.visible()))
	}
}

func TestPickerEscCancels(t *testing.T) {
	msg, done := pick(t, newTestPicker(), "esc")
	if msg != nil || !done {
		t.Errorf("esc: msg %v done %v", msg, done)
	}
}

func TestPickerViewMarksCursorAndCurrent(t *testing.T) {
	p := newTestPicker()
	out := p.view(60, 20)
	for _, want := range []string{"Status", "assigned", "closed", "●"} {
		if !contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}
