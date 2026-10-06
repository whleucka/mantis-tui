package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/service"
)

func newMonitorCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "monitor <id>...",
		Short: "Start monitoring issues",
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
			return opts.runBatch(cmd, ids, "monitored", s.client.Monitor)
		},
	}
}

func newUnmonitorCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "unmonitor <id>...",
		Short: "Stop monitoring issues",
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
			return opts.runBatch(cmd, ids, "unmonitored", func(ctx context.Context, id int) error {
				return service.Unmonitor(ctx, s.meta, id)
			})
		},
	}
}

func newOpenCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "open <id>",
		Short: "Open an issue in the browser",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			s, err := opts.openSession(cmd)
			if err != nil {
				return err
			}
			url := service.IssueURL(s.host.URL, id)
			fmt.Fprintln(cmd.OutOrStdout(), url)
			return opts.deps.openURL(url)
		},
	}
}
