package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// helpModal lists the bindings of the current screen, generated from the
// keymap so it cannot drift from the real keys.
type helpModal struct {
	title   string
	entries [][2]string // key label, description
	offset  int
}

func newHelp(title string, bindings []binding) *helpModal {
	h := &helpModal{title: title}
	for _, b := range bindings {
		h.entries = append(h.entries, [2]string{keyLabel(b.keys), b.help})
	}
	return h
}

// keyLabel shows sequences compactly: "g g" → "gg", alternatives joined by
// "/", and the digit run 1…9 as "1-9".
func keyLabel(keys []string) string {
	if strings.Join(keys, "") == "123456789" {
		return "1-9"
	}
	out := make([]string, len(keys))
	for i, k := range keys {
		parts := strings.Fields(k)
		short := true
		for _, p := range parts {
			if len([]rune(p)) > 1 {
				short = false
			}
		}
		if short {
			out[i] = strings.Join(parts, "")
		} else {
			out[i] = strings.Join(parts, " ")
		}
	}
	return strings.Join(out, "/")
}

func (h *helpModal) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "?", "esc", "q":
		return nil, true
	case "j", "down":
		h.offset = min(h.offset+1, max(len(h.entries)-1, 0))
	case "k", "up":
		h.offset = max(h.offset-1, 0)
	}
	return nil, false
}

func (h *helpModal) view(width, height int) string {
	keyW := 0
	for _, e := range h.entries {
		keyW = max(keyW, ansi.StringWidth(e[0]))
	}
	rows := max(height-6, 3) // border, title, footer
	start := min(h.offset, max(len(h.entries)-rows, 0))
	var b strings.Builder
	b.WriteString(styleTitle.Render(h.title) + "\n")
	for _, e := range h.entries[start:min(start+rows, len(h.entries))] {
		b.WriteString(styleGroup.Render(fmt.Sprintf("%-*s", keyW, e[0])) + "  " + e[1] + "\n")
	}
	footer := "? or esc to close"
	if len(h.entries) > rows {
		footer = fmt.Sprintf("j/k scroll (%d-%d of %d) · %s", start+1, min(start+rows, len(h.entries)), len(h.entries), footer)
	}
	b.WriteString(styleMuted.Render(footer))
	return styleModal.Width(min(max(width*2/3, 40), width-2)).Render(b.String())
}
