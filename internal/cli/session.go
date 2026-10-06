package cli

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
)

// session is everything a subcommand needs to talk to the selected host.
type session struct {
	cfg    *config.Config
	host   config.Host
	client *mantis.Client
}

// openSession loads config and picks a host. The CLI reads the last-used
// host from the state file but never writes it.
func (o *globalOpts) openSession(cmd *cobra.Command) (*session, error) {
	cfg, res, err := o.loadConfig(cmd.ErrOrStderr())
	if err != nil {
		return nil, asUsage(err)
	}
	for _, d := range res.Dropped {
		if o.host != "" && strings.EqualFold(d.Name, o.host) {
			return nil, usageErrorf("host %q is unavailable: %s", d.Name, d.Reason)
		}
	}
	st, _ := config.LoadState(config.DefaultStatePath(os.Getenv))
	host, err := config.Select(res.Hosts, config.SelectInput{
		Flag:     o.host,
		EnvHost:  os.Getenv("MANTIS_TUI_HOST"),
		LastUsed: st.LastHost,
	})
	if err != nil {
		if len(res.Hosts) == 0 && len(res.Dropped) > 0 {
			return nil, usageErrorf("%w: every host was dropped (%s: %s)", err, res.Dropped[0].Name, res.Dropped[0].Reason)
		}
		return nil, asUsage(err)
	}
	return &session{cfg: cfg, host: host, client: mantis.NewClient(host.URL, host.Token)}, nil
}

// ctx returns a context bounded by --timeout.
func (o *globalOpts) ctx(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	return context.WithTimeout(cmd.Context(), o.timeout)
}

// exactArgs is cobra.ExactArgs with the error marked as a usage error.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		return asUsage(cobra.ExactArgs(n)(cmd, args))
	}
}

func parseID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, usageErrorf("invalid issue id %q", s)
	}
	return id, nil
}
