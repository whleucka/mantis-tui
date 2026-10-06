// Package service holds the operations shared by the CLI and the TUI: turning
// user-facing names into Mantis ids, batch updates, and multi-step API calls.
package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
)

// InvalidValueError means a user-supplied name did not match any valid value.
type InvalidValueError struct {
	Kind  string // e.g. "project", "status"
	Value string
	Valid []string
}

func (e *InvalidValueError) Error() string {
	return fmt.Sprintf("unknown %s %q (valid: %s)", e.Kind, e.Value, strings.Join(e.Valid, ", "))
}

// AmbiguousError means a name matched more than one candidate.
type AmbiguousError struct {
	Kind       string
	Value      string
	Candidates []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("%s %q is ambiguous; use one of: %s", e.Kind, e.Value, strings.Join(e.Candidates, ", "))
}

// Resolver turns names typed by a user into Mantis objects, using the
// per-host metadata cache.
type Resolver struct {
	meta *meta.Cache
}

// NewResolver returns a resolver backed by m.
func NewResolver(m *meta.Cache) *Resolver { return &Resolver{meta: m} }

// Enum resolves an enum value by name, label or numeric id (case-insensitive).
func (r *Resolver) Enum(ctx context.Context, kind, s string) (mantis.EnumValue, error) {
	vals, err := r.meta.Enum(ctx, kind)
	if err != nil {
		return mantis.EnumValue{}, err
	}
	id, idErr := strconv.Atoi(s)
	names := make([]string, 0, len(vals))
	for _, v := range vals {
		if strings.EqualFold(v.Name, s) || strings.EqualFold(v.Label, s) || (idErr == nil && v.ID == id) {
			return v, nil
		}
		names = append(names, v.Name)
	}
	return mantis.EnumValue{}, &InvalidValueError{Kind: kind, Value: s, Valid: names}
}

// Project resolves a project by id or case-insensitive name.
func (r *Resolver) Project(ctx context.Context, s string) (mantis.Project, error) {
	ps, err := r.meta.Projects(ctx)
	if err != nil {
		return mantis.Project{}, err
	}
	id, idErr := strconv.Atoi(s)
	names := make([]string, 0, len(ps))
	for _, p := range ps {
		if strings.EqualFold(p.Name, s) || (idErr == nil && p.ID == id) {
			return p, nil
		}
		names = append(names, p.Name)
	}
	return mantis.Project{}, &InvalidValueError{Kind: "project", Value: s, Valid: names}
}

// Category resolves a category of a project by case-insensitive name or id.
func (r *Resolver) Category(ctx context.Context, projectID int, s string) (mantis.Category, error) {
	cats, err := r.meta.Categories(ctx, projectID)
	if err != nil {
		return mantis.Category{}, err
	}
	id, idErr := strconv.Atoi(s)
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		if strings.EqualFold(c.Name, s) || (idErr == nil && c.ID == id) {
			return c, nil
		}
		names = append(names, c.Name)
	}
	return mantis.Category{}, &InvalidValueError{Kind: "category", Value: s, Valid: names}
}

// User resolves a user by numeric id, username or real name. Ids are accepted
// as-is; usernames win over real names; a real name shared by several users
// is an AmbiguousError.
func (r *Resolver) User(ctx context.Context, projectID int, s string) (mantis.User, error) {
	if id, err := strconv.Atoi(s); err == nil && id > 0 {
		return mantis.User{ID: id}, nil
	}
	users, err := r.meta.Users(ctx, projectID)
	if err != nil {
		return mantis.User{}, err
	}
	var byReal []mantis.User
	names := make([]string, 0, len(users))
	for _, u := range users {
		if strings.EqualFold(u.Name, s) {
			return u, nil
		}
		if strings.EqualFold(u.RealName, s) {
			byReal = append(byReal, u)
		}
		names = append(names, u.Name)
	}
	switch len(byReal) {
	case 0:
		return mantis.User{}, &InvalidValueError{Kind: "user", Value: s, Valid: names}
	case 1:
		return byReal[0], nil
	}
	cands := make([]string, len(byReal))
	for i, u := range byReal {
		cands[i] = fmt.Sprintf("%s (%s, id %d)", u.Name, u.RealName, u.ID)
	}
	return mantis.User{}, &AmbiguousError{Kind: "user", Value: s, Candidates: cands}
}
