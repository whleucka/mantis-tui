package cli

import (
	"context"
	"os/exec"

	"github.com/whleucka/mantis-tui/internal/tui"
)

// notifierFor resolves the [ui] notify mode. "auto" uses herdr inside herdr
// (HERDR_ENV=1 and the binary can be found), and the terminal otherwise.
func notifierFor(mode string, getenv func(string) string, lookPath func(string) (string, error)) tui.Notifier {
	herdrBin := func() string {
		if p := getenv("HERDR_BIN_PATH"); p != "" {
			return p
		}
		if p, err := lookPath("herdr"); err == nil {
			return p
		}
		return ""
	}
	if mode == "auto" {
		mode = "terminal"
		if getenv("HERDR_ENV") == "1" && herdrBin() != "" {
			mode = "herdr"
		}
	}
	n := tui.Notifier{Mode: mode}
	if mode == "herdr" {
		bin := herdrBin()
		if bin == "" {
			bin = "herdr" // let the run fail visibly in the status bar
		}
		n.Herdr = func(ctx context.Context, title, body string) error {
			return exec.CommandContext(ctx, bin, herdrArgs(title, body)...).Run()
		}
	}
	return n
}

// herdrArgs is the herdr command line. herdr wants the title first and
// takes --body's text as the next argument; it has no "--" separator.
func herdrArgs(title, body string) []string {
	return []string{"notification", "show", title, "--body", body, "--sound", "request"}
}
