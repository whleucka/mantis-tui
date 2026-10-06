package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

// formFields are the create form's fields, in focus order.
var formFields = []string{"project", "category", "summary", "priority", "severity", "reproducibility", "assignee", "description"}

var fieldLabels = map[string]string{
	"project": "Project", "category": "Category", "summary": "Summary", "priority": "Priority",
	"severity": "Severity", "reproducibility": "Reproducibility", "assignee": "Assignee", "description": "Description",
}

// createForm builds a new issue. Values hold names (as the CLI takes them);
// labels hold what is shown when that differs (e.g. a user's real name).
type createForm struct {
	sess      *Session
	values    map[string]string
	labels    map[string]string
	projectID int
	focus     int
	summary   textinput.Model
	errText   string
	dirty     bool
}

type (
	createSetMsg struct {
		host         string
		field, value string
		label        string
		id           int // project id when field == "project"
	}
	createRevalidateMsg struct {
		host       string
		projectID  int
		categories []string
		users      []string
		err        error
	}
	createdMsg struct {
		host  string
		issue *mantis.Issue
		err   error
	}
)

func (msg createSetMsg) hostName() string        { return msg.host }
func (msg createRevalidateMsg) hostName() string { return msg.host }
func (msg createdMsg) hostName() string          { return msg.host }

// openCreate starts the form, preset to the project of the issue under the cursor.
func (m *Model) openCreate() tea.Cmd {
	f := &createForm{sess: m.cur.sess, values: map[string]string{}, labels: map[string]string{}}
	if is := m.currentIssue(); is != nil && is.Project.ID != 0 {
		f.values["project"], f.projectID = is.Project.Name, is.Project.ID
	}
	f.summary = textinput.New()
	f.summary.Placeholder = "one-line summary"
	f.summary.SetWidth(max(m.width-26, 20))
	m.cur.create, m.cur.screen = f, screenCreate
	return nil
}

func (f *createForm) field() string { return formFields[f.focus] }

func (f *createForm) update(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "alt+enter", "ctrl+s":
		return f.submit(m)
	case "esc":
		if !f.dirty {
			f.close(m)
			return nil
		}
		m.modal = &confirmModal{prompt: "Discard new issue?", onYes: func() tea.Cmd { f.close(m); return nil }}
		return nil
	case "tab", "down":
		return f.move(1)
	case "shift+tab", "up":
		return f.move(-1)
	}

	switch f.field() {
	case "summary":
		if msg.String() == "enter" {
			return f.move(1)
		}
		var cmd tea.Cmd
		f.summary, cmd = f.summary.Update(msg)
		if v := f.summary.Value(); v != f.values["summary"] {
			f.values["summary"], f.dirty, f.errText = v, true, ""
		}
		return cmd
	case "description":
		if msg.String() == "enter" || msg.String() == "e" {
			return f.editDescription(m)
		}
	default:
		if msg.String() == "enter" {
			return f.pick(m)
		}
	}
	return nil
}

func (f *createForm) move(delta int) tea.Cmd {
	f.focus = (f.focus + delta + len(formFields)) % len(formFields)
	if f.field() == "summary" {
		return f.summary.Focus()
	}
	f.summary.Blur()
	return nil
}

func (f *createForm) close(m *Model) {
	if hv := m.hosts[f.sess.Host.Name]; hv != nil && hv.create == f {
		hv.create, hv.screen = nil, screenList
	}
}

// pick opens a picker for the focused field with options from the server.
func (f *createForm) pick(m *Model) tea.Cmd {
	field, host, cache, projectID := f.field(), f.sess.Host.Name, f.sess.Meta, f.projectID
	current := f.values[field]
	if (field == "category" || field == "assignee") && projectID == 0 {
		f.errText = "choose a project first"
		return nil
	}
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		var opts []pickerOption
		switch field {
		case "project":
			ps, err := cache.Projects(ctx)
			if err != nil {
				return errMsg{host: host, err: err}
			}
			for _, p := range ps {
				opts = append(opts, pickerOption{label: p.Name, value: createSetMsg{host: host, field: field, value: p.Name, id: p.ID}, current: p.Name == current})
			}
		case "category":
			cats, err := cache.Categories(ctx, projectID)
			if err != nil {
				return errMsg{host: host, err: err}
			}
			for _, c := range cats {
				opts = append(opts, pickerOption{label: c.Name, value: createSetMsg{host: host, field: field, value: c.Name}, current: c.Name == current})
			}
		case "assignee":
			users, err := cache.Users(ctx, projectID)
			if err != nil {
				return errMsg{host: host, err: err}
			}
			opts = append(opts, pickerOption{label: "(nobody)", value: createSetMsg{host: host, field: field}, current: current == ""})
			for _, u := range users {
				opts = append(opts, pickerOption{label: u.Display(), detail: u.Name, current: u.Name == current,
					value: createSetMsg{host: host, field: field, value: u.Name, label: u.Display()}})
			}
		default: // priority, severity, reproducibility
			vals, err := cache.Enum(ctx, field)
			if err != nil {
				return errMsg{host: host, err: err}
			}
			opts = append(opts, pickerOption{label: "(server default)", value: createSetMsg{host: host, field: field}, current: current == ""})
			for _, v := range vals {
				opts = append(opts, pickerOption{label: v.Label, current: v.Name == current,
					value: createSetMsg{host: host, field: field, value: v.Name, label: v.Label}})
			}
		}
		return modalReadyMsg{host: host, modal: newPicker(fieldLabels[field], opts, func(o pickerOption) tea.Cmd {
			set := o.value.(createSetMsg)
			return func() tea.Msg { return set }
		})}
	})
}

func (f *createForm) set(m *Model, msg createSetMsg) tea.Cmd {
	f.values[msg.field], f.labels[msg.field] = msg.value, msg.label
	f.dirty, f.errText = true, ""
	if msg.field != "project" || msg.id == f.projectID {
		return nil
	}
	f.projectID = msg.id
	// Categories and users belong to a project: keep only what still exists.
	host, cache, projectID := f.sess.Host.Name, f.sess.Meta, msg.id
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		out := createRevalidateMsg{host: host, projectID: projectID}
		cats, err := cache.Categories(ctx, projectID)
		if err != nil {
			out.err = err
			return out
		}
		for _, c := range cats {
			out.categories = append(out.categories, c.Name)
		}
		users, err := cache.Users(ctx, projectID)
		if err != nil {
			out.err = err
			return out
		}
		for _, u := range users {
			out.users = append(out.users, u.Name)
		}
		return out
	})
}

func (f *createForm) revalidate(msg createRevalidateMsg) tea.Cmd {
	if msg.projectID != f.projectID {
		return nil
	}
	if msg.err != nil {
		return errCmd(msg.host, msg.err)
	}
	keep := func(field string, valid []string) {
		v := f.values[field]
		for _, ok := range valid {
			if strings.EqualFold(ok, v) {
				return
			}
		}
		f.values[field], f.labels[field] = "", ""
	}
	keep("category", msg.categories)
	keep("assignee", msg.users)
	return nil
}

func (f *createForm) editDescription(m *Model) tea.Cmd {
	host := f.sess.Host.Name
	return m.runEditor(editor.Request{
		Host: host, Kind: "description", Initial: f.values["description"],
		Hints: []string{
			"Description of the new issue on " + host + ".",
			"Lines starting with '# ' like these are removed.",
		},
	}, func(_ *Model, s *editor.Session, text string, err error) tea.Cmd {
		s.Cleanup()
		if errors.Is(err, editor.ErrEmpty) {
			return nil // unchanged
		}
		if err != nil {
			return errCmd(host, err)
		}
		f.values["description"], f.dirty, f.errText = text, true, ""
		return nil
	})
}

func (f *createForm) submit(m *Model) tea.Cmd {
	in := service.CreateInput{
		Project: f.values["project"], Category: f.values["category"],
		Summary: f.values["summary"], Description: f.values["description"],
		Priority: f.values["priority"], Severity: f.values["severity"],
		Reproducibility: f.values["reproducibility"], Assignee: f.values["assignee"],
	}
	for _, req := range []struct{ field, value, hint string }{
		{"project", in.Project, ""}, {"category", in.Category, ""},
		{"summary", strings.TrimSpace(in.Summary), ""}, {"description", strings.TrimSpace(in.Description), " (e to edit)"},
	} {
		if req.value == "" {
			f.errText = req.field + " is required" + req.hint
			return nil
		}
	}
	host, resolve, api := f.sess.Host.Name, f.sess.Resolve, f.sess.API
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		req, err := resolve.NewIssue(ctx, in)
		if err != nil {
			return createdMsg{host: host, err: err}
		}
		is, err := api.CreateIssue(ctx, req)
		return createdMsg{host: host, issue: is, err: err}
	})
}

func (f *createForm) context() string { return "new issue" }

func (f *createForm) view(width, _ int) string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("New issue on "+f.sess.Host.Name) + "\n\n")
	for i, field := range formFields {
		value := f.values[field]
		if l := f.labels[field]; l != "" {
			value = l
		}
		switch field {
		case "summary":
			value = f.summary.View()
		case "description":
			first, _, _ := strings.Cut(value, "\n")
			if value == "" {
				value = styleMuted.Render("(empty, e to edit)")
			} else if first != value {
				value = first + styleMuted.Render(" …")
			}
		case "priority", "severity", "reproducibility":
			if value == "" {
				value = styleMuted.Render("(server default)")
			}
		case "assignee":
			if value == "" {
				value = styleMuted.Render("(nobody)")
			}
		default:
			if value == "" {
				value = styleMuted.Render("(choose)")
			}
		}
		marker := "  "
		label := fmt.Sprintf("%-16s", fieldLabels[field])
		if i == f.focus {
			marker, label = "▸ ", styleSelected.Render(label)
		}
		b.WriteString(marker + label + " " + ansi.Truncate(value, max(width-22, 10), "…") + "\n")
	}
	if desc := f.values["description"]; strings.Contains(desc, "\n") {
		b.WriteString("\n" + styleMuted.Render(ansi.Truncate(strings.ReplaceAll(desc, "\n", " ⏎ "), width-2, "…")) + "\n")
	}
	if f.errText != "" {
		b.WriteString("\n" + styleError.Render(f.errText) + "\n")
	}
	b.WriteString("\n" + styleMuted.Render("tab/↑↓ move · enter choose · e edit description · alt+enter (or ctrl+s) create · esc cancel"))
	return b.String()
}
