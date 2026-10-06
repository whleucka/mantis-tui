package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
)

func newUpdateCmd(opts *globalOpts) *cobra.Command {
	var status, priority, severity, category, summary, resolution string
	cmd := &cobra.Command{
		Use:   "update <id>...",
		Short: "Change status, priority, severity, category, summary or resolution",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			f := cmd.Flags()
			if !f.Changed("status") && !f.Changed("priority") && !f.Changed("severity") &&
				!f.Changed("category") && !f.Changed("summary") && !f.Changed("resolution") {
				return usageErrorf("nothing to update: pass at least one of --status, --priority, --severity, --category, --summary, --resolution")
			}
			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}

			// Resolve enum names once, before touching any issue, so a typo
			// fails the whole command without partial writes.
			ctx, cancel := opts.ctx(cmd)
			defer cancel()
			var patch mantis.IssuePatch
			for _, e := range []struct {
				kind, value string
				dst         **mantis.Ref
			}{
				{meta.Status, status, &patch.Status},
				{meta.Priority, priority, &patch.Priority},
				{meta.Severity, severity, &patch.Severity},
				{meta.Resolution, resolution, &patch.Resolution},
			} {
				if e.value == "" {
					continue
				}
				v, err := s.resolve.Enum(ctx, e.kind, e.value)
				if err != nil {
					return err
				}
				*e.dst = &mantis.Ref{ID: v.ID, Name: v.Name}
			}
			if f.Changed("summary") {
				if summary == "" {
					return usageErrorf("--summary must not be empty")
				}
				patch.Summary = &summary
			}

			return opts.runBatch(cmd, ids, "updated", func(ctx context.Context, id int) error {
				p := patch
				if category != "" {
					// Categories belong to a project, so resolve per issue.
					res, err := s.client.GetIssue(ctx, id)
					if err != nil {
						return err
					}
					c, err := s.resolve.Category(ctx, res.Issue.Project.ID, category)
					if err != nil {
						return err
					}
					p.Category = &mantis.Ref{ID: c.ID, Name: c.Name}
				}
				_, err := s.client.UpdateIssue(ctx, id, p)
				return err
			})
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&status, "status", "", "new status (name, label or id)")
	fl.StringVar(&priority, "priority", "", "new priority")
	fl.StringVar(&severity, "severity", "", "new severity")
	fl.StringVar(&resolution, "resolution", "", "new resolution")
	fl.StringVar(&category, "category", "", "new category (in each issue's project)")
	fl.StringVar(&summary, "summary", "", "new summary")
	return cmd
}

func newAssignCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "assign <id>... <user>",
		Short: "Assign issues to a user (username, real name or id)",
		Args:  minArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args[:len(args)-1])
			if err != nil {
				return err
			}
			user := args[len(args)-1]
			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}
			return opts.runBatch(cmd, ids, "assigned to "+user, func(ctx context.Context, id int) error {
				// Users are listed per project, so resolve against each issue's project.
				res, err := s.client.GetIssue(ctx, id)
				if err != nil {
					return err
				}
				u, err := s.resolve.User(ctx, res.Issue.Project.ID, user)
				if err != nil {
					return err
				}
				_, err = s.client.UpdateIssue(ctx, id, mantis.IssuePatch{Handler: &mantis.Ref{ID: u.ID}})
				return err
			})
		},
	}
}
