// Package mantistest provides an in-memory fake of mantis.API for tests.
package mantistest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// Fake is an in-memory mantis.API. Set its fields before use; every method
// counts its calls and can be made to fail via Errs[methodName].
type Fake struct {
	mu sync.Mutex

	CurrentUser  mantis.User
	Issues       map[int]mantis.Issue
	ProjectList  []mantis.Project
	UsersByProj  map[int][]mantis.User
	ConfigValues map[string]json.RawMessage
	Errs         map[string]error

	calls map[string]int
}

var _ mantis.API = (*Fake)(nil)

// Calls returns how many times method was called.
func (f *Fake) Calls(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *Fake) enter(method string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[method]++
	return f.Errs[method]
}

// BaseURL implements mantis.API.
func (f *Fake) BaseURL() string { return "https://mantis.example.test" }

// Me implements mantis.API.
func (f *Fake) Me(context.Context) (*mantis.User, error) {
	if err := f.enter("Me"); err != nil {
		return nil, err
	}
	u := f.CurrentUser
	return &u, nil
}

// ListIssues implements mantis.API, returning all issues ordered by id descending.
func (f *Fake) ListIssues(context.Context, mantis.ListOptions) (*mantis.IssueList, error) {
	if err := f.enter("ListIssues"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]mantis.Issue, 0, len(f.Issues))
	for _, is := range f.Issues {
		out = append(out, is)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	raw, _ := json.Marshal(map[string]any{"issues": out})
	return &mantis.IssueList{Issues: out, Raw: raw}, nil
}

// GetIssue implements mantis.API.
func (f *Fake) GetIssue(_ context.Context, id int) (*mantis.IssueResult, error) {
	if err := f.enter("GetIssue"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	is, ok := f.Issues[id]
	if !ok {
		return nil, fmt.Errorf("get issue %d: %w", id, &mantis.APIError{Status: 404, Message: "Issue not found"})
	}
	raw, _ := json.Marshal(map[string]any{"issues": []mantis.Issue{is}})
	return &mantis.IssueResult{Issue: is, Raw: raw}, nil
}

// Projects implements mantis.API.
func (f *Fake) Projects(context.Context) ([]mantis.Project, error) {
	if err := f.enter("Projects"); err != nil {
		return nil, err
	}
	return f.ProjectList, nil
}

// Project implements mantis.API.
func (f *Fake) Project(_ context.Context, id int) (*mantis.Project, error) {
	if err := f.enter("Project"); err != nil {
		return nil, err
	}
	for _, p := range f.ProjectList {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, &mantis.APIError{Status: 404, Message: "Project not found"}
}

// ProjectUsers implements mantis.API.
func (f *Fake) ProjectUsers(_ context.Context, projectID int) ([]mantis.User, error) {
	if err := f.enter("ProjectUsers"); err != nil {
		return nil, err
	}
	return f.UsersByProj[projectID], nil
}

// Config implements mantis.API.
func (f *Fake) Config(_ context.Context, options ...string) (map[string]json.RawMessage, error) {
	if err := f.enter("Config"); err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for _, o := range options {
		if v, ok := f.ConfigValues[o]; ok {
			out[o] = v
		}
	}
	return out, nil
}

// StandardConfig returns enum config values shaped like MantisBT 2.27's defaults.
func StandardConfig() map[string]json.RawMessage {
	enum := func(pairs ...any) json.RawMessage {
		var vals []mantis.EnumValue
		for i := 0; i < len(pairs); i += 2 {
			vals = append(vals, mantis.EnumValue{ID: pairs[i].(int), Name: pairs[i+1].(string), Label: pairs[i+1].(string)})
		}
		b, _ := json.Marshal(vals)
		return b
	}
	return map[string]json.RawMessage{
		"status_enum_string":          enum(10, "new", 20, "feedback", 30, "acknowledged", 40, "confirmed", 50, "assigned", 80, "resolved", 90, "closed"),
		"priority_enum_string":        enum(10, "none", 20, "low", 30, "normal", 40, "high", 50, "urgent", 60, "immediate"),
		"severity_enum_string":        enum(10, "feature", 20, "trivial", 30, "text", 40, "tweak", 50, "minor", 60, "major", 70, "crash", 80, "block"),
		"reproducibility_enum_string": enum(10, "always", 30, "sometimes", 50, "random", 70, "have not tried", 90, "unable to reproduce", 100, "N/A"),
		"resolution_enum_string":      enum(10, "open", 20, "fixed", 30, "reopened", 40, "unable to reproduce", 50, "not fixable", 60, "duplicate", 70, "no change required", 80, "suspended", 90, "won't fix"),
		"status_colors":               json.RawMessage(`{"new":"#fcbdbd","assigned":"#c2dfff","resolved":"#d2f5b0","closed":"#c9ccc4"}`),
	}
}
