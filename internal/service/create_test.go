package service

import (
	"context"
	"errors"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

func TestNewIssueResolvesEverything(t *testing.T) {
	r, _ := newResolver()
	in, err := r.NewIssue(context.Background(), CreateInput{
		Project: "soprano", Category: "backend", Summary: "  Crash on save ", Description: "It crashes.\n",
		Priority: "high", Severity: "crash", Reproducibility: "always", Assignee: "jsmith",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := mantis.NewIssue{
		Summary: "Crash on save", Description: "It crashes.",
		Project:         mantis.Ref{ID: 4, Name: "Soprano"},
		Category:        mantis.Ref{ID: 2, Name: "Backend"},
		Priority:        &mantis.Ref{ID: 40, Name: "high"},
		Severity:        &mantis.Ref{ID: 70, Name: "crash"},
		Reproducibility: &mantis.Ref{ID: 10, Name: "always"},
		Handler:         &mantis.Ref{ID: 3, Name: "jsmith"},
	}
	if in.Summary != want.Summary || in.Description != want.Description || in.Project != want.Project ||
		in.Category != want.Category || *in.Priority != *want.Priority || *in.Severity != *want.Severity ||
		*in.Reproducibility != *want.Reproducibility || *in.Handler != *want.Handler {
		t.Errorf("got  %+v\nwant %+v", in, want)
	}
}

func TestNewIssueOptionalFieldsOmitted(t *testing.T) {
	r, _ := newResolver()
	in, err := r.NewIssue(context.Background(), CreateInput{Project: "4", Category: "General", Summary: "s", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if in.Priority != nil || in.Severity != nil || in.Reproducibility != nil || in.Handler != nil {
		t.Errorf("optional fields should be nil: %+v", in)
	}
}

func TestNewIssueValidation(t *testing.T) {
	r, _ := newResolver()
	ctx := context.Background()
	base := CreateInput{Project: "4", Category: "General", Summary: "s", Description: "d"}

	for name, mutate := range map[string]func(*CreateInput){
		"no project":     func(c *CreateInput) { c.Project = "" },
		"no category":    func(c *CreateInput) { c.Category = "" },
		"blank summary":  func(c *CreateInput) { c.Summary = "   " },
		"no description": func(c *CreateInput) { c.Description = "" },
	} {
		in := base
		mutate(&in)
		var missing *MissingFieldError
		if _, err := r.NewIssue(ctx, in); !errors.As(err, &missing) {
			t.Errorf("%s: err = %v, want MissingFieldError", name, err)
		}
	}

	in := base
	in.Severity = "catastrophic"
	var inv *InvalidValueError
	if _, err := r.NewIssue(ctx, in); !errors.As(err, &inv) {
		t.Errorf("bad severity: err = %v, want InvalidValueError", err)
	}
}
