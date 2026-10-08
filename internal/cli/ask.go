package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// runFunc runs a program and returns its stdout.
type runFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, err
}

// paneRunnerFor returns a function that runs a shell command in a new herdr
// pane split off this one, or nil outside herdr.
func paneRunnerFor(getenv func(string) string, lookPath func(string) (string, error), cwd string, run runFunc) func(ctx context.Context, command string, right bool) error {
	bin := herdrBin(getenv, lookPath)
	if getenv("HERDR_ENV") != "1" || bin == "" {
		return nil
	}
	return func(ctx context.Context, command string, right bool) error {
		dir := "down"
		if right {
			dir = "right"
		}
		args := []string{"pane", "split", "--current", "--direction", dir, "--focus"}
		if cwd != "" {
			args = append(args, "--cwd", cwd)
		}
		out, err := run(ctx, bin, args...)
		if err != nil {
			return fmt.Errorf("herdr pane split: %w", err)
		}
		var resp struct {
			Result struct {
				Pane struct {
					PaneID string `json:"pane_id"`
				} `json:"pane"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &resp); err != nil || resp.Result.Pane.PaneID == "" {
			return errors.New("herdr pane split: no pane id in its response")
		}
		if _, err := run(ctx, bin, "pane", "run", resp.Result.Pane.PaneID, command); err != nil {
			return fmt.Errorf("herdr pane run: %w", err)
		}
		return nil
	}
}
