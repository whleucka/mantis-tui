package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

type (
	// editorDoneMsg arrives when $EDITOR exits; then decides what to do
	// with the text.
	editorDoneMsg struct {
		host string
		sess *editor.Session
		err  error
		then func(m *Model, s *editor.Session, text string, err error) tea.Cmd
	}
	noteAddedMsg struct {
		host    string
		issueID int
		note    *mantis.Note
		sess    *editor.Session
		err     error
	}
	noteDeletedMsg struct {
		host    string
		issueID int
		noteID  int
		err     error
	}
)

// defaultExecEditor suspends the TUI and runs the editor on the session file.
func defaultExecEditor(s *editor.Session, done func(error) tea.Msg) tea.Cmd {
	return tea.ExecProcess(s.Cmd(), func(err error) tea.Msg { return done(err) })
}

// startNote asks for the note options, then opens $EDITOR.
func (m *Model) startNote(is mantis.Issue) tea.Cmd {
	host, cache := m.cur.sess.Host.Name, m.cur.sess.Meta
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		enabled, err := cache.TimeTrackingEnabled(ctx)
		if err != nil {
			return errMsg{host: host, err: err}
		}
		return modalReadyMsg{host: host, modal: newNoteForm(is.ID, enabled, m.width, func(private bool, dur string) tea.Cmd {
			return m.openEditor(is.ID, private, dur)
		})}
	})
}

func (m *Model) openEditor(issueID int, private bool, dur string) tea.Cmd {
	host := m.cur.sess.Host.Name
	return m.runEditor(editor.Request{
		Host: host, IssueID: issueID, Kind: "note",
		Hints: []string{
			fmt.Sprintf("Note for issue #%d on %s.", issueID, host),
			"Lines starting with '# ' like these are removed. Save an empty file to cancel.",
		},
	}, func(m *Model, s *editor.Session, text string, err error) tea.Cmd {
		if errors.Is(err, editor.ErrEmpty) {
			s.Cleanup()
			return infoCmd(host, "note discarded")
		}
		if err != nil {
			return errCmd(host, err)
		}
		hv := m.hosts[host]
		if hv == nil {
			return nil
		}
		api := hv.sess.API
		return m.call(func(ctx context.Context) tea.Msg {
			ctx, cancel := timed(ctx)
			defer cancel()
			n, err := api.AddNote(ctx, issueID, mantis.NewNote{Text: text, Private: private, TimeTracking: dur})
			return noteAddedMsg{host: host, issueID: issueID, note: n, sess: s, err: err}
		})
	})
}

// runEditor writes req to a temp file, hands the terminal to $EDITOR, and
// calls then with the edited text once it exits.
func (m *Model) runEditor(req editor.Request, then func(*Model, *editor.Session, string, error) tea.Cmd) tea.Cmd {
	host := m.cur.sess.Host.Name
	s, err := editor.Prepare(req)
	if err != nil {
		return errCmd(host, err)
	}
	m.editing = true
	return m.execEditor(s, func(err error) tea.Msg {
		return editorDoneMsg{host: host, sess: s, err: err, then: then}
	})
}

func (m *Model) onEditorDone(msg editorDoneMsg) tea.Cmd {
	m.editing = false
	if msg.err != nil {
		msg.sess.Cleanup()
		return errCmd(msg.host, fmt.Errorf("editor: %w", msg.err))
	}
	text, err := msg.sess.Text()
	return msg.then(m, msg.sess, text, err)
}

func (m *Model) onNoteAdded(msg noteAddedMsg) tea.Cmd {
	if msg.err != nil {
		return errCmd(msg.host, fmt.Errorf("note not sent (%w); your text is saved in %s", msg.err, msg.sess.Path))
	}
	msg.sess.Cleanup()
	return tea.Batch(infoCmd(msg.host, fmt.Sprintf("#%d note %d added", msg.issueID, msg.note.ID)),
		m.markSeenFromServer(msg.host, msg.issueID), m.reloadIssue(msg.host, msg.issueID))
}

// reloadIssue silently refreshes the issue view if it shows issueID, and
// drops the preview's copy so the preview fetches it again.
func (m *Model) reloadIssue(host string, issueID int) tea.Cmd {
	hv := m.hosts[host]
	if hv == nil {
		return nil
	}
	hv.list.pv.forget(issueID)
	if hv.issue != nil && hv.issue.id == issueID {
		return hv.issue.fetch(true)
	}
	return nil
}

// pickNoteToDelete lists the issue's notes, then confirms the deletion.
func (m *Model) pickNoteToDelete(is *mantis.Issue) tea.Cmd {
	host := m.cur.sess.Host.Name
	if is == nil || len(is.Notes) == 0 {
		return infoCmd(host, "this issue has no notes")
	}
	opts := make([]pickerOption, len(is.Notes))
	for i, n := range is.Notes {
		first, _, _ := strings.Cut(n.Text, "\n")
		opts[i] = pickerOption{label: fmt.Sprintf("note %d · %s", n.ID, n.Reporter.Display()), detail: first, value: n.ID}
	}
	api, issueID := m.cur.sess.API, is.ID
	m.modal = newPicker("Delete which note?", opts, func(o pickerOption) tea.Cmd {
		noteID := o.value.(int)
		confirm := &confirmModal{prompt: fmt.Sprintf("Delete note %d from #%d?", noteID, issueID), onYes: func() tea.Cmd {
			return m.call(func(ctx context.Context) tea.Msg {
				ctx, cancel := timed(ctx)
				defer cancel()
				return noteDeletedMsg{host: host, issueID: issueID, noteID: noteID, err: api.DeleteNote(ctx, issueID, noteID)}
			})
		}}
		return func() tea.Msg { return modalReadyMsg{host: host, modal: confirm} }
	})
	return nil
}

func (m *Model) onNoteDeleted(msg noteDeletedMsg) tea.Cmd {
	if msg.err != nil {
		return errCmd(msg.host, msg.err)
	}
	return tea.Batch(infoCmd(msg.host, fmt.Sprintf("#%d note %d deleted", msg.issueID, msg.noteID)), m.reloadIssue(msg.host, msg.issueID))
}

// noteForm collects the private flag and optional time before the editor opens.
type noteForm struct {
	issueID     int
	timeEnabled bool
	private     bool
	time        textinput.Model
	focus       int // 0 private, 1 time
	errText     string
	onSubmit    func(private bool, dur string) tea.Cmd
}

func newNoteForm(issueID int, timeEnabled bool, width int, onSubmit func(bool, string) tea.Cmd) *noteForm {
	in := textinput.New()
	in.Placeholder = "H:MM"
	in.SetWidth(min(max(width/4, 8), 12))
	return &noteForm{issueID: issueID, timeEnabled: timeEnabled, time: in, onSubmit: onSubmit}
}

func (f *noteForm) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		return nil, true
	case "enter":
		dur := strings.TrimSpace(f.time.Value())
		if dur != "" && !service.ValidDuration(dur) {
			f.errText = "time spent must look like H:MM, e.g. 0:30"
			return nil, false
		}
		return f.onSubmit(f.private, dur), true
	case "tab", "shift+tab", "up", "down":
		if f.timeEnabled {
			f.focus = 1 - f.focus
			if f.focus == 1 {
				return f.time.Focus(), false
			}
			f.time.Blur()
		}
		return nil, false
	}
	if f.focus == 0 {
		if msg.String() == "space" || msg.String() == "p" {
			f.private = !f.private
		}
		return nil, false
	}
	var cmd tea.Cmd
	f.time, cmd = f.time.Update(msg)
	f.errText = ""
	return cmd, false
}

func (f *noteForm) view(width, _ int) string {
	box := "[ ]"
	if f.private {
		box = "[x]"
	}
	private := box + " private"
	if f.focus == 0 {
		private = styleSelected.Render(private)
	}
	body := styleTitle.Render(fmt.Sprintf("Add note to #%d", f.issueID)) + "\n\n" + private
	if f.timeEnabled {
		body += "\nTime spent " + f.time.View()
	}
	if f.errText != "" {
		body += "\n" + styleError.Render(f.errText)
	}
	help := "space toggle · enter open $EDITOR · esc cancel"
	if f.timeEnabled {
		help = "space toggle · tab time · enter open $EDITOR · esc cancel"
	}
	body += "\n\n" + styleMuted.Render(help)
	return styleModal.Width(min(max(width/2, 44), width-2)).Render(body)
}
