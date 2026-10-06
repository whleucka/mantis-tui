// Package service holds the operations shared by the CLI and the TUI: turning
// user-facing names into Mantis ids, batch updates, and multi-step API calls.
package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/whleucka/mantis-tui/internal/mantis"
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

// ResolveProject accepts a project id or a case-insensitive project name.
func ResolveProject(ctx context.Context, api mantis.API, s string) (int, error) {
	if id, err := strconv.Atoi(s); err == nil && id > 0 {
		return id, nil
	}
	projects, err := api.Projects(ctx)
	if err != nil {
		return 0, err
	}
	valid := make([]string, 0, len(projects))
	for _, p := range projects {
		if strings.EqualFold(p.Name, s) {
			return p.ID, nil
		}
		valid = append(valid, p.Name)
	}
	return 0, &InvalidValueError{Kind: "project", Value: s, Valid: valid}
}
