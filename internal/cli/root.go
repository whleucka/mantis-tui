// Package cli implements the mantis-tui command line: the root command that
// launches the TUI and the non-interactive subcommands.
package cli

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

// globalOpts holds the flags shared by every subcommand.
type globalOpts struct {
	host       string
	configPath string
	json       bool
	timeout    time.Duration
	issue      int // the TUI opens this issue alone, as in a pane of its own

	deps deps
}

// deps are the side effects the command tree needs; tests replace them.
type deps struct {
	openURL    func(url string) error
	stdin      io.Reader
	isTerminal func() bool // is stdin an interactive terminal?
	clipboard  func(context.Context) *mantis.FileUpload
}

func defaultDeps() deps {
	return deps{openURL: service.OpenBrowser, stdin: os.Stdin, isTerminal: stdinIsTerminal, clipboard: service.Clipboard}
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// NewRootCmd builds the mantis-tui command tree.
func NewRootCmd() *cobra.Command { return newRootCmd(defaultDeps()) }

func newRootCmd(d deps) *cobra.Command {
	opts := &globalOpts{deps: d}

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
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.runTUI(cmd)
		},
	}

	flags := root.PersistentFlags()
	flags.StringVar(&opts.host, "host", "", "host name from the config to use")
	flags.StringVar(&opts.configPath, "config", "", "path to config.toml (default $XDG_CONFIG_HOME/mantis-tui/config.toml)")
	flags.BoolVar(&opts.json, "json", false, "print raw API JSON instead of a table")
	flags.DurationVar(&opts.timeout, "timeout", 30*time.Second, "timeout for each API request")
	root.Flags().IntVar(&opts.issue, "issue", 0, "open only this issue; backing out of it quits")

	root.SetIn(d.stdin)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return asUsage(err) })
	root.AddCommand(
		newHostsCmd(opts), newListCmd(opts), newShowCmd(opts),
		newUpdateCmd(opts), newAssignCmd(opts),
		newMonitorCmd(opts), newUnmonitorCmd(opts), newOpenCmd(opts),
		newNoteCmd(opts), newCreateCmd(opts), newDeleteCmd(opts),
		newFilesCmd(opts), newDownloadCmd(opts),
	)
	return root
}
