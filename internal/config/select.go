package config

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNoHosts means no usable host is configured.
	ErrNoHosts = errors.New("no usable Mantis hosts configured")
	// ErrNeedPicker means several hosts qualify and the user must choose.
	ErrNeedPicker = errors.New("multiple hosts configured")
)

// SelectInput carries the sources that can name a host, highest priority first.
type SelectInput struct {
	Flag     string // --host
	EnvHost  string // MANTIS_TUI_HOST
	LastUsed string // from the state file
}

// Select picks a host in the order: flag → MANTIS_TUI_HOST → default = true →
// the only host → last used. Otherwise it returns ErrNeedPicker, wrapped with
// the list of host names so CLI callers can print it as-is.
func Select(hosts []Host, in SelectInput) (Host, error) {
	if len(hosts) == 0 {
		return Host{}, ErrNoHosts
	}
	for _, src := range []struct{ name, label string }{
		{in.Flag, "--host"},
		{in.EnvHost, "MANTIS_TUI_HOST"},
	} {
		if src.name == "" {
			continue
		}
		if h, ok := find(hosts, src.name); ok {
			return h, nil
		}
		return Host{}, fmt.Errorf("%s: unknown host %q (available: %s)", src.label, src.name, names(hosts))
	}
	for _, h := range hosts {
		if h.Default {
			return h, nil
		}
	}
	if len(hosts) == 1 {
		return hosts[0], nil
	}
	if h, ok := find(hosts, in.LastUsed); ok && in.LastUsed != "" {
		return h, nil
	}
	return Host{}, fmt.Errorf("%w; pick one with --host (available: %s)", ErrNeedPicker, names(hosts))
}

func find(hosts []Host, name string) (Host, bool) {
	for _, h := range hosts {
		if strings.EqualFold(h.Name, name) {
			return h, true
		}
	}
	return Host{}, false
}

func names(hosts []Host) string {
	out := make([]string, len(hosts))
	for i, h := range hosts {
		out[i] = h.Name
	}
	return strings.Join(out, ", ")
}
