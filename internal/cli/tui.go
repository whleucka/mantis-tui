package cli

import (
	"errors"
	"os"
	"os/exec"

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
	case errors.Is(err, config.ErrNeedPicker):
		// leave initial nil: the TUI asks
	default:
		return err
	}

	statePath := config.DefaultStatePath(os.Getenv)
	cwd, _ := os.Getwd()
	model := tui.New(tui.Options{
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
	})
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
