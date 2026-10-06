package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

const (
	tabNotes = iota
	tabHistory
)

// issueModel shows one issue with its notes and history.
type issueModel struct {
	sess   *Session
	id     int
	issue  *mantis.Issue
	err    error
	tab    int
	vp     viewport.Model
	width  int
	req    int
	loaded bool
}

type issueLoadedMsg struct {
	host  string
	id    int
	req   int
	issue *mantis.Issue
	err   error
}

func (msg issueLoadedMsg) hostName() string { return msg.host }

func newIssueModel(sess *Session, id int) *issueModel {
	return &issueModel{sess: sess, id: id, vp: viewport.New()}
}

func (iv *issueModel) load(m *Model) tea.Cmd {
	iv.req++
	req, id, host, api := iv.req, iv.id, iv.sess.Host.Name, iv.sess.API
	return tea.Batch(m.startLoading(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		res, err := api.GetIssue(ctx, id)
		msg := issueLoadedMsg{host: host, id: id, req: req, err: err}
		if err == nil {
			msg.issue = &res.Issue
		}
		return msg
	})
}

func (iv *issueModel) handleMsg(m *Model, msg issueLoadedMsg) tea.Cmd {
	m.stopLoading()
	if msg.req != iv.req || msg.id != iv.id {
		return nil
	}
	if msg.err != nil {
		iv.err = msg.err
		iv.render(m)
		return errCmd(iv.sess.Host.Name, msg.err)
	}
	iv.issue, iv.err, iv.loaded = msg.issue, nil, true
	offset := iv.vp.YOffset()
	iv.render(m)
	iv.vp.SetYOffset(offset) // keep the reading position across refreshes
	return nil
}

func (iv *issueModel) handleAction(m *Model, a action) tea.Cmd {
	switch a {
	case actBack:
		m.cur.screen = screenList
		m.cur.issue = nil
	case actUp:
		iv.vp.ScrollUp(1)
	case actDown:
		iv.vp.ScrollDown(1)
	case actPageDown:
		iv.vp.HalfPageDown()
	case actPageUp:
		iv.vp.HalfPageUp()
	case actTop:
		iv.vp.GotoTop()
	case actBottom:
		iv.vp.GotoBottom()
	case actNextTab:
		iv.tab = (iv.tab + 1) % 2
		iv.render(m)
	case actRefresh:
		return iv.load(m)
	}
	return nil
}

func (iv *issueModel) context() string {
	return fmt.Sprintf("issue #%d", iv.id)
}

// render rebuilds the viewport content for the current size and tab.
func (iv *issueModel) render(m *Model) {
	w, h := max(m.width, 20), max(m.height-1, 1)
	iv.width = w
	iv.vp.SetWidth(w)
	iv.vp.SetHeight(h)
	iv.vp.SetContent(iv.content(w))
}

func (iv *issueModel) view() string { return iv.vp.View() }

func (iv *issueModel) content(w int) string {
	if iv.issue == nil {
		if iv.err != nil {
			return styleError.Render(fmt.Sprintf("Could not load issue #%d: %v", iv.id, iv.err)) + "\n" + styleMuted.Render("q to go back, r to retry")
		}
		return styleMuted.Render(fmt.Sprintf("Loading issue #%d…", iv.id))
	}
	is := iv.issue
	text := lipgloss.NewStyle().Width(w - 2)
	var b strings.Builder

	b.WriteString(styleTitle.Render(fmt.Sprintf("#%d %s", is.ID, is.Summary)) + "\n\n")

	status := is.Status.Label
	if is.Resolution.Name != "" && is.Resolution.Name != "open" {
		status += " (" + is.Resolution.Label + ")"
	}
	handler := ""
	if is.Handler != nil && is.Handler.ID != 0 {
		handler = is.Handler.Display()
	}
	pairs := [][2]string{
		{"Project", is.Project.Name}, {"Category", is.Category.Name},
		{"Status", status}, {"Priority", is.Priority.Label},
		{"Severity", is.Severity.Label}, {"Reproducibility", is.Reproducibility.Label},
		{"Reporter", is.Reporter.Display()}, {"Handler", handler},
		{"Created", timeOf(is.CreatedAt)}, {"Updated", timeOf(is.UpdatedAt)},
	}
	if len(is.Tags) > 0 {
		names := make([]string, len(is.Tags))
		for i, t := range is.Tags {
			names[i] = t.Name
		}
		pairs = append(pairs, [2]string{"Tags", strings.Join(names, ", ")})
	}
	for _, r := range is.Relationships {
		pairs = append(pairs, [2]string{"Related", fmt.Sprintf("%s #%d %s", r.Type.Label, r.Issue.ID, r.Issue.Summary)})
	}
	for _, a := range is.Attachments {
		pairs = append(pairs, [2]string{"Attachment", a.Filename})
	}
	for _, cf := range is.CustomFields {
		pairs = append(pairs, [2]string{cf.Field.Name, cf.Value})
	}
	for _, p := range pairs {
		if p[1] != "" {
			b.WriteString(styleMuted.Render(fmt.Sprintf("%-16s", p[0])) + p[1] + "\n")
		}
	}

	for _, sec := range [][2]string{
		{"Description", is.Description},
		{"Steps to reproduce", is.StepsToReproduce},
		{"Additional information", is.AdditionalInformation},
	} {
		if strings.TrimSpace(sec[1]) != "" {
			b.WriteString("\n" + styleHeader.Render(sec[0]) + "\n" + text.Render(sec[1]) + "\n")
		}
	}

	notes := fmt.Sprintf("Notes (%d)", len(is.Notes))
	history := fmt.Sprintf("History (%d)", len(is.History))
	if iv.tab == tabNotes {
		notes, history = styleSelected.Render(" "+notes+" "), styleMuted.Render(" "+history+" ")
	} else {
		notes, history = styleMuted.Render(" "+notes+" "), styleSelected.Render(" "+history+" ")
	}
	b.WriteString("\n" + notes + " " + history + styleMuted.Render("  (tab)") + "\n\n")

	if iv.tab == tabNotes {
		if len(is.Notes) == 0 {
			b.WriteString(styleMuted.Render("No notes. N adds one.") + "\n")
		}
		for _, n := range is.Notes {
			meta := []string{n.Reporter.Display(), timeOf(n.CreatedAt)}
			if n.Private() {
				meta = append(meta, "private")
			}
			if n.TimeTracking != nil && n.TimeTracking.Duration != "" && n.TimeTracking.Duration != "00:00" {
				meta = append(meta, n.TimeTracking.Duration)
			}
			meta = append(meta, fmt.Sprintf("note %d", n.ID))
			b.WriteString(styleGroup.Render("── "+strings.Join(meta, " · ")) + "\n")
			b.WriteString(text.Render(n.Text) + "\n")
			for _, a := range n.Attachments {
				b.WriteString(styleMuted.Render("attachment: "+a.Filename) + "\n")
			}
			b.WriteString("\n")
		}
	} else {
		if len(is.History) == 0 {
			b.WriteString(styleMuted.Render("No history.") + "\n")
		}
		for _, h := range is.History {
			line := h.Message
			if h.Change != "" {
				line += ": " + h.Change
			}
			b.WriteString(styleMuted.Render(timeOf(h.CreatedAt)+"  "+fmt.Sprintf("%-12s", h.User.Name)) + " " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func timeOf(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}
