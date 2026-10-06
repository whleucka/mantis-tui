package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type hostRow struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Default bool   `json:"default"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
}

func newHostsCmd(opts *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "hosts",
		Short: "List configured hosts and whether they are usable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, res, err := opts.loadConfig(cmd.ErrOrStderr())
			if err != nil {
				return err
			}

			dropped := map[string]string{}
			for _, d := range res.Dropped {
				dropped[d.Name] = d.Reason
			}
			rows := make([]hostRow, 0, len(cfg.Hosts))
			for _, h := range cfg.Hosts {
				row := hostRow{Name: h.Name, URL: h.URL, Default: h.Default, Status: "active"}
				if reason, ok := dropped[h.Name]; ok {
					row.Status, row.Reason = "dropped", reason
				}
				rows = append(rows, row)
			}

			out := cmd.OutOrStdout()
			if opts.json {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}

			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tURL\tSTATUS")
			for _, r := range rows {
				status := r.Status
				if r.Default {
					status += " (default)"
				}
				if r.Reason != "" {
					status += ": " + r.Reason
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.URL, status)
			}
			return tw.Flush()
		},
	}
}
