package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
)

// MissingFieldError means a required field was left empty.
type MissingFieldError struct{ Field string }

func (e *MissingFieldError) Error() string { return e.Field + " is required" }

// CreateInput is a new issue as the user typed it: names, labels or ids.
// Empty optional fields use the server's defaults.
type CreateInput struct {
	Project, Category, Summary, Description string
	Priority, Severity, Reproducibility     string
	Assignee                                string
}

// NewIssue validates and resolves a CreateInput into a create request. The
// CLI and the TUI create form both go through here.
func (r *Resolver) NewIssue(ctx context.Context, in CreateInput) (mantis.NewIssue, error) {
	summary, description := strings.TrimSpace(in.Summary), strings.TrimSpace(in.Description)
	for _, f := range []struct{ name, value string }{
		{"project", in.Project}, {"category", in.Category}, {"summary", summary}, {"description", description},
	} {
		if strings.TrimSpace(f.value) == "" {
			return mantis.NewIssue{}, &MissingFieldError{Field: f.name}
		}
	}

	project, err := r.Project(ctx, in.Project)
	if err != nil {
		return mantis.NewIssue{}, err
	}
	category, err := r.Category(ctx, project.ID, in.Category)
	if err != nil {
		return mantis.NewIssue{}, err
	}
	out := mantis.NewIssue{
		Summary:     summary,
		Description: description,
		Project:     mantis.Ref{ID: project.ID, Name: project.Name},
		Category:    mantis.Ref(category),
	}

	for _, e := range []struct {
		kind, value string
		dst         **mantis.Ref
	}{
		{meta.Priority, in.Priority, &out.Priority},
		{meta.Severity, in.Severity, &out.Severity},
		{meta.Reproducibility, in.Reproducibility, &out.Reproducibility},
	} {
		if e.value == "" {
			continue
		}
		v, err := r.Enum(ctx, e.kind, e.value)
		if err != nil {
			return mantis.NewIssue{}, err
		}
		*e.dst = &mantis.Ref{ID: v.ID, Name: v.Name}
	}
	if in.Assignee != "" {
		u, err := r.User(ctx, project.ID, in.Assignee)
		if err != nil {
			return mantis.NewIssue{}, fmt.Errorf("assignee: %w", err)
		}
		out.Handler = &mantis.Ref{ID: u.ID, Name: u.Name}
	}
	return out, nil
}
