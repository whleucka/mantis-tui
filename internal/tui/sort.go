package tui

import (
	"cmp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
)

// sortField describes one way to order the list. compare returns <0 when a
// comes first in the field's natural direction.
type sortField struct {
	name    string
	column  string // header of the column that shows the field
	desc    string // the natural direction, for the picker
	natural bool   // true when the natural direction is descending values
	compare func(a, b mantis.Issue) int
}

var sortFields = map[string]sortField{
	"updated": {"updated", "UPDATED", "newest first", true, func(a, b mantis.Issue) int { return b.UpdatedAt.Compare(a.UpdatedAt) }},
	"priority": {"priority", "", "highest first", true, func(a, b mantis.Issue) int {
		return cmp.Compare(b.Priority.ID, a.Priority.ID)
	}},
	"severity": {"severity", "SEVERITY", "worst first", true, func(a, b mantis.Issue) int {
		return cmp.Compare(b.Severity.ID, a.Severity.ID)
	}},
	"status": {"status", "STATUS", "workflow order", false, func(a, b mantis.Issue) int {
		return cmp.Compare(a.Status.ID, b.Status.ID)
	}},
	"id": {"id", "ID", "newest first", true, func(a, b mantis.Issue) int { return cmp.Compare(b.ID, a.ID) }},
	"summary": {"summary", "SUMMARY", "A–Z", false, func(a, b mantis.Issue) int {
		return cmp.Compare(strings.ToLower(a.Summary), strings.ToLower(b.Summary))
	}},
}

// listSort is the list's current order.
type listSort struct {
	field   string
	reverse bool
}

// arrow shows which way the values run: ↓ largest (or newest) first.
func (s listSort) arrow() string {
	if sortFields[s.field].natural != s.reverse {
		return "↓"
	}
	return "↑"
}

// sortIssues orders l.issues by the current sort. Ties go to the most
// recently updated, then the highest id, whatever the direction.
func (l *listModel) sortIssues() {
	f := sortFields[l.sort.field]
	slices.SortStableFunc(l.issues, func(a, b mantis.Issue) int {
		c := f.compare(a, b)
		if l.sort.reverse {
			c = -c
		}
		if c != 0 {
			return c
		}
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})
}

// resort reapplies the sort, keeping the cursor on its issue.
func (l *listModel) resort(m *Model) {
	id := l.currentID()
	l.sortIssues()
	l.cursorTo(m, id)
}

type sortChosenMsg struct {
	host  string
	field string
}

func (msg sortChosenMsg) hostName() string { return msg.host }

// setSort sorts by field; choosing the current field again reverses it.
func (l *listModel) setSort(m *Model, field string) {
	if field == l.sort.field {
		l.sort.reverse = !l.sort.reverse
	} else {
		l.sort = listSort{field: field}
	}
	l.resort(m)
}

func (l *listModel) sortPicker() *picker {
	opts := make([]pickerOption, len(config.SortFields))
	for i, name := range config.SortFields {
		f := sortFields[name]
		detail := f.desc
		if name == l.sort.field {
			detail = l.sort.arrow() + " · choose again to reverse"
		}
		opts[i] = pickerOption{label: name, detail: detail, value: name, current: name == l.sort.field}
	}
	host := l.host()
	return newPicker("Sort by", opts, func(o pickerOption) tea.Cmd {
		return func() tea.Msg { return sortChosenMsg{host: host, field: o.value.(string)} }
	})
}
