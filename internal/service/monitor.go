package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
)

// IsMonitoring reports whether userID monitors the issue.
func IsMonitoring(is mantis.Issue, userID int) bool {
	return slices.ContainsFunc(is.Monitors, func(u mantis.User) bool { return u.ID == userID })
}

// Unmonitor removes the current user from an issue's monitors. MantisBT has
// no DELETE route for monitors (verified on 2.24), so this fetches the issue
// and PATCHes back the monitor list without the current user, preserving
// everyone else.
func Unmonitor(ctx context.Context, m *meta.Cache, issueID int) error {
	me, err := m.Me(ctx)
	if err != nil {
		return fmt.Errorf("unmonitor issue %d: %w", issueID, err)
	}
	res, err := m.API().GetIssue(ctx, issueID)
	if err != nil {
		return fmt.Errorf("unmonitor: %w", err)
	}
	if !IsMonitoring(res.Issue, me.ID) {
		return nil
	}
	keep := []mantis.Ref{}
	for _, u := range res.Issue.Monitors {
		if u.ID != me.ID {
			keep = append(keep, mantis.Ref{ID: u.ID})
		}
	}
	if _, err := m.API().UpdateIssue(ctx, issueID, mantis.IssuePatch{Monitors: &keep}); err != nil {
		return fmt.Errorf("unmonitor: %w", err)
	}
	return nil
}
