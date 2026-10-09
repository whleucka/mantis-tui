package tui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
)

// preTag matches an opening or closing <pre> tag, any case, with attributes.
var preTag = regexp.MustCompile(`(?i)<(/?)pre(\s[^>]*)?>`)

// segment is a run of body text: prose, or the inside of a <pre> block.
type segment struct {
	text  string
	code  bool
	attrs string // the <pre> tag's attributes, for a class naming the language
}

// splitPre cuts s into prose and <pre> blocks. An unclosed <pre> runs to the
// end; a stray </pre> stays as text.
func splitPre(s string) []segment {
	var segs []segment
	add := func(text string, code bool, attrs string) {
		if code {
			text = strings.TrimPrefix(strings.TrimPrefix(text, "\r"), "\n")
			text = strings.TrimRight(text, "\r\n")
		} else {
			text = strings.Trim(text, "\r\n")
		}
		if strings.TrimSpace(text) != "" {
			segs = append(segs, segment{text, code, attrs})
		}
	}
	in, start, attrs := false, 0, ""
	for _, m := range preTag.FindAllStringSubmatchIndex(s, -1) {
		closing := m[3] > m[2]
		if closing != in {
			continue // nested <pre> or stray </pre>: keep scanning
		}
		add(s[start:m[0]], in, attrs)
		in, start, attrs = !in, m[1], ""
		if in && m[4] >= 0 {
			attrs = s[m[4]:m[5]]
		}
	}
	add(s[start:], in, attrs)
	return segs
}

// renderBody lays out a description or note in w columns: prose word-wraps,
// <pre> blocks keep their spacing behind a gutter, hard-wrap when wide and
// are highlighted in st (nil for none).
func renderBody(s string, w int, st *chroma.Style) string {
	w = max(w-2, 10)
	prose := lipgloss.NewStyle().Width(w)
	var out []string
	prevCode := false
	for i, seg := range splitPre(s) {
		if i > 0 && (seg.code || prevCode) {
			out = append(out, "") // a blank line either side of a block
		}
		prevCode = seg.code
		if !seg.code {
			out = append(out, prose.Render(seg.text))
			continue
		}
		gutter := styleMuted.Render("│ ")
		code := strings.NewReplacer("\r", "", "\t", "    ").Replace(seg.text)
		for _, line := range highlight(code, seg.attrs, st) {
			for part := range strings.SplitSeq(ansi.Hardwrap(line, w-2, true), "\n") {
				out = append(out, gutter+part)
			}
		}
	}
	return strings.Join(out, "\n")
}

// stripPre drops <pre> tags from a one-line excerpt.
func stripPre(s string) string { return strings.TrimSpace(preTag.ReplaceAllString(s, "")) }
