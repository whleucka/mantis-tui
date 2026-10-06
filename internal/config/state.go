package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// State is what the TUI remembers between runs.
type State struct {
	LastHost string `toml:"last_host"`
}

// DefaultStatePath is $XDG_STATE_HOME/mantis-tui/state.toml, falling back to
// ~/.local/state.
func DefaultStatePath(getenv func(string) string) string {
	base := getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(getenv("HOME"), ".local", "state")
	}
	return filepath.Join(base, "mantis-tui", "state.toml")
}

// LoadState reads the state file. A missing or corrupt file yields an empty
// State: losing the last-used host is harmless.
func LoadState(path string) (State, error) {
	var st State
	if _, err := toml.DecodeFile(path, &st); err != nil {
		return State{}, nil //nolint:nilerr // missing or corrupt state is deliberately ignored
	}
	return st, nil
}

// SaveState writes the state file with owner-only permissions.
func SaveState(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := toml.NewEncoder(f).Encode(st); err != nil {
		_ = f.Close()
		return fmt.Errorf("write state: %w", err)
	}
	return f.Close()
}
