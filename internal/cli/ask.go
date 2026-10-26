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

// paneRunner runs a shell command in a new pane named label ("" leaves it
// unnamed), to the right or below.
type paneRunner func(ctx context.Context, command, label string, right bool) error

// paneRunnerFor returns a function that runs a shell command in a new pane
// split off this one inside herdr, else in a new kitty OS window when kitty's
// remote control is listening, else nil.
func paneRunnerFor(getenv func(string) string, lookPath func(string) (string, error), cwd string, run runFunc) paneRunner {
	// herdr first: inside herdr in kitty, kitty's variables are set too.
	if bin := herdrBin(getenv, lookPath); getenv("HERDR_ENV") == "1" && bin != "" {
		return herdrPaneRunner(bin, cwd, run)
	}
	if to := getenv("KITTY_LISTEN_ON"); to != "" {
		if bin, err := lookPath("kitten"); err == nil {
			return kittyPaneRunner(bin, to, getenv, cwd, run)
		}
	}
	return nil
}

func herdrPaneRunner(bin, cwd string, run runFunc) paneRunner {
	return func(ctx context.Context, command, label string, right bool) error {
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
		id := resp.Result.Pane.PaneID
		if label != "" {
			// Best effort: an unnamed pane still does its job.
			_, _ = run(ctx, bin, "pane", "rename", id, label)
		}
		if _, err := run(ctx, bin, "pane", "run", id, command); err != nil {
			return fmt.Errorf("herdr pane run: %w", err)
		}
		return nil
	}
}

// kittyPaneRunner opens a new kitty OS window through the remote control
// socket at to; right doesn't apply. kitty starts it with its own
// environment, not this process's, so the command runs under an interactive
// $SHELL, which reads the user's shell startup files (where PATH and host
// tokens are usually exported) as a herdr pane would. A plain -c shell skips
// ~/.zshrc and ~/.bashrc.
func kittyPaneRunner(bin, to string, getenv func(string) string, cwd string, run runFunc) paneRunner {
	shell := getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return func(ctx context.Context, command, label string, _ bool) error {
		args := []string{"@", "--to", to, "launch", "--type", "os-window"}
		if cwd != "" {
			args = append(args, "--cwd", cwd)
		}
		if label != "" {
			args = append(args, "--os-window-title", label)
		}
		args = append(args, shell, "-ic", command)
		if _, err := run(ctx, bin, args...); err != nil {
			return fmt.Errorf("kitten @ launch: %w", err)
		}
		return nil
	}
}
