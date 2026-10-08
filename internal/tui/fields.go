package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

// longField is a multi-line issue field that is edited in $EDITOR.
type longField struct {
	kind  string // part of the temp file name
	label string
	get   func(*mantis.Issue) string
	patch func(string) mantis.IssuePatch
}

var longFields = []longField{
	{"description", "Description",
		func(is *mantis.Issue) string { return is.Description },
		func(v string) mantis.IssuePatch { return mantis.IssuePatch{Description: &v} }},
	{"steps", "Steps to reproduce",
		func(is *mantis.Issue) string { return is.StepsToReproduce },
		func(v string) mantis.IssuePatch { return mantis.IssuePatch{StepsToReproduce: &v} }},
	{"additional", "Additional information",
		func(is *mantis.Issue) string { return is.AdditionalInformation },
		func(v string) mantis.IssuePatch { return mantis.IssuePatch{AdditionalInformation: &v} }},
}

// errFieldChanged means someone else saved the field while it was being edited.
var errFieldChanged = errors.New("it changed on the server while you were editing")

// fieldEditedMsg reports the PATCH of one long field.
type fieldEditedMsg struct {
	host  string
	id    int
	field longField
	sess  *editor.Session
	issue *mantis.Issue
	err   error
}

// pickEditField fetches the issue, then asks which field to edit: the
// summary in a prompt, or a long field in $EDITOR. Fetching first means the
// edit starts from the server's text rather than a stale list row.
func (m *Model) pickEditField(id int) tea.Cmd {
	host, api := m.cur.sess.Host.Name, m.cur.sess.API
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		res, err := api.GetIssue(ctx, id)
		if err != nil {
			return errMsg{host: host, err: err}
		}
		is := res.Issue
		opts := []pickerOption{{label: "Summary", detail: is.Summary}}
		for _, f := range longFields {
			first, _, _ := strings.Cut(strings.TrimSpace(f.get(&is)), "\n")
			if first == "" {
				first = "(empty)"
			}
			opts = append(opts, pickerOption{label: f.label, detail: first, value: f})
		}
		return modalReadyMsg{host: host, modal: newPicker(fmt.Sprintf("Edit which field of #%d?", id), opts, func(o pickerOption) tea.Cmd {
			if f, ok := o.value.(longField); ok {
				return m.editLongField(is, f)
			}
			return func() tea.Msg { return modalReadyMsg{host: host, modal: m.summaryPrompt(is)} }
		})}
	})
}

// editLongField opens $EDITOR on f's text and sends the result, unless the
// field changed on the server in the meantime.
func (m *Model) editLongField(is mantis.Issue, f longField) tea.Cmd {
	host, id, initial := m.cur.sess.Host.Name, is.ID, f.get(&is)
	name := strings.ToLower(f.label)
	return m.runEditor(editor.Request{
		Host: host, IssueID: id, Kind: f.kind, Initial: initial,
		Hints: []string{
			fmt.Sprintf("%s of issue #%d on %s.", f.label, id, host),
			"Lines starting with '# ' like these are removed. Save it unchanged or empty to cancel.",
		},
	}, func(m *Model, s *editor.Session, text string, err error) tea.Cmd {
		if errors.Is(err, editor.ErrEmpty) {
			s.Cleanup()
			return infoCmd(host, fmt.Sprintf("#%d %s unchanged", id, name))
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
			msg := fieldEditedMsg{host: host, id: id, field: f, sess: s}
			res, err := api.GetIssue(ctx, id)
			switch {
			case err != nil:
				msg.err = err
			case strings.TrimSpace(f.get(&res.Issue)) != strings.TrimSpace(initial):
				msg.err = errFieldChanged
			default:
				msg.issue, msg.err = api.UpdateIssue(ctx, id, f.patch(text))
			}
			return msg
		})
	})
}

func (m *Model) onFieldEdited(msg fieldEditedMsg) tea.Cmd {
	name := strings.ToLower(msg.field.label)
	if msg.err != nil {
		return errCmd(msg.host, fmt.Errorf("#%d %s not saved: %w; your text is in %s", msg.id, name, msg.err, msg.sess.Path))
	}
	msg.sess.Cleanup()
	hv := m.hosts[msg.host]
	if hv == nil {
		return nil
	}
	hv.list.pv.forget(msg.id)
	return hv.handleMsg(m, patchedMsg{
		host:    msg.host,
		label:   name + " updated",
		results: []service.Result{{ID: msg.id}},
		updated: map[int]*mantis.Issue{msg.id: msg.issue},
	})
}
