package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
	"github.com/whleucka/mantis-tui/internal/service"
)

const requestTimeout = 30 * time.Second

// listModel is the issue list for one host.
type listModel struct {
	sess      *Session
	cfg       config.ListConfig
	icons     config.Icons
	filter    string
	pageSize  int // issues per request
	maxIssues int

	issues   []mantis.Issue
	cursor   int // index into rows()
	offset   int // first visible row
	grouped  bool
	sort     listSort
	search   string
	selected map[int]mantis.Issue // by id; survives refreshes
	loaded   bool
	loadErr  error // why the first load failed; shown until a load succeeds
	req      int   // latest load; responses for older loads are ignored
	chain    loadChain
	capped   bool // the last load stopped at maxIssues
	spinning bool // the running load shows the spinner

	meID   int
	colors map[string]string

	interval time.Duration // auto-refresh; 0 disables
	tickGen  int           // current auto-refresh chain
	inFlight bool          // a load is running

	pv   preview
	seen *config.Seen

	lastClickID int // for double clicks
	lastClickAt time.Time

	now func() time.Time // the model's clock, for relative times
}

// loadChain is one load of the whole filter, a chunk at a time.
type loadChain struct {
	keepID      int  // issue to put the cursor on when first shown; 0 keeps the current one
	progressive bool // show chunks as they arrive (first load, new filter)
	applied     bool // the list already shows part of this load
	acc         []mantis.Issue
	ids         map[int]bool
}

type (
	issuesLoadedMsg struct {
		host   string
		req    int
		page   int
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

// refreshTickMsg fires an auto-refresh for target ("list" or "issue").
type refreshTickMsg struct {
	host   string
	target string
	gen    int
}

func (msg refreshTickMsg) hostName() string { return msg.host }

func newListModel(cfg *config.Config, sess *Session, seen *config.Seen) *listModel {
	return &listModel{
		sess:      sess,
		cfg:       cfg.List,
		icons:     cfg.Icons,
		filter:    cfg.List.DefaultFilter,
		grouped:   cfg.List.GroupByProject,
		pageSize:  cfg.List.PageSize,
		sort:      listSort{field: cfg.List.Sort},
		maxIssues: cfg.List.MaxIssues,
		interval:  cfg.List.AutoRefresh.Duration,
		selected:  map[int]mantis.Issue{},
		pv:        newPreview(cfg.List.Preview),
		seen:      seen,
		now:       time.Now,
	}
}

func (l *listModel) init(m *Model) tea.Cmd {
	return tea.Batch(l.load(m, 0, true, false), l.loadMeta(), l.scheduleRefresh())
}

func (l *listModel) host() string { return l.sess.Host.Name }

// load fetches the whole filter, a chunk at a time, replacing any load still
// running. A progressive load shows chunks as they arrive; otherwise the
// list keeps showing the old issues until the last chunk is in. A silent
// load (auto-refresh) shows no spinner. keepID is the issue to put the
// cursor on; 0 keeps it where it is.
func (l *listModel) load(m *Model, keepID int, progressive, silent bool) tea.Cmd {
	l.endLoad(m) // a superseded load's responses are ignored from here on
	l.req++
	l.inFlight = true
	l.chain = loadChain{keepID: keepID, progressive: progressive, ids: map[int]bool{}}
	var spin tea.Cmd
	if !silent {
		l.spinning = true
		spin = m.startLoading()
	}
	return tea.Batch(spin, l.fetchChunk(1))
}

func (l *listModel) endLoad(m *Model) {
	l.inFlight = false
	if l.spinning {
		l.spinning = false
		m.stopLoading()
	}
}

func (l *listModel) fetchChunk(page int) tea.Cmd {
	req, host, api := l.req, l.host(), l.sess.API
	opts := mantis.ListOptions{Filter: l.filter, Page: page, PageSize: l.pageSize, Select: mantis.ListFields}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		res, err := api.ListIssues(ctx, opts)
		msg := issuesLoadedMsg{host: host, req: req, page: page, err: err}
		if err == nil {
			msg.issues = res.Issues
		}
		return msg
	}
}

// onChunk adds a chunk to the running load and asks for the next one until
// the filter or the cap runs out.
func (l *listModel) onChunk(m *Model, msg issuesLoadedMsg) tea.Cmd {
	if msg.req != l.req {
		return nil // superseded by a newer load
	}
	c := &l.chain
	if msg.err != nil {
		l.endLoad(m)
		if !l.loaded {
			l.loadErr = msg.err
		}
		return errCmd(l.host(), msg.err)
	}
	for _, is := range msg.issues { // an issue updated mid-load can move between chunks
		if !c.ids[is.ID] {
			c.ids[is.ID] = true
			c.acc = append(c.acc, is)
		}
	}
	capped := len(c.acc) >= l.maxIssues
	if capped {
		c.acc = c.acc[:l.maxIssues]
	}
	done := capped || len(msg.issues) < l.pageSize
	if c.progressive || done {
		keep := l.currentID()
		if !c.applied && c.keepID != 0 {
			keep = c.keepID
		}
		c.applied = true
		l.setIssues(m, c.acc, keep)
	}
	if !done {
		return l.fetchChunk(msg.page + 1)
	}
	l.capped = capped
	l.endLoad(m)
	return nil
}

// setIssues shows issues, keeping the cursor on keepID when it is there.
func (l *listModel) setIssues(m *Model, issues []mantis.Issue, keepID int) {
	l.issues = append([]mantis.Issue(nil), issues...)
	l.sortIssues()
	l.loaded, l.loadErr = true, nil
	for _, is := range l.issues { // keep selection snapshots fresh
		if _, ok := l.selected[is.ID]; ok {
			l.selected[is.ID] = is
		}
	}
	l.cursorTo(m, keepID)
}

// scheduleRefresh starts the next auto-refresh tick, replacing any earlier chain.
func (l *listModel) scheduleRefresh() tea.Cmd {
	if l.interval <= 0 {
		return nil
	}
	l.tickGen++
	msg := refreshTickMsg{host: l.host(), target: "list", gen: l.tickGen}
	return tea.Tick(l.interval, func(time.Time) tea.Msg { return msg })
}

// onTick re-fetches the page silently unless the user is busy elsewhere.
func (l *listModel) onTick(m *Model, msg refreshTickMsg) tea.Cmd {
	if msg.gen != l.tickGen {
		return nil
	}
	next := l.scheduleRefresh()
	hv := m.hosts[l.host()]
	if m.cur != hv || hv.screen != screenList || m.busy() || l.inFlight {
		return next
	}
	return tea.Batch(next, l.load(m, 0, false, true))
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
	s := l.filter
	if l.loaded {
		s += fmt.Sprintf(" · %d issues", len(l.issues))
		switch {
		case l.inFlight && l.chain.progressive:
			s += " (loading…)"
		case l.capped:
			s += fmt.Sprintf(" (limit %d)", l.maxIssues)
		}
		if l.sort != (listSort{field: "updated"}) {
			s += " · by " + l.sort.field + " " + l.sort.arrow()
		}
		if n := l.unreadCount(); n > 0 {
			s += fmt.Sprintf(" · %d unread", n)
		}
	}
	if n := len(l.selected); n > 0 {
		s += fmt.Sprintf(" · %d selected", n)
	}
	return s
}

func (l *listModel) currentID() int {
	rs := l.rows()
	if l.cursor < 0 || l.cursor >= len(rs) || rs[l.cursor].idx < 0 {
		return 0
	}
	return l.issues[rs[l.cursor].idx].ID
}

func (l *listModel) handleMsg(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case issuesLoadedMsg:
		return l.onChunk(m, msg)
	case listMetaMsg:
		if msg.err != nil {
			return errCmd(l.host(), msg.err)
		}
		l.meID, l.colors = msg.meID, msg.colors
	case sortChosenMsg:
		l.setSort(m, msg.field)
	case previewTickMsg:
		return l.onPreviewTick(msg)
	case previewLoadedMsg:
		return l.onPreviewLoaded(m, msg)
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
	case actToggleGroup:
		id := l.currentID()
		l.grouped = !l.grouped
		l.cursorTo(m, id)
	case actSearch:
		m.modal = &searchInput{l: l, m: m}
	case actEscape: // backs out one layer: the search, then the selection
		switch {
		case l.search != "":
			l.setSearch(m, "")
		case len(l.selected) > 0:
			l.selected = map[int]mantis.Issue{}
		}
	case actPageDown:
		l.move(m, max(bodyRows(m)/2, 1))
	case actPageUp:
		l.move(m, -max(bodyRows(m)/2, 1))
	case actRefresh:
		return l.load(m, 0, false, false)
	case actFilter:
		m.modal = l.filterPicker()
	case actSort:
		m.modal = l.sortPicker()
	case actOpen:
		if id := l.currentID(); id != 0 {
			return m.openIssue(id)
		}
	case actCreate:
		return m.openCreate()
	case actToggleSelect:
		if is := m.currentIssue(); is != nil {
			if _, ok := l.selected[is.ID]; ok {
				delete(l.selected, is.ID)
			} else {
				l.selected[is.ID] = *is
			}
			l.move(m, 1)
		}
	case actSelectAll:
		for _, r := range l.rows() {
			if r.idx >= 0 {
				l.selected[l.issues[r.idx].ID] = l.issues[r.idx]
			}
		}
	case actNextUnread:
		return l.nextUnread(m, 1)
	case actPrevUnread:
		return l.nextUnread(m, -1)
	case actToggleRead:
		return l.toggleRead(m)
	case actTogglePreview:
		return l.togglePreview(m)
	case actPreviewDown:
		l.pv.vp.HalfPageDown()
	case actPreviewUp:
		l.pv.vp.HalfPageUp()
	case actStatus, actPriority, actSeverity, actCategory, actAssign, actDelete:
		return l.batch(m, a)
	default:
		return m.issueAction(a)
	}
	return nil
}

// targets are the selected issues (in list order, newest first), or the
// issue under the cursor when nothing is selected.
func (l *listModel) targets(m *Model) []mantis.Issue {
	if len(l.selected) == 0 {
		if is := m.currentIssue(); is != nil {
			return []mantis.Issue{*is}
		}
		return nil
	}
	out := make([]mantis.Issue, 0, len(l.selected))
	for _, is := range l.selected {
		out = append(out, is)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// batch runs a field change or delete on the targets: the selection, or
// the issue under the cursor.
func (l *listModel) batch(m *Model, a action) tea.Cmd {
	targets := l.targets(m)
	if len(targets) == 0 {
		return nil
	}
	switch a {
	case actStatus:
		return m.pickEnum(meta.Status, targets)
	case actPriority:
		return m.pickEnum(meta.Priority, targets)
	case actSeverity:
		return m.pickEnum(meta.Severity, targets)
	case actCategory:
		return m.pickCategory(targets)
	case actAssign:
		return m.pickUser(targets)
	case actDelete:
		m.confirmDelete(targets)
	}
	return nil
}

// applyPatched swaps in updated issues and reports the outcome. Issues that
// succeeded leave the selection, so a partial failure leaves only the
// failures selected for a retry.
func (l *listModel) applyPatched(m *Model, msg patchedMsg) tea.Cmd {
	for _, r := range msg.results {
		if r.Err == nil {
			delete(l.selected, r.ID)
		}
	}
	for i, is := range l.issues {
		if fresh, ok := msg.updated[is.ID]; ok && fresh != nil && fresh.ID != 0 {
			fresh.Notes, fresh.History = nil, nil // the list never shows them
			l.issues[i] = *fresh
		}
	}
	l.resort(m) // a changed field can move the row
	text, err := summarize(msg.label, msg.results)
	if err != nil {
		return errCmd(l.host(), err)
	}
	return infoCmd(l.host(), text)
}

// removeDeleted drops deleted rows, keeping the cursor at the same position.
func (l *listModel) removeDeleted(m *Model, msg deletedMsg) tea.Cmd {
	gone := map[int]bool{}
	for _, r := range msg.results {
		if r.Err == nil {
			gone[r.ID] = true
			delete(l.selected, r.ID)
		}
	}
	// The issue that will occupy the cursor's row once the deleted ones go.
	next := 0
	rs := l.rows()
	for i := l.cursor; i < len(rs) && next == 0; i++ {
		if rs[i].idx >= 0 && !gone[l.issues[rs[i].idx].ID] {
			next = l.issues[rs[i].idx].ID
		}
	}
	for i := l.cursor; i >= 0 && next == 0 && i < len(rs); i-- {
		if rs[i].idx >= 0 && !gone[l.issues[rs[i].idx].ID] {
			next = l.issues[rs[i].idx].ID
		}
	}
	kept := l.issues[:0]
	for _, is := range l.issues {
		if !gone[is.ID] {
			kept = append(kept, is)
		}
	}
	l.issues = kept
	l.cursorTo(m, next)

	text, err := summarize("deleted", msg.results)
	if err != nil {
		return errCmd(l.host(), err)
	}
	return infoCmd(l.host(), text)
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
	return l.load(m, 0, true, false)
}

// move steps the cursor over issue rows, skipping group headers.
func (l *listModel) move(m *Model, delta int) {
	rs := l.rows()
	var issueRows []int
	pos := 0
	for i, r := range rs {
		if r.idx >= 0 {
			if i == l.cursor {
				pos = len(issueRows)
			}
			issueRows = append(issueRows, i)
		}
	}
	if len(issueRows) == 0 {
		return
	}
	l.cursor = issueRows[min(max(pos+delta, 0), len(issueRows)-1)]
	l.scrollToCursor(m)
}

// cursorTo puts the cursor on issue id, or the first issue row if it is not shown.
func (l *listModel) cursorTo(m *Model, id int) {
	rs := l.rows()
	l.cursor = 0
	first := -1
	for i, r := range rs {
		if r.idx < 0 {
			continue
		}
		if first < 0 {
			first = i
		}
		if l.issues[r.idx].ID == id {
			l.cursor = i
			l.scrollToCursor(m)
			return
		}
	}
	l.cursor = max(first, 0)
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
	l.offset = max(min(l.offset, len(l.rows())-rows), 0)
}

// column is one list column; width 0 means "fill the rest" (summary).
type column struct {
	key   string // the sort field it shows, if any
	title string
	width int
	right bool
	cell  func(l *listModel, is mantis.Issue) string
}

// columnsFor drops lower-value columns as the terminal narrows.
func columnsFor(width int) []column {
	cols := []column{
		{key: "priority", title: "", width: 2, cell: func(l *listModel, is mantis.Issue) string { return l.priorityIcon(is.Priority.Name) }},
		{key: "id", title: "ID", width: 6, right: true, cell: func(_ *listModel, is mantis.Issue) string { return fmt.Sprint(is.ID) }},
		{key: "severity", title: "SEVERITY", width: 9, cell: func(_ *listModel, is mantis.Issue) string { return is.Severity.Label }},
		{key: "status", title: "STATUS", width: 13, cell: func(_ *listModel, is mantis.Issue) string { return is.Status.Label }},
		{title: "CATEGORY", width: 12, cell: func(_ *listModel, is mantis.Issue) string { return is.Category.Name }},
		{title: "HANDLER", width: 12, cell: func(_ *listModel, is mantis.Issue) string {
			if is.Handler == nil {
				return ""
			}
			return is.Handler.Name
		}},
		{key: "summary", title: "SUMMARY", width: 0, cell: func(_ *listModel, is mantis.Issue) string { return is.Summary }},
		{key: "updated", title: "UPDATED", width: 10, cell: func(l *listModel, is mantis.Issue) string { return dateOf(is.UpdatedAt, l.now()) }},
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
	if width < 140 {
		drop("HANDLER")
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

func (l *listModel) view(m *Model, width, height int) string {
	lw, pw := l.previewWidths(width)
	list := l.listView(m, lw, height)
	if pw == 0 {
		return list
	}
	return withPreview(list, l.previewView(m.currentIssue(), pw), lw, pw, height)
}

func (l *listModel) listView(m *Model, width, height int) string {
	searchLine := ""
	if _, typing := m.modal.(*searchInput); typing || l.search != "" {
		searchLine = styleTitle.Render("/"+l.search) + "\n"
		height--
	}
	if !l.loaded {
		if l.loadErr != nil {
			return styleError.Render("  Could not load issues: "+m.redact(l.loadErr.Error())) + "\n" + styleMuted.Render(fmt.Sprintf("  %s to retry, %s to change the filter, %s to switch host", keyOf(actRefresh), keyOf(actFilter), keyOf(actSwitchHost)))
		}
		return styleMuted.Render("  Loading issues…")
	}
	if len(l.issues) == 0 {
		return styleMuted.Render(fmt.Sprintf("  No issues match the %q filter. %s changes the filter.", l.filter, keyOf(actFilter)))
	}
	if len(l.rows()) == 0 {
		return searchLine + styleMuted.Render("  No issues match the search. "+keyOf(actEscape)+" clears it.")
	}

	cols := columnsFor(width)
	const marker = 3 // cursor, selection and unread gutter
	fixed := marker
	for _, c := range cols {
		fixed += c.width + 1
	}
	summary := max(width-fixed, 10)

	var b strings.Builder
	b.WriteString(searchLine)
	header := strings.Repeat(" ", marker)
	for _, c := range cols {
		w := c.width
		if w == 0 {
			w = summary
		}
		title := c.title
		if c.key == l.sort.field {
			title = strings.TrimSpace(title + " " + l.sort.arrow())
		}
		header += fit(title, w, c.right) + " "
	}
	b.WriteString(styleHeader.Render(ansi.Truncate(header, width, "")) + "\n")

	rs := l.rows()
	visible := max(height-1, 1)
	for i := l.offset; i < len(rs) && i < l.offset+visible; i++ {
		if rs[i].idx < 0 {
			b.WriteString(styleGroup.Render(ansi.Truncate(fmt.Sprintf("── %s (%d) ", rs[i].header, rs[i].count), width, "")) + "\n")
			continue
		}
		is := l.issues[rs[i].idx]
		unread := l.unread(is)
		var cells []string
		plain := ""
		for _, c := range cols {
			w := c.width
			if w == 0 {
				w = summary
			}
			text := fit(c.cell(l, is), w, c.right)
			plain += text + " "
			if c.title == "SUMMARY" && unread && i != l.cursor {
				text = styleUnread.Render(text)
			}
			if c.title == "STATUS" && i != l.cursor {
				if hex := statusColor(is, l.colors); hex != "" {
					text = lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(text)
				}
			}
			cells = append(cells, text)
		}
		mark := " "
		if _, ok := l.selected[is.ID]; ok {
			mark = styleGroup.Render("●")
		}
		dot, dotPlain := " ", " "
		if unread {
			dot, dotPlain = styleUnreadDot.Render("•"), "•"
		}
		line := " " + mark + dot + strings.Join(cells, " ")
		if i == l.cursor {
			sel := " "
			if _, ok := l.selected[is.ID]; ok {
				sel = "●"
			}
			line = styleSelected.Bold(unread).Render(ansi.Truncate("▸"+sel+dotPlain+plain, width, ""))
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
