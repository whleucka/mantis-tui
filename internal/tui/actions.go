package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
	"github.com/whleucka/mantis-tui/internal/service"
)

type (
	// doneMsg wraps the result of m.call so the spinner stops first.
	doneMsg struct{ inner tea.Msg }

	// modalReadyMsg opens a modal built from data fetched in the background.
	modalReadyMsg struct {
		host  string
		modal modal
	}

	// patchedMsg reports a (possibly batch) update.
	patchedMsg struct {
		host    string
		batch   int
		label   string
		results []service.Result
		updated map[int]*mantis.Issue
	}

	// batchProgressMsg reports one finished operation of a running batch.
	batchProgressMsg struct {
		host        string
		batch       int
		label       string
		done, total int
		next        tea.Cmd
	}

	// deletedMsg reports deleted issues.
	deletedMsg struct {
		host    string
		results []service.Result
	}
)

func (msg modalReadyMsg) hostName() string { return msg.host }
func (msg patchedMsg) hostName() string    { return msg.host }
func (msg deletedMsg) hostName() string    { return msg.host }

// call runs fn in the background with the spinner showing until it returns.
func (m *Model) call(fn func(ctx context.Context) tea.Msg) tea.Cmd {
	return tea.Batch(m.startLoading(), func() tea.Msg {
		return doneMsg{inner: fn(context.Background())}
	})
}

// timed bounds a single API request.
func timed(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, requestTimeout)
}

// currentIssue is the issue the user is looking at: the open issue, or the
// list row under the cursor.
func (m *Model) currentIssue() *mantis.Issue {
	if m.cur == nil {
		return nil
	}
	if m.cur.screen == screenIssue && m.cur.issue != nil {
		return m.cur.issue.issue
	}
	l := m.cur.list
	rs := l.rows()
	if l.cursor < 0 || l.cursor >= len(rs) || rs[l.cursor].idx < 0 {
		return nil
	}
	return &l.issues[rs[l.cursor].idx]
}

// issueAction handles the actions shared by the list and the issue view.
func (m *Model) issueAction(a action) tea.Cmd {
	is := m.currentIssue()
	if is == nil {
		return nil
	}
	targets := []mantis.Issue{*is}
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
	case actSummary:
		m.modal = m.summaryPrompt(*is)
	case actMonitor:
		return m.toggleMonitor(*is)
	case actBrowser:
		return m.openInBrowser(is.ID)
	case actCopyURL:
		url := service.IssueURL(m.cur.sess.Host.URL, is.ID)
		return tea.Batch(m.clipboard(url), infoCmd(m.cur.sess.Host.Name, "copied "+url))
	case actAddNote:
		return m.startNote(*is)
	}
	return nil
}

func ids(issues []mantis.Issue) []int {
	out := make([]int, len(issues))
	for i, is := range issues {
		out[i] = is.ID
	}
	return out
}

func targetLabel(issues []mantis.Issue) string {
	if len(issues) == 1 {
		return fmt.Sprintf("#%d", issues[0].ID)
	}
	return fmt.Sprintf("%d issues", len(issues))
}

func currentEnum(is mantis.Issue, kind string) string {
	switch kind {
	case meta.Status:
		return is.Status.Name
	case meta.Priority:
		return is.Priority.Name
	case meta.Severity:
		return is.Severity.Name
	}
	return ""
}

// pickEnum loads an enum and opens a picker that patches every target.
func (m *Model) pickEnum(kind string, targets []mantis.Issue) tea.Cmd {
	host, cache := m.cur.sess.Host.Name, m.cur.sess.Meta
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		vals, err := cache.Enum(ctx, kind)
		if err != nil {
			return errMsg{host: host, err: err}
		}
		opts := make([]pickerOption, len(vals))
		for i, v := range vals {
			opts[i] = pickerOption{label: v.Label, value: v, current: len(targets) == 1 && v.Name == currentEnum(targets[0], kind)}
		}
		title := fmt.Sprintf("%s for %s", strings.ToUpper(kind[:1])+kind[1:], targetLabel(targets))
		return modalReadyMsg{host: host, modal: newPicker(title, opts, func(o pickerOption) tea.Cmd {
			v := o.value.(mantis.EnumValue)
			ref := &mantis.Ref{ID: v.ID, Name: v.Name}
			var patch mantis.IssuePatch
			switch kind {
			case meta.Status:
				patch.Status = ref
			case meta.Priority:
				patch.Priority = ref
			case meta.Severity:
				patch.Severity = ref
			}
			return m.applyPatch(targets, func(mantis.Issue) mantis.IssuePatch { return patch }, kind+" → "+v.Label)
		})}
	})
}

// pickCategory offers the categories of the first target's project.
func (m *Model) pickCategory(targets []mantis.Issue) tea.Cmd {
	host, cache := m.cur.sess.Host.Name, m.cur.sess.Meta
	projectID := targets[0].Project.ID
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		cats, err := cache.Categories(ctx, projectID)
		if err != nil {
			return errMsg{host: host, err: err}
		}
		opts := make([]pickerOption, len(cats))
		for i, c := range cats {
			opts[i] = pickerOption{label: c.Name, value: c, current: len(targets) == 1 && c.Name == targets[0].Category.Name}
		}
		return modalReadyMsg{host: host, modal: newPicker("Category for "+targetLabel(targets), opts, func(o pickerOption) tea.Cmd {
			c := o.value.(mantis.Category)
			// Category ids belong to a project; issues elsewhere get the
			// name and the server resolves it in their own project.
			return m.applyPatch(targets, func(is mantis.Issue) mantis.IssuePatch {
				if is.Project.ID == projectID {
					return mantis.IssuePatch{Category: &mantis.Ref{ID: c.ID, Name: c.Name}}
				}
				return mantis.IssuePatch{Category: &mantis.Ref{Name: c.Name}}
			}, "category → "+c.Name)
		})}
	})
}

// pickUser offers the users of the first target's project.
func (m *Model) pickUser(targets []mantis.Issue) tea.Cmd {
	host, cache := m.cur.sess.Host.Name, m.cur.sess.Meta
	projectID := targets[0].Project.ID
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		users, err := cache.Users(ctx, projectID)
		if err != nil {
			return errMsg{host: host, err: err}
		}
		opts := make([]pickerOption, len(users))
		for i, u := range users {
			current := len(targets) == 1 && targets[0].Handler != nil && targets[0].Handler.ID == u.ID
			opts[i] = pickerOption{label: u.Display(), detail: u.Name, value: u, current: current}
		}
		return modalReadyMsg{host: host, modal: newPicker("Assign "+targetLabel(targets), opts, func(o pickerOption) tea.Cmd {
			u := o.value.(mantis.User)
			patch := mantis.IssuePatch{Handler: &mantis.Ref{ID: u.ID}}
			return m.applyPatch(targets, func(mantis.Issue) mantis.IssuePatch { return patch }, "assigned to "+u.Display())
		})}
	})
}

func (m *Model) summaryPrompt(is mantis.Issue) *textPrompt {
	return newTextPrompt(fmt.Sprintf("Summary of #%d", is.ID), is.Summary, m.width, func(v string) (tea.Cmd, string) {
		if v == "" {
			return nil, "summary must not be empty"
		}
		if v == is.Summary {
			return nil, ""
		}
		patch := mantis.IssuePatch{Summary: &v}
		return m.applyPatch([]mantis.Issue{is}, func(mantis.Issue) mantis.IssuePatch { return patch }, "summary updated"), ""
	})
}

// applyPatch updates every target (at most service.MaxInFlight at once),
// streaming progress to the status bar for batches.
func (m *Model) applyPatch(targets []mantis.Issue, patchFor func(mantis.Issue) mantis.IssuePatch, label string) tea.Cmd {
	host, api := m.cur.sess.Host.Name, m.cur.sess.API
	byID := make(map[int]mantis.Issue, len(targets))
	for _, is := range targets {
		byID[is.ID] = is
	}
	m.batchSeq++
	batch := m.batchSeq
	m.batchID = batch
	progress := make(chan struct{}, len(targets))

	run := m.call(func(ctx context.Context) tea.Msg {
		defer close(progress)
		var mu sync.Mutex
		updated := make(map[int]*mantis.Issue, len(targets))
		results := service.Batch(ctx, ids(targets), func(ctx context.Context, id int) error {
			ctx, cancel := timed(ctx)
			defer cancel()
			is, err := api.UpdateIssue(ctx, id, patchFor(byID[id]))
			if err == nil {
				mu.Lock()
				updated[id] = is
				mu.Unlock()
			}
			progress <- struct{}{}
			return err
		})
		return patchedMsg{host: host, batch: batch, label: label, results: results, updated: updated}
	})
	if len(targets) == 1 {
		return run
	}
	done := 0
	var listen tea.Cmd
	listen = func() tea.Msg {
		if _, ok := <-progress; !ok {
			return nil
		}
		done++
		return batchProgressMsg{host: host, batch: batch, label: label, done: done, total: len(targets), next: listen}
	}
	return tea.Batch(run, listen)
}

// toggleMonitor monitors or unmonitors is for the current user, then
// re-fetches it so the row's monitor icon is accurate.
func (m *Model) toggleMonitor(is mantis.Issue) tea.Cmd {
	host, sess := m.cur.sess.Host.Name, m.cur.sess
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := timed(ctx)
		defer cancel()
		me, err := sess.Meta.Me(ctx)
		if err != nil {
			return errMsg{host: host, err: err}
		}
		label := "monitoring"
		if service.IsMonitoring(is, me.ID) {
			label = "stopped monitoring"
			err = service.Unmonitor(ctx, sess.Meta, is.ID)
		} else {
			err = sess.API.Monitor(ctx, is.ID)
		}
		res := []service.Result{{ID: is.ID, Err: err}}
		updated := map[int]*mantis.Issue{}
		if err == nil {
			if fresh, ferr := sess.API.GetIssue(ctx, is.ID); ferr == nil {
				updated[is.ID] = &fresh.Issue
			}
		}
		return patchedMsg{host: host, label: label, results: res, updated: updated}
	})
}

func (m *Model) openInBrowser(id int) tea.Cmd {
	host := m.cur.sess.Host.Name
	url := service.IssueURL(m.cur.sess.Host.URL, id)
	open := m.opts.OpenURL
	return func() tea.Msg {
		if open == nil {
			return errMsg{host: host, err: errors.New("no browser opener configured")}
		}
		if err := open(url); err != nil {
			return errMsg{host: host, err: err}
		}
		return infoMsg{host: host, text: "opened " + url}
	}
}

// confirmDelete asks before deleting targets.
func (m *Model) confirmDelete(targets []mantis.Issue) {
	prompt := fmt.Sprintf("Delete #%d %s?", targets[0].ID, targets[0].Summary)
	if len(targets) > 1 {
		prompt = fmt.Sprintf("Delete %d issues?", len(targets))
	}
	host, api := m.cur.sess.Host.Name, m.cur.sess.API
	m.modal = &confirmModal{prompt: prompt, onYes: func() tea.Cmd {
		return m.call(func(ctx context.Context) tea.Msg {
			results := service.Batch(ctx, ids(targets), func(ctx context.Context, id int) error {
				ctx, cancel := timed(ctx)
				defer cancel()
				return api.DeleteIssue(ctx, id)
			})
			return deletedMsg{host: host, results: results}
		})
	}}
}

// summarize turns batch results into a status line.
func summarize(label string, results []service.Result) (text string, failed error) {
	var first error
	n := 0
	for _, r := range results {
		if r.Err != nil {
			n++
			if first == nil {
				first = fmt.Errorf("#%d: %w", r.ID, r.Err)
			}
		}
	}
	switch {
	case n == 0 && len(results) == 1:
		return fmt.Sprintf("#%d %s", results[0].ID, label), nil
	case n == 0:
		return fmt.Sprintf("%d issues: %s", len(results), label), nil
	case len(results) == 1:
		return "", first
	}
	return "", fmt.Errorf("%d of %d failed (%d ok): %w", n, len(results), len(results)-n, first)
}
