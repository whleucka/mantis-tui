package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

const requestTimeout = 30 * time.Second

// listModel is the issue list for one host.
type listModel struct {
	sess     *Session
	cfg      config.ListConfig
	icons    config.Icons
	filter   string
	page     int
	pageSize int

	issues []mantis.Issue
	cursor int
	offset int // first visible row
	loaded bool
	req    int // latest load request; older responses are ignored

	meID   int
	colors map[string]string
}

type (
	issuesLoadedMsg struct {
		host   string
		req    int
		page   int
		keepID int // issue to keep the cursor on, 0 for the first row
		issues []mantis.Issue
		err    error
	}
	listMetaMsg struct {
		host   string
		meID   int
		colors map[string]string
		err    error
	}
)

func (msg issuesLoadedMsg) hostName() string { return msg.host }
func (msg listMetaMsg) hostName() string     { return msg.host }

func newListModel(cfg *config.Config, sess *Session) *listModel {
	return &listModel{
		sess:     sess,
		cfg:      cfg.List,
		icons:    cfg.Icons,
		filter:   cfg.List.DefaultFilter,
		page:     1,
		pageSize: cfg.List.PageSize,
	}
}

func (l *listModel) init(m *Model) tea.Cmd {
	return tea.Batch(l.load(m, 1, 0), l.loadMeta())
}

func (l *listModel) host() string { return l.sess.Host.Name }

// load fetches a page, keeping the cursor on keepID when it is still there.
func (l *listModel) load(m *Model, page, keepID int) tea.Cmd {
	l.req++
	req, host, api := l.req, l.host(), l.sess.API
	opts := mantis.ListOptions{Filter: l.filter, Page: page, PageSize: l.pageSize, Select: mantis.ListFields}
	fetch := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		res, err := api.ListIssues(ctx, opts)
		msg := issuesLoadedMsg{host: host, req: req, page: page, keepID: keepID, err: err}
		if err == nil {
			msg.issues = res.Issues
		}
		return msg
	}
	return tea.Batch(m.startLoading(), fetch)
}

func (l *listModel) loadMeta() tea.Cmd {
	host, meta := l.host(), l.sess.Meta
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		me, err := meta.Me(ctx)
		if err != nil {
			return listMetaMsg{host: host, err: err}
		}
		colors, err := meta.StatusColors(ctx)
		return listMetaMsg{host: host, meID: me.ID, colors: colors, err: err}
	}
}

func (l *listModel) context() string {
	s := fmt.Sprintf("%s · page %d", l.filter, l.page)
	if l.loaded {
		s += fmt.Sprintf(" · %d issues", len(l.issues))
	}
	return s
}

func (l *listModel) currentID() int {
	if l.cursor < 0 || l.cursor >= len(l.issues) {
		return 0
	}
	return l.issues[l.cursor].ID
}

func (l *listModel) handleMsg(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case issuesLoadedMsg:
		m.stopLoading()
		if msg.req != l.req {
			return nil // superseded by a newer request
		}
		if msg.err != nil {
			return errCmd(l.host(), msg.err)
		}
		if msg.page > 1 && len(msg.issues) == 0 {
			return infoCmd(l.host(), "no more issues")
		}
		l.page, l.issues, l.loaded = msg.page, msg.issues, true
		l.cursor = 0
		for i, is := range l.issues {
			if is.ID == msg.keepID {
				l.cursor = i
			}
		}
		l.scrollToCursor(m)
	case listMetaMsg:
		if msg.err != nil {
			return errCmd(l.host(), msg.err)
		}
		l.meID, l.colors = msg.meID, msg.colors
	}
	return nil
}

func (l *listModel) handleAction(m *Model, a action) tea.Cmd {
	switch a {
	case actQuit:
		return tea.Quit
	case actUp:
		l.move(m, -1)
	case actDown:
		l.move(m, 1)
	case actTop:
		l.move(m, -len(l.issues))
	case actBottom:
		l.move(m, len(l.issues))
	case actRefresh:
		return l.load(m, l.page, l.currentID())
	case actNextPage:
		if len(l.issues) < l.pageSize {
			return infoCmd(l.host(), "already on the last page")
		}
		return l.load(m, l.page+1, 0)
	case actPrevPage:
		if l.page > 1 {
			return l.load(m, l.page-1, 0)
		}
	case actFilter:
		m.modal = l.filterPicker()
	case actOpen:
		if id := l.currentID(); id != 0 {
			return m.openIssue(id)
		}
	}
	return nil
}

func (l *listModel) filterPicker() *picker {
	opts := make([]pickerOption, len(config.Filters))
	for i, f := range config.Filters {
		opts[i] = pickerOption{label: f, value: f, current: f == l.filter}
	}
	host := l.host()
	return newPicker("Filter issues", opts, func(o pickerOption) tea.Cmd {
		return func() tea.Msg { return filterChosenMsg{host: host, filter: o.value.(string)} }
	})
}

type filterChosenMsg struct {
	host   string
	filter string
}

func (msg filterChosenMsg) hostName() string { return msg.host }

func (l *listModel) setFilter(m *Model, f string) tea.Cmd {
	l.filter = f
	return l.load(m, 1, 0)
}

func (l *listModel) move(m *Model, delta int) {
	if len(l.issues) == 0 {
		return
	}
	l.cursor = min(max(l.cursor+delta, 0), len(l.issues)-1)
	l.scrollToCursor(m)
}

// bodyRows is how many issue rows fit under the header.
func bodyRows(m *Model) int { return max(m.height-2, 1) }

func (l *listModel) scrollToCursor(m *Model) {
	rows := bodyRows(m)
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+rows {
		l.offset = l.cursor - rows + 1
	}
	l.offset = max(min(l.offset, len(l.issues)-rows), 0)
}

// column is one list column; width 0 means "fill the rest" (summary).
type column struct {
	title string
	width int
	right bool
	cell  func(l *listModel, is mantis.Issue) string
}

// columnsFor drops lower-value columns as the terminal narrows.
func columnsFor(width int) []column {
	cols := []column{
		{title: "", width: 2, cell: func(l *listModel, is mantis.Issue) string { return l.priorityIcon(is.Priority.Name) }},
		{title: "ID", width: 6, right: true, cell: func(_ *listModel, is mantis.Issue) string { return fmt.Sprint(is.ID) }},
		{title: "SEVERITY", width: 9, cell: func(_ *listModel, is mantis.Issue) string { return is.Severity.Label }},
		{title: "STATUS", width: 13, cell: func(_ *listModel, is mantis.Issue) string { return is.Status.Label }},
		{title: "CATEGORY", width: 12, cell: func(_ *listModel, is mantis.Issue) string { return is.Category.Name }},
		{title: "SUMMARY", width: 0, cell: func(_ *listModel, is mantis.Issue) string { return is.Summary }},
		{title: "UPDATED", width: 10, cell: func(_ *listModel, is mantis.Issue) string { return dateOf(is.UpdatedAt) }},
		{title: "", width: 2, cell: func(l *listModel, is mantis.Issue) string {
			if l.meID != 0 && service.IsMonitoring(is, l.meID) {
				return l.icons.Monitor
			}
			return ""
		}},
	}
	drop := func(title string) {
		for i, c := range cols {
			if c.title == title {
				cols = append(cols[:i], cols[i+1:]...)
				return
			}
		}
	}
	if width < 110 {
		drop("CATEGORY")
	}
	if width < 90 {
		drop("SEVERITY")
	}
	if width < 70 {
		drop("UPDATED")
	}
	return cols
}

func (l *listModel) priorityIcon(name string) string {
	switch name {
	case "immediate":
		return l.icons.Immediate
	case "urgent":
		return l.icons.Urgent
	case "high":
		return l.icons.High
	case "normal":
		return l.icons.Normal
	case "low":
		return l.icons.Low
	}
	return l.icons.None
}

func dateOf(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02")
}

func (l *listModel) view(_ *Model, width, height int) string {
	if !l.loaded {
		return styleMuted.Render("  Loading issues…")
	}
	if len(l.issues) == 0 {
		return styleMuted.Render(fmt.Sprintf("  No issues (filter: %s, page %d). F changes the filter.", l.filter, l.page))
	}

	cols := columnsFor(width)
	const marker = 2 // cursor/selection gutter
	fixed := marker
	for _, c := range cols {
		fixed += c.width + 1
	}
	summary := max(width-fixed, 10)

	var b strings.Builder
	header := strings.Repeat(" ", marker)
	for _, c := range cols {
		w := c.width
		if w == 0 {
			w = summary
		}
		header += fit(c.title, w, c.right) + " "
	}
	b.WriteString(styleHeader.Render(ansi.Truncate(header, width, "")) + "\n")

	rows := max(height-1, 1)
	for i := l.offset; i < len(l.issues) && i < l.offset+rows; i++ {
		is := l.issues[i]
		var cells []string
		plain := ""
		for _, c := range cols {
			w := c.width
			if w == 0 {
				w = summary
			}
			text := fit(c.cell(l, is), w, c.right)
			plain += text + " "
			if c.title == "STATUS" && i != l.cursor {
				if hex := l.colors[is.Status.Name]; hex != "" {
					text = lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(text)
				}
			}
			cells = append(cells, text)
		}
		line := "  " + strings.Join(cells, " ")
		if i == l.cursor {
			line = styleSelected.Render(ansi.Truncate("▸ "+plain, width, ""))
		}
		b.WriteString(ansi.Truncate(line, width, "") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// fit pads or truncates s to exactly w cells.
func fit(s string, w int, right bool) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	pad := strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
	if right {
		return pad + s
	}
	return s + pad
}

func errCmd(host string, err error) tea.Cmd {
	return func() tea.Msg { return errMsg{host: host, err: err} }
}

func infoCmd(host, text string) tea.Cmd {
	return func() tea.Msg { return infoMsg{host: host, text: text} }
}
