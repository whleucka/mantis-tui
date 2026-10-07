package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/config"
)

// paletteRunMsg runs a chosen command once the palette has closed, so a
// command that opens another modal is not closed along with the palette.
type paletteRunMsg struct{ run func(*Model) tea.Cmd }

type paletteEntry struct {
	label string
	keys  string // shortcut shown on the right; "" when there is none
	run   func(*Model) tea.Cmd
}

// palette is a fuzzy-filtered list of every command on the current screen.
type palette struct {
	query   string
	entries []paletteEntry
	cursor  int // index into visible()
}

// paletteSkip are bindings too small to be worth a palette row.
var paletteSkip = map[action]bool{
	actPalette: true, actJumpHost: true, actUp: true, actDown: true, actPageUp: true, actPageDown: true,
	actPreviewUp: true, actPreviewDown: true,
}

func (m *Model) newPalette() *palette {
	p := &palette{}
	for _, b := range m.bindings() {
		if paletteSkip[b.action] {
			continue
		}
		a := b.action
		p.entries = append(p.entries, paletteEntry{
			label: capitalize(b.help),
			keys:  keyLabel(b.keys),
			run:   func(m *Model) tea.Cmd { return m.dispatch(a) },
		})
	}
	if m.cur.screen == screenList {
		l := m.cur.list
		for _, f := range config.Filters {
			label := "Filter: " + f
			if f == l.filter {
				label += " (current)"
			}
			p.entries = append(p.entries, paletteEntry{label: label, run: func(m *Model) tea.Cmd {
				return m.cur.list.setFilter(m, f)
			}})
		}
		p.entries = append(p.entries, paletteEntry{label: "Mark all read", run: func(m *Model) tea.Cmd {
			return m.cur.list.markAllRead(m)
		}})
	}
	for _, h := range m.opts.Hosts {
		if h.Name == m.cur.sess.Host.Name {
			continue
		}
		p.entries = append(p.entries, paletteEntry{label: "Host: " + h.Name, run: func(m *Model) tea.Cmd {
			return m.selectHost(h.Name)
		}})
	}
	return p
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// issueQuery returns the issue id the query names ("123" or "#123").
func issueQuery(q string) (int, bool) {
	id, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(q), "#"))
	return id, err == nil && id > 0
}

// visible lists the entries matching the query: "Open issue #N" first when
// the query is a number, then labels starting with the query, then labels
// where every term starts a word, then other fuzzy matches. Each tier keeps
// the original order.
func (p *palette) visible() []paletteEntry {
	var out []paletteEntry
	if id, ok := issueQuery(p.query); ok {
		out = append(out, paletteEntry{label: fmt.Sprintf("Open issue #%d", id), run: func(m *Model) tea.Cmd {
			return m.openIssue(id)
		}})
	}
	q := strings.ToLower(strings.TrimSpace(p.query))
	terms := strings.Fields(q)
	var tiers [3][]paletteEntry
	for _, e := range p.entries {
		hay := strings.ToLower(e.label + " " + e.keys)
		switch {
		case q == "" || strings.HasPrefix(strings.ToLower(e.label), q):
			tiers[0] = append(tiers[0], e)
		case wordPrefixes(hay, terms):
			tiers[1] = append(tiers[1], e)
		case fuzzyWords(hay, terms):
			tiers[2] = append(tiers[2], e)
		}
	}
	for _, t := range tiers {
		out = append(out, t...)
	}
	return out
}

// wordPrefixes reports whether every term starts some word of hay.
func wordPrefixes(hay string, terms []string) bool {
	words := strings.Fields(hay)
	for _, t := range terms {
		if !slices.ContainsFunc(words, func(w string) bool { return strings.HasPrefix(w, t) }) {
			return false
		}
	}
	return len(terms) > 0
}

func (p *palette) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	vis := p.visible()
	switch msg.String() {
	case "esc":
		return nil, true
	case "enter":
		if len(vis) == 0 {
			return nil, false
		}
		run := vis[min(p.cursor, len(vis)-1)].run
		return func() tea.Msg { return paletteRunMsg{run: run} }, true
	case "up", "ctrl+p", "ctrl+k":
		p.cursor = max(p.cursor-1, 0)
	case "down", "ctrl+n", "ctrl+j":
		p.cursor = min(p.cursor+1, max(len(vis)-1, 0))
	case "backspace":
		if r := []rune(p.query); len(r) > 0 {
			p.query = string(r[:len(r)-1])
			p.cursor = 0
		}
	default:
		if msg.Text != "" {
			p.query += msg.Text
			p.cursor = 0
		}
	}
	return nil, false
}

func (p *palette) view(width, height int) string {
	boxW := min(max(width/2, 50), width-2)
	inner := max(boxW-4, 10)
	var b strings.Builder
	b.WriteString(styleTitle.Render(": ") + p.query + styleMuted.Render("▏") + "\n")

	vis := p.visible()
	start, rows := p.window(height)
	for i := start; i < len(vis) && i < start+rows; i++ {
		e := vis[i]
		keys := e.keys
		labelW := max(inner-ansi.StringWidth(keys)-2, 1)
		line := fit(e.label, labelW, false) + "  "
		if i == p.cursor {
			line = styleSelected.Render(line) + styleMuted.Render(keys)
		} else {
			line += styleMuted.Render(keys)
		}
		b.WriteString(ansi.Truncate(line, inner, "…") + "\n")
	}
	if len(vis) == 0 {
		b.WriteString(styleMuted.Render("  no matching command") + "\n")
	}
	b.WriteString(styleMuted.Render(ansi.Truncate("enter run · type a number to open an issue · esc close", inner, "…")))
	return styleModal.Width(boxW).Render(b.String())
}
