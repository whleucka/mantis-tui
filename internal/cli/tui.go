package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
	"github.com/whleucka/mantis-tui/internal/tui"
)

// runTUI starts the interactive UI. Unlike the CLI, it shows a host picker
// when the selection order does not settle on one host.
func (o *globalOpts) runTUI(cmd *cobra.Command) error {
	cfg, res, err := o.loadConfig(cmd.ErrOrStderr())
	if err != nil {
		return asUsage(err)
	}
	var initial *config.Host
	host, err := o.selectHost(res)
	switch {
	case err == nil:
		initial = &host
	case errors.Is(err, config.ErrNeedPicker) && o.issue == 0:
		// leave initial nil: the TUI asks
	case errors.Is(err, config.ErrNeedPicker):
		return usageErrorf("--issue needs a host: pass --host")
	default:
		return err
	}

	statePath := config.DefaultStatePath(os.Getenv)
	cwd, _ := os.Getwd()
	opts := tui.Options{
		Config:  cfg,
		Hosts:   res.Hosts,
		Initial: initial,
		NewSession: func(h config.Host) *tui.Session {
			return tui.NewSession(h, mantis.NewClient(h.URL, h.Token))
		},
		OpenURL:      o.deps.openURL,
		Seen:         config.LoadSeen(config.DefaultSeenPath(os.Getenv)),
		Notify:       notifierFor(cfg.UI.Notify, os.Getenv, exec.LookPath),
		RunInPane:    paneRunnerFor(os.Getenv, exec.LookPath, cwd, execRun),
		IssueCommand: o.issueCommand(),
		FilesDir:     config.DefaultFilesDir(os.Getenv),
		OpenFile:     service.OpenFile,
		ShowImage:    imageShowerFor(exec.LookPath),
		Clipboard:    service.Clipboard,
		InlineImages: true,
		SaveLastHost: func(name string) error {
			st, _ := config.LoadState(statePath)
			st.LastHost = name
			return config.SaveState(statePath, st)
		},
	}
	if o.issue > 0 {
		// The instance that opened this pane owns the shared state: read
		// marks, the last host and new-issue alerts.
		opts.Issue = o.issue
		opts.Seen, opts.Notify, opts.SaveLastHost = nil, tui.Notifier{}, nil
	}
	model := tui.New(opts)
	_, err = tea.NewProgram(model, tea.WithContext(cmd.Context())).Run()
	_, _ = os.Stdout.WriteString(model.ImageCleanup()) // free the thumbnails in the terminal
	return err
}

// imageShowerFor shows images with kitten icat when kitten is installed.
// Whether the terminal supports it is checked each time an image opens.
func imageShowerFor(lookPath func(string) (string, error)) tui.ImageShower {
	if path, err := lookPath("kitten"); err == nil {
		return tui.KittyImages(path)
	}
	return nil
}

// issueCommand is the shell command that runs this mantis-tui on one issue
// with the same config file. exec lets the pane close when it quits.
func (o *globalOpts) issueCommand() func(host string, id int) string {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	return issueCommandFor(self, o.configPath)
}

func issueCommandFor(self, configPath string) func(host string, id int) string {
	prefix := "exec " + shellQuote(self)
	if configPath != "" {
		if abs, err := filepath.Abs(configPath); err == nil {
			configPath = abs
		}
		prefix += " --config " + shellQuote(configPath)
	}
	return func(host string, id int) string {
		return fmt.Sprintf("%s --host %s --issue %d", prefix, shellQuote(host), id)
	}
}

// shellQuote quotes s as one POSIX shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
