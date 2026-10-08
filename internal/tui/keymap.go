package tui

// action is a user command, independent of the key that triggers it.
type action string

const (
	actQuit           action = "quit"
	actHelp           action = "help"
	actPalette        action = "palette"
	actSwitchHost     action = "switch-host"
	actJumpHost       action = "jump-host"
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
	actCopyURL        action = "copy-url"
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
	actSort           action = "sort"
	actToggleGroup    action = "toggle-group"
	actNextIssue      action = "next-issue"
	actPrevIssue      action = "prev-issue"
	actAddNote        action = "add-note"
	actDeleteNote     action = "delete-note"
	actSearch         action = "search"
	actEscape         action = "escape"
	actNextTab        action = "next-tab"
	actToggleSelect   action = "toggle-select"
	actSelectAll      action = "select-all"
	actTogglePreview  action = "toggle-preview"
	actPreviewDown    action = "preview-down"
	actPreviewUp      action = "preview-up"
	actNextUnread     action = "next-unread"
	actPrevUnread     action = "prev-unread"
	actToggleRead     action = "toggle-read"
	actMarkUnreadBack action = "mark-unread-back"
	actAsk            action = "ask"
)

// binding maps key sequences to an action. Each entry in keys is one
// sequence of space-separated key names as Bubble Tea reports them
// (e.g. "g g", "ctrl+a", "space").
type binding struct {
	action action
	keys   []string
	help   string
}

// keyOf is the first key bound to a, for hints in the UI, so they cannot
// drift from the keymap.
func keyOf(a action) string {
	km := defaultKeymap()
	for _, b := range append(km.list, km.issue...) {
		if b.action == a {
			return keyLabel(b.keys[:1])
		}
	}
	return "?"
}

type keymap struct {
	list  []binding
	issue []binding
}

// defaultKeymap is the TUI's keymap: vim-style movement, single-letter
// actions that follow the selection, and no keys that terminals confuse
// (ctrl+h is Backspace in many of them).
func defaultKeymap() keymap {
	common := []binding{
		{actHelp, []string{"?"}, "toggle help"},
		{actPalette, []string{":", "ctrl+p"}, "command palette"},
		{actSwitchHost, []string{"H"}, "switch host"},
		{actJumpHost, []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, "switch to host N"},
		{actRefresh, []string{"R"}, "refresh"},
		{actUp, []string{"k", "up"}, "up"},
		{actDown, []string{"j", "down"}, "down"},
		{actBrowser, []string{"o"}, "open in browser"},
		{actCopyURL, []string{"y"}, "copy issue URL"},
		{actAddNote, []string{"r"}, "add note (reply)"},
		{actStatus, []string{"s"}, "change status"},
		{actPriority, []string{"p"}, "change priority"},
		{actSeverity, []string{"v"}, "change severity"},
		{actCategory, []string{"c"}, "change category"},
		{actAssign, []string{"a"}, "assign"},
		{actSummary, []string{"e"}, "edit summary"},
		{actMonitor, []string{"m"}, "toggle monitoring"},
		{actAsk, []string{"A"}, "ask Claude in a new pane"},
	}
	list := append(append([]binding{}, common...),
		binding{actQuit, []string{"q"}, "quit"},
		binding{actOpen, []string{"enter", "l"}, "view issue"},
		binding{actTop, []string{"g g", "home"}, "first issue"},
		binding{actBottom, []string{"G", "end"}, "last issue"},
		binding{actPageDown, []string{"ctrl+d", "pgdown"}, "half page down"},
		binding{actPageUp, []string{"ctrl+u", "pgup"}, "half page up"},
		binding{actFilter, []string{"f"}, "filter"},
		binding{actSort, []string{"S"}, "sort"},
		binding{actSearch, []string{"/"}, "search the list"},
		binding{actEscape, []string{"esc"}, "clear search, then selection"},
		binding{actNextUnread, []string{"n"}, "next unread issue"},
		binding{actPrevUnread, []string{"N"}, "previous unread issue"},
		binding{actToggleRead, []string{"u"}, "toggle read / unread"},
		binding{actToggleSelect, []string{"space"}, "toggle selection"},
		binding{actSelectAll, []string{"ctrl+a"}, "select all shown"},
		binding{actCreate, []string{"C"}, "create issue"},
		binding{actDelete, []string{"D"}, "delete issue(s)"},
		binding{actTogglePreview, []string{"P"}, "toggle preview pane"},
		binding{actPreviewDown, []string{"J"}, "scroll preview down"},
		binding{actPreviewUp, []string{"K"}, "scroll preview up"},
		binding{actToggleGroup, []string{"ctrl+g"}, "group by project"},
	)
	issue := append(append([]binding{}, common...),
		binding{actBack, []string{"q", "esc", "h"}, "back to list"},
		binding{actTop, []string{"g g"}, "top"},
		binding{actBottom, []string{"G"}, "bottom"},
		binding{actPageDown, []string{"ctrl+d", "pgdown"}, "half page down"},
		binding{actPageUp, []string{"ctrl+u", "pgup"}, "half page up"},
		binding{actNextIssue, []string{"]"}, "next issue"},
		binding{actPrevIssue, []string{"["}, "previous issue"},
		binding{actNextTab, []string{"tab"}, "notes / history"},
		binding{actDeleteNote, []string{"d n"}, "delete note"},
		binding{actMarkUnreadBack, []string{"u"}, "mark unread, back to list"},
	)
	return keymap{list: list, issue: issue}
}
