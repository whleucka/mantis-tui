package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/whleucka/mantis-tui/internal/service"
)

// batchError reports a partially failed batch; it unwraps to the first
// failure so ExitCode reflects its class.
type batchError struct {
	failed, total int
	first         error
}

func (e *batchError) Error() string {
	return fmt.Sprintf("%d of %d failed: %v", e.failed, e.total, e.first)
}

func (e *batchError) Unwrap() error { return e.first }

type batchRow struct {
	ID    int    `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// runBatch applies op to every id (each call bounded by --timeout), prints a
// line or JSON row per id, and returns a batchError if any failed.
func (o *globalOpts) runBatch(cmd *cobra.Command, ids []int, verb string, op func(ctx context.Context, id int) error) error {
	results := service.Batch(cmd.Context(), ids, func(ctx context.Context, id int) error {
		ctx, cancel := context.WithTimeout(ctx, o.timeout)
		defer cancel()
		return op(ctx, id)
	})

	out := cmd.OutOrStdout()
	rows := make([]batchRow, len(results))
	var first error
	failed := 0
	for i, r := range results {
		rows[i] = batchRow{ID: r.ID, OK: r.Err == nil}
		if r.Err != nil {
			rows[i].Error = r.Err.Error()
			failed++
			if first == nil {
				first = r.Err
			}
		}
	}

	if o.json {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			return err
		}
	} else {
		for _, r := range rows {
			if r.OK {
				fmt.Fprintf(out, "#%d %s\n", r.ID, verb)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "#%d failed: %s\n", r.ID, r.Error)
			}
		}
	}
	if failed > 0 {
		return &batchError{failed: failed, total: len(ids), first: first}
	}
	return nil
}

func parseIDs(args []string) ([]int, error) {
	ids := make([]int, len(args))
	for i, a := range args {
		id, err := parseID(a)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

// minArgs is cobra.MinimumNArgs with the error marked as a usage error.
func minArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		return asUsage(cobra.MinimumNArgs(n)(cmd, args))
	}
}
