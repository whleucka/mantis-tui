// Package config loads mantis-tui's TOML config, resolves host tokens from the
// environment and picks which host to use.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ErrNoConfig is returned by Load when the default config file does not exist.
var ErrNoConfig = errors.New("no config file")

// Filters are the issue-list filters Mantis supports.
var Filters = []string{"all", "assigned", "reported", "monitored", "unassigned"}

// Config is the parsed config.toml.
type Config struct {
	List  ListConfig   `toml:"list"`
	Issue IssueConfig  `toml:"issue"`
	Icons Icons        `toml:"icons"`
	UI    UIConfig     `toml:"ui"`
	Hosts []HostConfig `toml:"hosts"`

	// Warnings are non-fatal problems found while loading (e.g. an inline
	// token in a file others can read). They never contain token values.
	Warnings []string `toml:"-"`
}

// ListConfig configures the issue list.
type ListConfig struct {
	DefaultFilter  string   `toml:"default_filter"`
	PageSize       int      `toml:"page_size"`
	MaxIssues      int      `toml:"max_issues"`
	AutoRefresh    Duration `toml:"auto_refresh"`
	GroupByProject bool     `toml:"group_by_project"`
	Preview        bool     `toml:"preview"`
}

// IssueConfig configures the single-issue view.
type IssueConfig struct {
	AutoRefresh Duration `toml:"auto_refresh"`
}

// UIConfig configures the TUI as a whole.
type UIConfig struct {
	Mouse bool `toml:"mouse"`
}

// Icons are the glyphs shown for priorities and monitored issues.
type Icons struct {
	Immediate string `toml:"immediate"`
	Urgent    string `toml:"urgent"`
	High      string `toml:"high"`
	Normal    string `toml:"normal"`
	Low       string `toml:"low"`
	None      string `toml:"none"`
	Monitor   string `toml:"monitor"`
}

// HostConfig is one [[hosts]] entry as written in the file.
type HostConfig struct {
	Name    string `toml:"name"`
	URL     string `toml:"url"`
	Env     string `toml:"env"`
	Token   string `toml:"token"`
	Default bool   `toml:"default"`
}

// Duration is a time.Duration that decodes from a Go duration string.
type Duration struct{ time.Duration }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q (want e.g. \"120s\", or \"0\" to disable)", b)
	}
	if v < 0 {
		return fmt.Errorf("duration %q must not be negative", b)
	}
	d.Duration = v
	return nil
}

// Defaults returns a Config with every optional setting filled in.
func Defaults() *Config {
	return &Config{
		List: ListConfig{
			DefaultFilter: "all",
			PageSize:      100,
			MaxIssues:     1000,
			AutoRefresh:   Duration{120 * time.Second},
			Preview:       true,
		},
		Issue: IssueConfig{AutoRefresh: Duration{120 * time.Second}},
		UI:    UIConfig{Mouse: true},
		Icons: Icons{
			Immediate: "🔥",
			Urgent:    "⚠️",
			High:      "🔺",
			Normal:    "🔵",
			Low:       "🔻",
			None:      "🟣",
			Monitor:   "👁️",
		},
	}
}

// DefaultConfigPath is $XDG_CONFIG_HOME/mantis-tui/config.toml, falling back
// to ~/.config.
func DefaultConfigPath(getenv func(string) string) string {
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(getenv("HOME"), ".config")
	}
	return filepath.Join(base, "mantis-tui", "config.toml")
}

// Load reads and validates the config at path. When explicit is false (the
// default path was used) a missing file returns ErrNoConfig so the caller can
// fall back to AutoDetect.
func Load(path string, explicit bool) (*Config, error) {
	cfg := Defaults()
	md, err := toml.DecodeFile(path, cfg)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if explicit {
				return nil, fmt.Errorf("config file %s does not exist", path)
			}
			return nil, ErrNoConfig
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("%s: unknown keys: %s", path, strings.Join(keys, ", "))
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Warnings = permissionWarnings(path, cfg)
	return cfg, nil
}

func (c *Config) validate() error {
	if !slices.Contains(Filters, c.List.DefaultFilter) {
		return fmt.Errorf("list.default_filter %q must be one of %s", c.List.DefaultFilter, strings.Join(Filters, ", "))
	}
	if c.List.PageSize < 1 {
		return fmt.Errorf("list.page_size must be at least 1, got %d", c.List.PageSize)
	}
	if c.List.MaxIssues < 1 {
		return fmt.Errorf("list.max_issues must be at least 1, got %d", c.List.MaxIssues)
	}
	seen := map[string]bool{}
	for i := range c.Hosts {
		h := &c.Hosts[i]
		if h.Name == "" {
			return fmt.Errorf("hosts[%d]: name is required", i)
		}
		key := strings.ToLower(h.Name)
		if seen[key] {
			return fmt.Errorf("hosts: duplicate name %q", h.Name)
		}
		seen[key] = true
		if (h.Env == "") == (h.Token == "") {
			return fmt.Errorf("host %q: set exactly one of env or token", h.Name)
		}
		u, err := normalizeURL(h.URL)
		if err != nil {
			return fmt.Errorf("host %q: %w", h.Name, err)
		}
		h.URL = u
	}
	return nil
}

func normalizeURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("url %q must be an http(s) URL", raw)
	}
	trimmed := strings.TrimRight(raw, "/")
	if strings.HasSuffix(trimmed, "/api/rest") {
		return "", fmt.Errorf("url %q should be the Mantis base URL, without /api/rest", raw)
	}
	return trimmed, nil
}

func permissionWarnings(path string, cfg *Config) []string {
	inline := slices.ContainsFunc(cfg.Hosts, func(h HostConfig) bool { return h.Token != "" })
	if !inline {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0o077 == 0 {
		return nil
	}
	return []string{fmt.Sprintf(
		"%s contains an inline API token but is readable by other users (mode %o); run: chmod 600 %s",
		path, info.Mode().Perm(), path)}
}

// Host is a configured host whose token has been resolved.
type Host struct {
	Name    string
	URL     string
	Token   string
	Default bool
}

// DroppedHost is a configured host that cannot be used, and why.
type DroppedHost struct {
	Name   string
	URL    string
	Reason string
}

// Resolution is the result of resolving tokens for every configured host.
type Resolution struct {
	Hosts   []Host
	Dropped []DroppedHost
}

// Resolve looks up each host's token. Hosts whose env variable is unset or
// empty are dropped, mirroring mantis.nvim.
func Resolve(cfg *Config, environ []string) Resolution {
	env := envMap(environ)
	var res Resolution
	for _, hc := range cfg.Hosts {
		token := hc.Token
		if hc.Env != "" {
			token = env[hc.Env]
			if token == "" {
				res.Dropped = append(res.Dropped, DroppedHost{
					Name:   hc.Name,
					URL:    hc.URL,
					Reason: fmt.Sprintf("environment variable %s is not set", hc.Env),
				})
				continue
			}
		}
		res.Hosts = append(res.Hosts, Host{Name: hc.Name, URL: hc.URL, Token: token, Default: hc.Default})
	}
	return res
}

// reservedEnv are MANTIS_* variables that are not host tokens.
var reservedEnv = map[string]bool{"MANTIS_TUI_HOST": true, "MANTIS_IT_HOST": true}

// AutoDetect builds a config from MANTIS_<NAME> + MANTIS_<NAME>_URL pairs in
// the environment, for when no config file exists. It returns nil when no
// pair is found.
func AutoDetect(environ []string) *Config {
	env := envMap(environ)
	var names []string
	for k := range env {
		if strings.HasPrefix(k, "MANTIS_") && !strings.HasSuffix(k, "_URL") && !reservedEnv[k] {
			names = append(names, k)
		}
	}
	sort.Strings(names)

	cfg := Defaults()
	for _, k := range names {
		u, err := normalizeURL(env[k+"_URL"])
		if err != nil {
			continue
		}
		cfg.Hosts = append(cfg.Hosts, HostConfig{
			Name: strings.ToLower(strings.TrimPrefix(k, "MANTIS_")),
			URL:  u,
			Env:  k,
		})
	}
	if len(cfg.Hosts) == 0 {
		return nil
	}
	return cfg
}

// SampleConfig is shown when no config and no env hosts are found.
const SampleConfig = `# ~/.config/mantis-tui/config.toml
[[hosts]]
name = "work"
url  = "https://mantis.example.com"   # without /api/rest
env  = "MANTIS_WORK"                   # env var holding your API token
`

func envMap(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}
