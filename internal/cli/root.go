// Package cli implements the mantis-tui command line: the root command that
// launches the TUI and the non-interactive subcommands.
package cli

import (
	"time"

	"github.com/spf13/cobra"
)

// globalOpts holds the flags shared by every subcommand.
type globalOpts struct {
	host       string
	configPath string
	json       bool
	timeout    time.Duration
}

// NewRootCmd builds the mantis-tui command tree.
func NewRootCmd() *cobra.Command {
	opts := &globalOpts{}

	root := &cobra.Command{
		Use:           "mantis-tui",
		Short:         "A terminal UI and CLI for the MantisBT bug tracker",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErrorf("unknown command %q (see --help)", args[0])
			}
			return nil
		},
		// Launching the TUI lands in Task 10; until then show help.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	flags := root.PersistentFlags()
	flags.StringVar(&opts.host, "host", "", "host name from the config to use")
	flags.StringVar(&opts.configPath, "config", "", "path to config.toml (default $XDG_CONFIG_HOME/mantis-tui/config.toml)")
	flags.BoolVar(&opts.json, "json", false, "print raw API JSON instead of a table")
	flags.DurationVar(&opts.timeout, "timeout", 30*time.Second, "timeout for each API request")

	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return asUsage(err) })
	root.AddCommand(newHostsCmd(opts), newListCmd(opts), newShowCmd(opts))
	return root
}
