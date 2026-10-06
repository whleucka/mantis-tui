package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/editor"
	"github.com/whleucka/mantis-tui/internal/service"
)

func newCreateCmd(opts *globalOpts) *cobra.Command {
	var in service.CreateInput
	var edit bool
	cmd := &cobra.Command{
		Use:   "create --project P --category C --summary S [-d text | --edit]",
		Short: "Create an issue",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			withDesc := cmd.Flags().Changed("description")
			switch {
			case withDesc && edit:
				return usageErrorf("use only one of -d or --edit")
			case !withDesc && !edit && !opts.deps.isTerminal():
				return usageErrorf("description is required: pass -d <text> or --edit")
			case !withDesc:
				edit = true
			}
			for _, f := range []struct{ flag, value string }{
				{"--project", in.Project}, {"--category", in.Category}, {"--summary", in.Summary},
			} {
				if strings.TrimSpace(f.value) == "" {
					return usageErrorf("%s is required", f.flag)
				}
			}

			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}
			ctx, cancel := opts.ctx(cmd)
			defer cancel()

			// Resolve names before opening the editor so typos fail fast.
			probe := in
			if probe.Description == "" {
				probe.Description = "-"
			}
			if _, err := s.resolve.NewIssue(ctx, probe); err != nil {
				return err
			}

			var ed *editor.Session
			if edit {
				ed, err = editor.Prepare(editor.Request{
					Host: s.host.Name, Kind: "description",
					Hints: []string{
						fmt.Sprintf("Description for new issue %q on %s.", in.Summary, s.host.Name),
						"Lines starting with '# ' like these are removed. Save an empty file to cancel.",
					},
				})
				if err != nil {
					return err
				}
				if err := ed.Run(); err != nil {
					ed.Cleanup()
					return err
				}
				if in.Description, err = ed.Text(); err != nil {
					ed.Cleanup()
					return err
				}
			}

			req, err := s.resolve.NewIssue(ctx, in)
			if err != nil {
				return err
			}
			created, err := s.client.CreateIssue(ctx, req)
			if err != nil {
				if ed != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "issue not created; your description is saved in %s\n", ed.Path)
				}
				return err
			}
			if ed != nil {
				ed.Cleanup()
			}

			url := service.IssueURL(s.host.URL, created.ID)
			out := cmd.OutOrStdout()
			if opts.json {
				return json.NewEncoder(out).Encode([]map[string]any{{"id": created.ID, "ok": true, "url": url}})
			}
			fmt.Fprintf(out, "#%d created: %s\n", created.ID, url)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&in.Project, "project", "", "project name or id (required)")
	f.StringVar(&in.Category, "category", "", "category name (required)")
	f.StringVar(&in.Summary, "summary", "", "one-line summary (required)")
	f.StringVarP(&in.Description, "description", "d", "", "description text")
	f.BoolVar(&edit, "edit", false, "write the description in $VISUAL/$EDITOR")
	f.StringVar(&in.Priority, "priority", "", "priority (default: server default)")
	f.StringVar(&in.Severity, "severity", "", "severity")
	f.StringVar(&in.Reproducibility, "reproducibility", "", "reproducibility")
	f.StringVar(&in.Assignee, "assign", "", "assignee: username, real name or id")
	return cmd
}

func newDeleteCmd(opts *globalOpts) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>...",
		Short: "Delete issues",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}
			prompt := fmt.Sprintf("Delete issue #%d?", ids[0])
			if len(ids) > 1 {
				prompt = fmt.Sprintf("Delete %d issues (%s)?", len(ids), "#"+strings.Join(args, ", #"))
			}
			if err := opts.confirm(cmd, prompt, yes); err != nil {
				return err
			}
			return opts.runBatch(cmd, ids, "deleted", s.client.DeleteIssue)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	return cmd
}
