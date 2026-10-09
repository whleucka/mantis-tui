package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
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
		files   int
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

// startNote asks for the note options and attachments, then opens $EDITOR.
func (m *Model) startNote(is mantis.Issue) tea.Cmd {
	host, cache, clipboard := m.cur.sess.Host.Name, m.cur.sess.Meta, m.opts.Clipboard
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		enabled, err := cache.TimeTrackingEnabled(ctx)
		if err != nil {
			return errMsg{host: host, err: err}
		}
		limits, err := cache.Uploads(ctx)
		if err != nil {
			return errMsg{host: host, err: err}
		}
		f := newNoteForm(is.ID, enabled, limits, m.width, func(private bool, dur string, files []mantis.FileUpload) tea.Cmd {
			return m.openEditor(is.ID, private, dur, files)
		})
		if clipboard != nil {
			f.attach.setClip(clipboard(ctx))
		}
		return modalReadyMsg{host: host, modal: f}
	})
}

func (m *Model) openEditor(issueID int, private bool, dur string, files []mantis.FileUpload) tea.Cmd {
	host := m.cur.sess.Host.Name
	hints := []string{
		fmt.Sprintf("Note for issue #%d on %s.", issueID, host),
		"Lines starting with '# ' like these are removed. Save an empty file to cancel.",
	}
	if len(files) > 0 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.Name
		}
		hints = append(hints, "Attaching "+strings.Join(names, ", ")+". An empty note with them asks before sending.")
	}
	return m.runEditor(editor.Request{Host: host, IssueID: issueID, Kind: "note", Hints: hints},
		func(m *Model, s *editor.Session, text string, err error) tea.Cmd {
			send := func() tea.Cmd {
				return m.sendNote(host, issueID, s, mantis.NewNote{Text: text, Private: private, TimeTracking: dur, Files: files})
			}
			switch {
			case errors.Is(err, editor.ErrEmpty) && len(files) > 0:
				text = ""
				confirm := &confirmModal{prompt: fmt.Sprintf("Send %s without text?", plural(len(files), "attachment")), onYes: send,
					onNo: func() tea.Cmd { s.Cleanup(); return infoCmd(host, "note discarded") }}
				return func() tea.Msg { return modalReadyMsg{host: host, modal: confirm} }
			case errors.Is(err, editor.ErrEmpty):
				s.Cleanup()
				return infoCmd(host, "note discarded")
			case err != nil:
				return errCmd(host, err)
			}
			return send()
		})
}

func (m *Model) sendNote(host string, issueID int, s *editor.Session, n mantis.NewNote) tea.Cmd {
	hv := m.hosts[host]
	if hv == nil {
		return nil
	}
	api := hv.sess.API
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, uploadTimeout(len(n.Files)))
		defer cancel()
		note, err := api.AddNote(ctx, issueID, n)
		return noteAddedMsg{host: host, issueID: issueID, note: note, sess: s, files: len(n.Files), err: err}
	})
}

// uploadTimeout gives uploads longer than an ordinary request.
func uploadTimeout(files int) time.Duration {
	if files > 0 {
		return fileTimeout
	}
	return requestTimeout
}

// plural is "1 attachment" or "3 attachments".
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
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
	added := msg.note != nil && msg.note.ID != 0
	if msg.err != nil && !added {
		return errCmd(msg.host, fmt.Errorf("note not sent (%w); your text is saved in %s", msg.err, msg.sess.Path))
	}
	msg.sess.Cleanup()
	status := infoCmd(msg.host, fmt.Sprintf("#%d note %d added%s", msg.issueID, msg.note.ID, withFiles(msg.files)))
	if msg.err != nil { // added, but not all of its files
		status = errCmd(msg.host, msg.err)
	}
	return tea.Batch(status, m.markSeenFromServer(msg.host, msg.issueID), m.reloadIssue(msg.host, msg.issueID))
}

// withFiles is " with 2 files" for a status line, or nothing.
func withFiles(n int) string {
	if n == 0 {
		return ""
	}
	return " with " + plural(n, "file")
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
		var first string
		for line := range strings.SplitSeq(n.Text, "\n") {
			if first = stripPre(line); first != "" {
				break
			}
		}
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

// noteForm collects the private flag, optional time and attachments
// before the editor opens.
type noteForm struct {
	issueID     int
	timeEnabled bool
	private     bool
	time        textinput.Model
	attach      *attachSection
	limits      meta.UploadLimits
	focus       int // index into rows()
	errText     string
	onSubmit    func(private bool, dur string, files []mantis.FileUpload) tea.Cmd
}

func newNoteForm(issueID int, timeEnabled bool, limits meta.UploadLimits, width int, onSubmit func(bool, string, []mantis.FileUpload) tea.Cmd) *noteForm {
	in := textinput.New()
	in.Placeholder = "H:MM"
	in.SetWidth(min(max(width/4, 8), 12))
	return &noteForm{issueID: issueID, timeEnabled: timeEnabled, time: in, attach: newAttachSection(width), limits: limits, onSubmit: onSubmit}
}

// rows are the focusable rows: private, time, then the attachments.
func (f *noteForm) rows() []string {
	rows := []string{"private"}
	if f.timeEnabled {
		rows = append(rows, "time")
	}
	return append(rows, f.attach.rows()...)
}

func (f *noteForm) row() string {
	rows := f.rows()
	return rows[min(f.focus, len(rows)-1)]
}

// opened starts the clipboard thumbnail when the form appears.
func (f *noteForm) opened(m *Model) tea.Cmd { return m.clipThumb(f.attach) }

func (f *noteForm) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	row := f.row()
	if row == rowClip || row == rowFiles {
		if cmd, ok := f.attach.key(row, msg); ok {
			f.errText = ""
			return cmd, false
		}
	}
	switch msg.String() {
	case "esc":
		return nil, true
	case "enter":
		dur := strings.TrimSpace(f.time.Value())
		if dur != "" && !service.ValidDuration(dur) {
			f.errText = "time spent must look like H:MM, e.g. 0:30"
			return nil, false
		}
		clip, paths := f.attach.selection()
		files, err := gatherUploads(clip, paths, f.limits)
		if err != nil {
			f.errText = err.Error()
			return nil, false
		}
		return f.onSubmit(f.private, dur, files), true
	case "tab", "down":
		return f.move(1), false
	case "shift+tab", "up":
		return f.move(-1), false
	}
	switch row {
	case "private":
		if msg.String() == "space" || msg.String() == "p" {
			f.private = !f.private
		}
	case "time":
		var cmd tea.Cmd
		f.time, cmd = f.time.Update(msg)
		f.errText = ""
		return cmd, false
	}
	return nil, false
}

func (f *noteForm) move(delta int) tea.Cmd {
	rows := f.rows()
	f.focus = (f.focus + delta + len(rows)) % len(rows)
	f.time.Blur()
	switch row := f.row(); row {
	case "time":
		f.attach.focus("")
		return f.time.Focus()
	default:
		return f.attach.focus(row)
	}
}

func (f *noteForm) view(width, _ int) string {
	row := f.row()
	box := "[ ]"
	if f.private {
		box = "[x]"
	}
	private := box + " private"
	if row == "private" {
		private = styleSelected.Render(private)
	}
	body := styleTitle.Render(fmt.Sprintf("Add note to #%d", f.issueID)) + "\n\n" + private
	if f.timeEnabled {
		label := "Time spent"
		if row == "time" {
			label = styleSelected.Render(label)
		}
		body += "\n" + label + " " + f.time.View()
	}
	body += "\n\n" + f.attach.view(row)
	if f.errText != "" {
		body += "\n" + styleError.Render(f.errText)
	}
	body += "\n\n" + styleMuted.Render("↑↓ move · space toggle · tab completes a path · enter open $EDITOR · esc cancel")
	return styleModal.Width(min(max(width*2/3, 50), width-2)).Render(body)
}
