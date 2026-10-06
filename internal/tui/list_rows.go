package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// row is one display line of the list: an issue (idx into issues) or a
// project group header (idx -1).
type row struct {
	idx    int
	header string
	count  int
}

// rows applies the search and grouping to the loaded page.
func (l *listModel) rows() []row {
	var idxs []int
	terms := strings.Fields(strings.ToLower(l.search))
	for i, is := range l.issues {
		if matches(is, terms) {
			idxs = append(idxs, i)
		}
	}
	if !l.grouped {
		out := make([]row, len(idxs))
		for i, idx := range idxs {
			out[i] = row{idx: idx}
		}
		return out
	}

	sort.SliceStable(idxs, func(a, b int) bool {
		return strings.ToLower(l.issues[idxs[a]].Project.Name) < strings.ToLower(l.issues[idxs[b]].Project.Name)
	})
	var out []row
	for i := 0; i < len(idxs); {
		name := l.issues[idxs[i]].Project.Name
		j := i
		for j < len(idxs) && l.issues[idxs[j]].Project.Name == name {
			j++
		}
		out = append(out, row{idx: -1, header: name, count: j - i})
		for _, idx := range idxs[i:j] {
			out = append(out, row{idx: idx})
		}
		i = j
	}
	return out
}

// matches reports whether every search term fuzzily matches one word of the
// issue's id, summary, category or handler (as a subsequence within a word,
// so "srch" finds "search" but letters are not collected across words).
func matches(is mantis.Issue, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	hay := fmt.Sprintf("%d %s %s", is.ID, is.Summary, is.Category.Name)
	if is.Handler != nil {
		hay += " " + is.Handler.Name + " " + is.Handler.RealName
	}
	words := strings.Fields(strings.ToLower(hay))
	for _, t := range terms {
		found := false
		for _, w := range words {
			if subsequence(t, w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func subsequence(needle, hay string) bool {
	h := []rune(hay)
	i := 0
	for _, r := range needle {
		for i < len(h) && h[i] != r {
			i++
		}
		if i == len(h) {
			return false
		}
		i++
	}
	return true
}

func (l *listModel) setSearch(m *Model, q string) {
	id := l.currentID()
	l.search = q
	l.cursorTo(m, id)
}

// searchInput captures keys while the user types a search; the query itself
// is drawn by the list, so it has no box of its own.
type searchInput struct {
	l *listModel
	m *Model
}

func (s *searchInput) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		s.l.setSearch(s.m, "")
		return nil, true
	case "enter":
		return nil, true
	case "backspace":
		if r := []rune(s.l.search); len(r) > 0 {
			s.l.setSearch(s.m, string(r[:len(r)-1]))
		}
	default:
		if msg.Text != "" {
			s.l.setSearch(s.m, s.l.search+msg.Text)
		}
	}
	return nil, false
}

func (s *searchInput) view(int, int) string { return "" }
