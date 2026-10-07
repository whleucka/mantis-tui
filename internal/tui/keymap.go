package tui

// action is a user command, independent of the key that triggers it.
type action string

const (
	actQuit           action = "quit"
	actHelp           action = "help"
	actSwitchHost     action = "switch-host"
	actRefresh        action = "refresh"
	actUp             action = "up"
	actDown           action = "down"
	actTop            action = "top"
	actBottom         action = "bottom"
	actPageDown       action = "page-down"
	actPageUp         action = "page-up"
	actOpen           action = "open"
	actBack           action = "back"
	actBrowser        action = "browser"
	actCreate         action = "create"
	actDelete         action = "delete"
	actAssign         action = "assign"
	actSummary        action = "summary"
	actStatus         action = "status"
	actSeverity       action = "severity"
	actPriority       action = "priority"
	actCategory       action = "category"
	actMonitor        action = "monitor"
	actFilter         action = "filter"
	actToggleGroup    action = "toggle-group"
	actNextPage       action = "next-page"
	actPrevPage       action = "prev-page"
	actAddNote        action = "add-note"
	actDeleteNote     action = "delete-note"
	actSearch         action = "search"
	actClearSearch    action = "clear-search"
	actNextTab        action = "next-tab"
	actToggleSelect   action = "toggle-select"
	actSelectAll      action = "select-all"
	actClearSelection action = "clear-selection"
	actBatchStatus    action = "batch-status"
	actBatchPriority  action = "batch-priority"
	actBatchSeverity  action = "batch-severity"
	actBatchCategory  action = "batch-category"
	actBatchAssign    action = "batch-assign"
	actBatchDelete    action = "batch-delete"
	actTogglePreview  action = "toggle-preview"
	actPreviewDown    action = "preview-down"
	actPreviewUp      action = "preview-up"
	actNextUnread     action = "next-unread"
	actToggleRead     action = "toggle-read"
	actMarkUnreadBack action = "mark-unread-back"
	actPalette        action = "palette"
)

// binding maps key sequences to an action. Each entry in keys is one
// sequence of space-separated key names as Bubble Tea reports them
// (e.g. "g g", "ctrl+a", "space").
type binding struct {
	action action
	keys   []string
	help   string
}

type keymap struct {
	list  []binding
	issue []binding
}

// defaultKeymap mirrors mantis.nvim's default keys.
func defaultKeymap() keymap {
	common := []binding{
		{actHelp, []string{"?"}, "toggle help"},
		{actPalette, []string{":", "ctrl+p"}, "command palette"},
		{actSwitchHost, []string{"ctrl+h"}, "switch host"},
		{actRefresh, []string{"r"}, "refresh"},
		{actUp, []string{"k", "up"}, "up"},
		{actDown, []string{"j", "down"}, "down"},
		{actBrowser, []string{"o"}, "open in browser"},
		{actAddNote, []string{"N"}, "add note"},
		{actAssign, []string{"a"}, "assign"},
		{actStatus, []string{"s"}, "change status"},
		{actPriority, []string{"p"}, "change priority"},
		{actSeverity, []string{"V"}, "change severity"},
		{actCategory, []string{"c"}, "change category"},
		{actSummary, []string{"S"}, "change summary"},
		{actMonitor, []string{"m"}, "toggle monitoring"},
	}
	list := append(append([]binding{}, common...),
		binding{actQuit, []string{"q"}, "quit"},
		binding{actOpen, []string{"enter", "l"}, "view issue"},
		binding{actTop, []string{"g g", "home"}, "first issue"},
		binding{actBottom, []string{"G", "end"}, "last issue"},
		binding{actCreate, []string{"C"}, "create issue"},
		binding{actDelete, []string{"D"}, "delete issue"},
		binding{actFilter, []string{"F"}, "filter"},
		binding{actSearch, []string{"/"}, "search this page"},
		binding{actClearSearch, []string{"esc"}, "clear search"},
		binding{actToggleGroup, []string{"ctrl+g"}, "group by project"},
		binding{actNextUnread, []string{"n"}, "next unread issue"},
		binding{actToggleRead, []string{"u"}, "toggle read / unread"},
		binding{actTogglePreview, []string{"P"}, "toggle preview pane"},
		binding{actPreviewDown, []string{"ctrl+d"}, "scroll preview down"},
		binding{actPreviewUp, []string{"ctrl+u"}, "scroll preview up"},
		binding{actNextPage, []string{"L"}, "next page"},
		binding{actPrevPage, []string{"H"}, "previous page"},
		binding{actToggleSelect, []string{"space"}, "toggle selection"},
		binding{actSelectAll, []string{"ctrl+a"}, "select page"},
		binding{actClearSelection, []string{"ctrl+x"}, "clear selection"},
		binding{actBatchStatus, []string{"b s"}, "batch status"},
		binding{actBatchPriority, []string{"b p"}, "batch priority"},
		binding{actBatchSeverity, []string{"b v"}, "batch severity"},
		binding{actBatchCategory, []string{"b c"}, "batch category"},
		binding{actBatchAssign, []string{"b a"}, "batch assign"},
		binding{actBatchDelete, []string{"b D"}, "batch delete"},
	)
	issue := append(append([]binding{}, common...),
		binding{actBack, []string{"q", "esc", "h"}, "back to list"},
		binding{actTop, []string{"g g"}, "top"},
		binding{actBottom, []string{"G"}, "bottom"},
		binding{actPageDown, []string{"ctrl+d", "pgdown"}, "page down"},
		binding{actPageUp, []string{"ctrl+u", "pgup"}, "page up"},
		binding{actNextTab, []string{"tab"}, "notes / history"},
		binding{actDeleteNote, []string{"d n"}, "delete note"},
		binding{actMarkUnreadBack, []string{"u"}, "mark unread, back to list"},
	)
	return keymap{list: list, issue: issue}
}
