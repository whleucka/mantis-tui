package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// relatedTarget is an issue that g r can open.
type relatedTarget struct {
	id     int
	label  string
	detail string
}

// mentionRe finds "#123" not glued to a word or an HTML entity ("&#123;").
// The character after the digits is checked separately (RE2 has no
// lookahead).
var mentionRe = regexp.MustCompile(`(?:^|[^\pL\pN&#])#(\d+)`)

// mentions returns the issue ids text mentions, in order.
func mentions(text string) []int {
	var ids []int
	for _, loc := range mentionRe.FindAllStringSubmatchIndex(text, -1) {
		if r, _ := utf8.DecodeRuneInString(text[loc[3]:]); unicode.IsLetter(r) {
			continue
		}
		if id, err := strconv.Atoi(text[loc[2]:loc[3]]); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// relatedTargets lists is's relationships, then the issues its text and
// notes mention, each id once and never is itself.
func relatedTargets(is *mantis.Issue) []relatedTarget {
	seen := map[int]bool{is.ID: true}
	var out []relatedTarget
	for _, r := range is.Relationships {
		if r.Issue.ID == 0 || seen[r.Issue.ID] {
			continue
		}
		seen[r.Issue.ID] = true
		out = append(out, relatedTarget{
			id:     r.Issue.ID,
			label:  fmt.Sprintf("%s #%d %s", enumLabel(r.Type), r.Issue.ID, r.Issue.Summary),
			detail: enumLabel(r.Issue.Status),
		})
	}
	add := func(text, where string) {
		for _, id := range mentions(text) {
			if !seen[id] {
				seen[id] = true
				out = append(out, relatedTarget{id: id, label: fmt.Sprintf("#%d", id), detail: "mentioned in " + where})
			}
		}
	}
	add(is.Description, "description")
	add(is.StepsToReproduce, "steps to reproduce")
	add(is.AdditionalInformation, "additional information")
	for _, n := range is.Notes {
		add(n.Text, fmt.Sprintf("note %d by %s", n.ID, n.Reporter.Display()))
	}
	return out
}

func enumLabel(v mantis.EnumValue) string {
	if v.Label != "" {
		return v.Label
	}
	return v.Name
}

// goRelated opens the issue view's only related issue, or offers a picker.
// The issue left behind joins the trail, so back returns to it.
func (m *Model) goRelated(iv *issueModel) tea.Cmd {
	if iv.issue == nil {
		return nil
	}
	targets := relatedTargets(iv.issue)
	trail := append(slices.Clone(iv.trail), iv.id)
	switch len(targets) {
	case 0:
		return infoCmd(iv.sess.Host.Name, fmt.Sprintf("#%d has no related or mentioned issues", iv.id))
	case 1:
		return m.openIssueTrail(targets[0].id, trail)
	}
	opts := make([]pickerOption, len(targets))
	for i, t := range targets {
		opts[i] = pickerOption{label: t.label, detail: t.detail, value: t.id}
	}
	m.modal = newPicker(fmt.Sprintf("Go to which issue from #%d?", iv.id), opts, func(o pickerOption) tea.Cmd {
		return m.openIssueTrail(o.value.(int), trail)
	})
	return nil
}
