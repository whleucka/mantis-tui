package cli

import (
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
)

func newListCmd(opts *globalOpts) *cobra.Command {
	var (
		filter   string
		project  string
		page     int
		pageSize int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}
			if filter == "" {
				filter = s.cfg.List.DefaultFilter
			}
			if !slices.Contains(config.Filters, filter) {
				return usageErrorf("invalid --filter %q (valid: %s)", filter, strings.Join(config.Filters, ", "))
			}
			if pageSize == 0 {
				pageSize = s.cfg.List.PageSize
			}
			if page < 1 || pageSize < 1 {
				return usageErrorf("--page and --page-size must be at least 1")
			}

			ctx, cancel := opts.ctx(cmd)
			defer cancel()

			listOpts := mantis.ListOptions{Filter: filter, Page: page, PageSize: pageSize, Select: mantis.ListFields}
			if project != "" {
				p, err := s.resolve.Project(ctx, project)
				if err != nil {
					return err
				}
				listOpts.ProjectID = p.ID
			}
			res, err := s.client.ListIssues(ctx, listOpts)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if opts.json {
				return writeRaw(out, res.Raw)
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tSTATUS\tPRIORITY\tSEVERITY\tCATEGORY\tHANDLER\tUPDATED\tSUMMARY")
			for _, is := range res.Issues {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					is.ID, is.Status.Label, is.Priority.Label, is.Severity.Label,
					is.Category.Name, handlerName(is), formatDate(is.UpdatedAt), is.Summary)
			}
			return tw.Flush()
		},
	}
	f := cmd.Flags()
	f.StringVar(&filter, "filter", "", "all | assigned | reported | monitored | unassigned (default from config)")
	f.StringVar(&project, "project", "", "project id or name")
	f.IntVar(&page, "page", 1, "page number")
	f.IntVar(&pageSize, "page-size", 0, "issues per page (default from config)")
	return cmd
}
