// Package tui is the interactive terminal UI.
package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/editor"
)

// Options configure the TUI.
type Options struct {
	Config       *config.Config
	Hosts        []config.Host
	Initial      *config.Host // nil shows the host picker first
	NewSession   func(config.Host) *Session
	SaveLastHost func(name string) error
	OpenURL      func(url string) error
}

// screen is what fills the main area for the current host.
type screen int

const (
	screenList screen = iota
	screenIssue
	screenCreate
)

// hostView is the per-host UI state, kept while switching hosts.
type hostView struct {
	sess   *Session
	screen screen
	list   *listModel
	issue  *issueModel // set while screen == screenIssue
	create *createForm // set while screen == screenCreate
}

func (hv *hostView) context() string {
	if hv.screen == screenIssue && hv.issue != nil {
		return hv.issue.context()
	}
	if hv.screen == screenCreate && hv.create != nil {
		return hv.create.context()
	}
	return hv.list.context()
}

// handleMsg routes a host-scoped result to the part of the UI that asked.
func (hv *hostView) handleMsg(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case issueLoadedMsg:
		if hv.issue == nil {
			if !msg.silent {
				m.stopLoading()
			}
			return nil
		}
		return hv.issue.handleMsg(m, msg)
	case patchedMsg:
		if msg.batch == m.batchID {
			m.batchID = 0
		}
		cmds := []tea.Cmd{hv.list.applyPatched(m, msg)}
		if hv.issue != nil {
			if _, ok := msg.updated[hv.issue.id]; ok {
				cmds = append(cmds, hv.issue.fetch(true))
			}
		}
		return tea.Batch(cmds...)
	case deletedMsg:
		return hv.list.removeDeleted(m, msg)
	case createSetMsg:
		if hv.create != nil {
			return hv.create.set(m, msg)
		}
		return nil
	case createRevalidateMsg:
		if hv.create != nil {
			return hv.create.revalidate(msg)
		}
		return nil
	case createdMsg:
		if hv.create == nil {
			return nil
		}
		if msg.err != nil {
			hv.create.errText = m.redact(msg.err.Error())
			return nil
		}
		hv.create.close(m)
		if m.cur != hv {
			return hv.list.fetch(hv.list.page, msg.issue.ID, true)
		}
		return tea.Batch(hv.list.fetch(hv.list.page, msg.issue.ID, true), m.openIssue(msg.issue.ID),
			infoCmd(hv.sess.Host.Name, fmt.Sprintf("#%d created", msg.issue.ID)))
	case refreshTickMsg:
		if msg.target == "issue" {
			if hv.issue == nil {
				return nil
			}
			return hv.issue.onTick(m, msg)
		}
		return hv.list.onTick(m, msg)
	}
	return hv.list.handleMsg(m, msg)
}

// modal is an overlay that captures keys until it closes.
type modal interface {
	update(tea.KeyPressMsg) (cmd tea.Cmd, done bool)
	view(width, height int) string
}

// Model is the root Bubble Tea model.
type Model struct {
	opts   Options
	keys   keymap
	width  int
	height int

	hosts map[string]*hostView
	cur   *hostView

	modal      modal
	editing    bool // an external $EDITOR owns the terminal
	execEditor func(*editor.Session, func(error) tea.Msg) tea.Cmd
	chord      chord
	status     status
	spin       spinner.Model
	loading    int

	batchSeq int // last batch started
	batchID  int // batch whose progress is shown; 0 when none
}

// Messages. Anything tied to a host carries its name so responses that
// arrive after a host switch are dropped.
type (
	hostChosenMsg struct{ name string }
	errMsg        struct {
		host string
		err  error
	}
	infoMsg struct {
		host string
		text string
	}
	saveHostFailedMsg struct{ err error }
)

// New builds the root model.
func New(opts Options) *Model {
	return &Model{
		opts:       opts,
		keys:       defaultKeymap(),
		hosts:      map[string]*hostView{},
		spin:       spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		execEditor: defaultExecEditor,
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	if m.opts.Initial != nil {
		return m.selectHost(m.opts.Initial.Name)
	}
	m.modal = m.hostPicker()
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.cur != nil && m.cur.issue != nil {
			m.cur.issue.render(m)
		}
		return m, nil

	case tea.KeyPressMsg:
		return m, m.handleKey(msg)

	case chordTimeoutMsg:
		m.chord.timeout(msg.gen)
		return m, nil

	case spinner.TickMsg:
		if m.loading == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case hostChosenMsg:
		return m, m.selectHost(msg.name)

	case doneMsg:
		m.stopLoading()
		if msg.inner == nil {
			return m, nil
		}
		return m.Update(msg.inner)

	case editorDoneMsg:
		return m, m.onEditorDone(msg)
	case noteAddedMsg:
		return m, m.onNoteAdded(msg)
	case noteDeletedMsg:
		return m, m.onNoteDeleted(msg)

	case batchProgressMsg:
		if msg.batch != m.batchID || !m.isCurrent(msg.host) {
			return m, nil
		}
		m.status.info(fmt.Sprintf("%s… %d/%d", msg.label, msg.done, msg.total))
		return m, msg.next

	case modalReadyMsg:
		if m.isCurrent(msg.host) && m.modal == nil {
			m.modal = msg.modal
		}
		return m, nil

	case saveHostFailedMsg:
		m.status.err("could not remember last host: " + msg.err.Error())
		return m, nil

	case errMsg:
		if m.isCurrent(msg.host) {
			m.status.err(m.redact(msg.err.Error()))
		}
		return m, nil

	case infoMsg:
		if m.isCurrent(msg.host) {
			m.status.info(msg.text)
		}
		return m, nil

	case filterChosenMsg:
		if hv := m.hosts[msg.host]; hv != nil {
			return m, hv.list.setFilter(m, msg.filter)
		}
		return m, nil
	}

	// Host-scoped results go to the host that asked for them, even if the
	// user has switched away since.
	if hm, ok := msg.(interface{ hostName() string }); ok {
		if hv := m.hosts[hm.hostName()]; hv != nil {
			return m, hv.handleMsg(m, msg)
		}
	}
	return m, nil
}

// busy reports that the user is in the middle of something an auto-refresh
// must not disturb.
func (m *Model) busy() bool { return m.modal != nil || m.editing }

func (m *Model) isCurrent(host string) bool { return m.cur != nil && m.cur.sess.Host.Name == host }

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}
	if m.modal != nil {
		cmd, done := m.modal.update(msg)
		if done {
			m.modal = nil
			if m.cur == nil && cmd == nil {
				return tea.Quit // closed the startup host picker without choosing
			}
		}
		return cmd
	}
	if m.cur == nil {
		return nil
	}
	m.status.clear()
	if m.cur.screen == screenCreate && m.cur.create != nil {
		return m.cur.create.update(m, msg) // the form takes raw keys (typing)
	}

	a, waiting := m.chord.feed(msg.String(), m.bindings())
	if waiting {
		return m.chord.waitCmd()
	}
	switch a {
	case "":
		return nil
	case actSwitchHost:
		m.modal = m.hostPicker()
		return nil
	}
	if m.cur.screen == screenIssue && m.cur.issue != nil {
		return m.cur.issue.handleAction(m, a)
	}
	return m.cur.list.handleAction(m, a)
}

func (m *Model) bindings() []binding {
	if m.cur != nil && m.cur.screen == screenIssue {
		return m.keys.issue
	}
	return m.keys.list
}

func (m *Model) hostPicker() *picker {
	opts := make([]pickerOption, len(m.opts.Hosts))
	for i, h := range m.opts.Hosts {
		opts[i] = pickerOption{label: h.Name, detail: h.URL, value: h.Name, current: m.cur != nil && m.cur.sess.Host.Name == h.Name}
	}
	return newPicker("Select a host", opts, func(o pickerOption) tea.Cmd {
		name := o.value.(string)
		return func() tea.Msg { return hostChosenMsg{name: name} }
	})
}

// selectHost makes name current, creating its state on first use, and
// remembers it as the last-used host.
func (m *Model) selectHost(name string) tea.Cmd {
	var host *config.Host
	for i := range m.opts.Hosts {
		if m.opts.Hosts[i].Name == name {
			host = &m.opts.Hosts[i]
		}
	}
	if host == nil {
		m.status.err(fmt.Sprintf("unknown host %q", name))
		return nil
	}

	var cmds []tea.Cmd
	hv, ok := m.hosts[name]
	if !ok {
		sess := m.opts.NewSession(*host)
		hv = &hostView{sess: sess, list: newListModel(m.opts.Config, sess)}
		m.hosts[name] = hv
		m.cur = hv
		cmds = append(cmds, hv.list.init(m))
	}
	m.cur = hv
	m.chord = chord{}
	m.status.clear()

	if save := m.opts.SaveLastHost; save != nil {
		cmds = append(cmds, func() tea.Msg {
			if err := save(name); err != nil {
				return saveHostFailedMsg{err}
			}
			return nil
		})
	}
	return tea.Batch(cmds...)
}

// startLoading shows the spinner until the matching stopLoading.
func (m *Model) startLoading() tea.Cmd {
	m.loading++
	if m.loading == 1 {
		return m.spin.Tick
	}
	return nil
}

func (m *Model) stopLoading() {
	if m.loading > 0 {
		m.loading--
	}
}

// openIssue switches the current host to the issue view for id.
func (m *Model) openIssue(id int) tea.Cmd {
	iv := newIssueModel(m.cur.sess, id, m.opts.Config.Issue.AutoRefresh.Duration)
	m.cur.issue, m.cur.screen = iv, screenIssue
	iv.render(m)
	return tea.Batch(iv.load(m), iv.scheduleRefresh())
}

// redact removes every configured token from text shown on screen.
func (m *Model) redact(s string) string {
	for _, h := range m.opts.Hosts {
		if h.Token != "" {
			s = strings.ReplaceAll(s, h.Token, "<redacted>")
		}
	}
	return s
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	var body string
	switch {
	case m.cur == nil:
	case m.cur.screen == screenIssue && m.cur.issue != nil:
		body = m.cur.issue.view()
	case m.cur.screen == screenCreate && m.cur.create != nil:
		body = m.cur.create.view(m.width, max(m.height-1, 1))
	default:
		body = m.cur.list.view(m, m.width, max(m.height-1, 1))
	}
	body = lipgloss.NewStyle().Width(m.width).Height(max(m.height-1, 1)).MaxHeight(max(m.height-1, 1)).Render(body)
	content := body + "\n" + m.statusView()

	if m.modal != nil {
		if box := m.modal.view(m.width, m.height); box != "" {
			content = overlay(content, box, m.width, m.height)
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "mantis-tui"
	return v
}

// overlay centers box on top of base.
func overlay(base, box string, width, height int) string {
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	x, y := max((width-bw)/2, 0), max((height-bh)/3, 0)
	baseLayer := lipgloss.NewLayer(base)
	boxLayer := lipgloss.NewLayer(box).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(baseLayer, boxLayer).Render()
}
