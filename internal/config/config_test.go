package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

const twoHosts = `
[[hosts]]
name = "chainlogic"
url  = "https://mantis.chainlogic.it"
env  = "MANTIS_CL"

[[hosts]]
name = "williamhleucka"
url  = "https://mantis.williamhleucka.com/"
env  = "MANTIS_WH"
`

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, twoHosts, 0o600), true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.List.DefaultFilter != "all" || cfg.List.PageSize != 50 {
		t.Errorf("list defaults = %+v", cfg.List)
	}
	if cfg.List.AutoRefresh.Duration != 120*time.Second || cfg.Issue.AutoRefresh.Duration != 120*time.Second {
		t.Errorf("auto refresh defaults = %v / %v", cfg.List.AutoRefresh, cfg.Issue.AutoRefresh)
	}
	if cfg.Icons.Immediate != "🔥" || cfg.Icons.Monitor != "👁️" {
		t.Errorf("icon defaults = %+v", cfg.Icons)
	}
	if !cfg.List.Preview || !cfg.UI.Mouse {
		t.Error("list.preview and ui.mouse should default to true")
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	body := `
[list]
default_filter = "assigned"
page_size = 500
auto_refresh = "0"
group_by_project = true
preview = false

[ui]
mouse = false

[icons]
high = "H"
` + twoHosts
	cfg, err := Load(writeConfig(t, body, 0o600), true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.List.DefaultFilter != "assigned" || cfg.List.PageSize != 500 || !cfg.List.GroupByProject || cfg.List.Preview {
		t.Errorf("list = %+v", cfg.List)
	}
	if cfg.List.AutoRefresh.Duration != 0 {
		t.Errorf("auto_refresh = %v, want 0 (disabled)", cfg.List.AutoRefresh)
	}
	if cfg.UI.Mouse {
		t.Error("ui.mouse = false should disable the mouse")
	}
	if cfg.Icons.High != "H" || cfg.Icons.Low != "🔻" {
		t.Errorf("icons = %+v, want High overridden and Low default", cfg.Icons)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := map[string]string{
		"bad filter":      "[list]\ndefault_filter = \"mine\"\n" + twoHosts,
		"bad page size":   "[list]\npage_size = 0\n" + twoHosts,
		"bad duration":    "[list]\nauto_refresh = \"soon\"\n" + twoHosts,
		"missing url":     "[[hosts]]\nname = \"x\"\nenv = \"MANTIS_X\"\n",
		"non-http url":    "[[hosts]]\nname = \"x\"\nurl = \"ftp://x\"\nenv = \"MANTIS_X\"\n",
		"api/rest in url": "[[hosts]]\nname = \"x\"\nurl = \"https://x/api/rest\"\nenv = \"MANTIS_X\"\n",
		"no token source": "[[hosts]]\nname = \"x\"\nurl = \"https://x\"\n",
		"both token+env":  "[[hosts]]\nname = \"x\"\nurl = \"https://x\"\nenv = \"A\"\ntoken = \"b\"\n",
		"missing name":    "[[hosts]]\nurl = \"https://x\"\nenv = \"MANTIS_X\"\n",
		"duplicate names": "[[hosts]]\nname = \"x\"\nurl = \"https://x\"\nenv = \"A\"\n[[hosts]]\nname = \"x\"\nurl = \"https://y\"\nenv = \"B\"\n",
		"malformed toml":  "[[hosts\n",
		"unknown top key": "colour = \"red\"\n" + twoHosts,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, body, 0o600), true); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.toml")

	if _, err := Load(missing, true); err == nil {
		t.Error("explicit --config path that does not exist should be an error")
	}

	cfg, err := Load(missing, false)
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("default path missing: err = %v, want ErrNoConfig", err)
	}
	if cfg != nil {
		t.Error("expected nil config when file is missing")
	}
}

func TestResolveDropsHostsWithUnsetEnv(t *testing.T) {
	cfg, err := Load(writeConfig(t, twoHosts, 0o600), true)
	if err != nil {
		t.Fatal(err)
	}

	res := Resolve(cfg, []string{"MANTIS_WH=tok-wh", "MANTIS_CL="})

	if len(res.Hosts) != 1 || res.Hosts[0].Name != "williamhleucka" {
		t.Fatalf("hosts = %+v, want only williamhleucka", res.Hosts)
	}
	if res.Hosts[0].Token != "tok-wh" {
		t.Errorf("token not resolved from env")
	}
	if res.Hosts[0].URL != "https://mantis.williamhleucka.com" {
		t.Errorf("url = %q, want trailing slash trimmed", res.Hosts[0].URL)
	}
	if len(res.Dropped) != 1 || res.Dropped[0].Name != "chainlogic" || !strings.Contains(res.Dropped[0].Reason, "MANTIS_CL") {
		t.Errorf("dropped = %+v, want chainlogic with a reason naming MANTIS_CL", res.Dropped)
	}
}

func TestResolveInlineToken(t *testing.T) {
	body := "[[hosts]]\nname = \"x\"\nurl = \"https://x\"\ntoken = \"inline\"\n"
	cfg, err := Load(writeConfig(t, body, 0o600), true)
	if err != nil {
		t.Fatal(err)
	}
	res := Resolve(cfg, nil)
	if len(res.Hosts) != 1 || res.Hosts[0].Token != "inline" {
		t.Fatalf("hosts = %+v", res.Hosts)
	}
}

func TestPermissionWarning(t *testing.T) {
	inline := "[[hosts]]\nname = \"x\"\nurl = \"https://x\"\ntoken = \"inline\"\n"
	tests := []struct {
		name string
		body string
		mode os.FileMode
		warn bool
	}{
		{"inline token, world readable", inline, 0o644, true},
		{"inline token, private", inline, 0o600, false},
		{"env token, world readable", twoHosts, 0o644, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.body, tt.mode)
			if err := os.Chmod(path, tt.mode); err != nil { // defeat umask
				t.Fatal(err)
			}
			cfg, err := Load(path, true)
			if err != nil {
				t.Fatal(err)
			}
			got := len(cfg.Warnings) > 0
			if got != tt.warn {
				t.Errorf("warnings = %v, want warning=%v", cfg.Warnings, tt.warn)
			}
			for _, w := range cfg.Warnings {
				if strings.Contains(w, "inline") && strings.Contains(w, "token = ") {
					t.Errorf("warning leaks token value: %q", w)
				}
			}
		})
	}
}

func TestAutoDetect(t *testing.T) {
	environ := []string{
		"MANTIS_WH=tok-wh",
		"MANTIS_WH_URL=https://mantis.williamhleucka.com/",
		"MANTIS_CL=tok-cl", // no URL: ignored
		"MANTIS_TUI_HOST=wh",
		"MANTIS_IT_HOST=wh",
		"HOME=/home/x",
	}
	cfg := AutoDetect(environ)
	if cfg == nil {
		t.Fatal("expected a detected config")
	}
	res := Resolve(cfg, environ)
	if len(res.Hosts) != 1 {
		t.Fatalf("hosts = %+v, want only wh", res.Hosts)
	}
	h := res.Hosts[0]
	if h.Name != "wh" || h.URL != "https://mantis.williamhleucka.com" || h.Token != "tok-wh" {
		t.Errorf("host = %+v", h)
	}
	if cfg.List.PageSize != 50 {
		t.Errorf("auto-detected config should carry defaults, got %+v", cfg.List)
	}
}

func TestAutoDetectNothing(t *testing.T) {
	if cfg := AutoDetect([]string{"MANTIS_CL=tok", "HOME=/x"}); cfg != nil {
		t.Errorf("expected nil, got %+v", cfg)
	}
}

func TestDefaultPaths(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	if got := DefaultConfigPath(env(map[string]string{"XDG_CONFIG_HOME": "/xdg", "HOME": "/h"})); got != "/xdg/mantis-tui/config.toml" {
		t.Errorf("config path = %q", got)
	}
	if got := DefaultConfigPath(env(map[string]string{"HOME": "/h"})); got != "/h/.config/mantis-tui/config.toml" {
		t.Errorf("config path fallback = %q", got)
	}
	if got := DefaultStatePath(env(map[string]string{"XDG_STATE_HOME": "/st", "HOME": "/h"})); got != "/st/mantis-tui/state.toml" {
		t.Errorf("state path = %q", got)
	}
	if got := DefaultStatePath(env(map[string]string{"HOME": "/h"})); got != "/h/.local/state/mantis-tui/state.toml" {
		t.Errorf("state path fallback = %q", got)
	}
}
