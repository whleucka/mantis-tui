package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// pickerOption is one choice in a picker.
type pickerOption struct {
	label   string
	detail  string
	value   any
	current bool // the field's present value; the cursor starts here
}

// picker is a modal single-select list with type-to-filter.
type picker struct {
	title    string
	options  []pickerOption
	filter   string
	cursor   int // index into visible()
	onSelect func(pickerOption) tea.Cmd
}

func newPicker(title string, opts []pickerOption, onSelect func(pickerOption) tea.Cmd) *picker {
	p := &picker{title: title, options: opts, onSelect: onSelect}
	for i, o := range opts {
		if o.current {
			p.cursor = i
		}
	}
	return p
}

// visible returns indexes of options matching the filter: labels starting
// with it first, then other matches, each in their original order.
func (p *picker) visible() []int {
	f := strings.ToLower(p.filter)
	if f == "" {
		out := make([]int, len(p.options))
		for i := range out {
			out[i] = i
		}
		return out
	}
	var prefix, rest []int
	for i, o := range p.options {
		label := strings.ToLower(o.label)
		switch {
		case strings.HasPrefix(label, f):
			prefix = append(prefix, i)
		case strings.Contains(label+" "+strings.ToLower(o.detail), f):
			rest = append(rest, i)
		}
	}
	return append(prefix, rest...)
}

// update handles a key; done reports that the picker should close.
func (p *picker) update(msg tea.KeyPressMsg) (cmd tea.Cmd, done bool) {
	vis := p.visible()
	switch msg.String() {
	case "esc":
		return nil, true
	case "enter":
		if len(vis) == 0 {
			return nil, false
		}
		return p.onSelect(p.options[vis[p.cursor]]), true
	case "up", "ctrl+p", "ctrl+k":
		p.cursor = max(p.cursor-1, 0)
	case "down", "ctrl+n", "ctrl+j":
		p.cursor = min(p.cursor+1, max(len(vis)-1, 0))
	case "backspace":
		if r := []rune(p.filter); len(r) > 0 {
			p.filter = string(r[:len(r)-1])
			p.cursor = 0
		}
	default:
		if msg.Text != "" {
			p.filter += msg.Text
			p.cursor = 0
		}
	}
	return nil, false
}

func (p *picker) view(width, height int) string {
	boxW := min(max(width/2, 40), width-2)
	inner := max(boxW-4, 10) // border and padding
	var b strings.Builder
	b.WriteString(styleTitle.Render(p.title))
	if p.filter != "" {
		b.WriteString(styleMuted.Render("  filter: " + p.filter))
	}
	b.WriteString("\n")

	vis := p.visible()
	maxRows := max(height-6, 3)
	start := 0
	if p.cursor >= maxRows {
		start = p.cursor - maxRows + 1
	}
	for row, idx := range vis {
		if row < start || row >= start+maxRows {
			continue
		}
		o := p.options[idx]
		mark := "  "
		if o.current {
			mark = "● "
		}
		line := mark + o.label
		if o.detail != "" {
			line += styleMuted.Render("  " + o.detail)
		}
		if row == p.cursor {
			line = styleSelected.Render(mark + o.label)
			if o.detail != "" {
				line += styleMuted.Render("  " + o.detail)
			}
		}
		b.WriteString(ansi.Truncate(line, inner, "…") + "\n")
	}
	if len(vis) == 0 {
		b.WriteString(styleMuted.Render("  no matches") + "\n")
	}
	b.WriteString(styleMuted.Render(ansi.Truncate(fmt.Sprintf("%d/%d · type to filter · esc cancel", len(vis), len(p.options)), inner, "…")))
	return styleModal.Width(boxW).Render(b.String())
}
