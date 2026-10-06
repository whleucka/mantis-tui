// Package tui is the interactive terminal UI.
package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/whleucka/mantis-tui/internal/config"
)

// Options configure the TUI.
type Options struct {
	Config       *config.Config
	Hosts        []config.Host
	Initial      *config.Host // nil shows the host picker first
	NewSession   func(config.Host) *Session
	SaveLastHost func(name string) error
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
}

func (hv *hostView) context() string {
	if hv.screen == screenList {
		return hv.list.context()
	}
	return ""
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

	modal   modal
	chord   chord
	status  status
	spin    spinner.Model
	loading int
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
		opts:  opts,
		keys:  defaultKeymap(),
		hosts: map[string]*hostView{},
		spin:  spinner.New(spinner.WithSpinner(spinner.MiniDot)),
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
	}

	if m.cur != nil {
		return m, m.cur.list.handleMsg(m, msg)
	}
	return m, nil
}

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
		hv = &hostView{sess: m.opts.NewSession(*host), list: newListModel(m.opts.Config)}
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
	if m.cur != nil {
		body = m.cur.list.view(m, m.width, max(m.height-1, 1))
	}
	body = lipgloss.NewStyle().Width(m.width).Height(max(m.height-1, 1)).MaxHeight(max(m.height-1, 1)).Render(body)
	content := body + "\n" + m.statusView()

	if m.modal != nil {
		box := m.modal.view(m.width, m.height)
		content = overlay(content, box, m.width, m.height)
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
