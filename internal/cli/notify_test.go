package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestNotifierFor(t *testing.T) {
	found := func(string) (string, error) { return "/usr/bin/herdr", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	for _, tt := range []struct {
		name, mode string
		env        map[string]string
		look       func(string) (string, error)
		want       string
	}{
		{"auto inside herdr", "auto", map[string]string{"HERDR_ENV": "1"}, found, "herdr"},
		{"auto inside herdr via bin path", "auto", map[string]string{"HERDR_ENV": "1", "HERDR_BIN_PATH": "/opt/herdr"}, missing, "herdr"},
		{"auto inside herdr, no binary", "auto", map[string]string{"HERDR_ENV": "1"}, missing, "terminal"},
		{"auto outside herdr", "auto", nil, found, "terminal"},
		{"explicit terminal", "terminal", map[string]string{"HERDR_ENV": "1"}, found, "terminal"},
		{"explicit off", "off", map[string]string{"HERDR_ENV": "1"}, found, "off"},
		{"explicit herdr", "herdr", nil, missing, "herdr"},
	} {
		n := notifierFor(tt.mode, func(k string) string { return tt.env[k] }, tt.look)
		if n.Mode != tt.want {
			t.Errorf("%s: mode = %q, want %q", tt.name, n.Mode, tt.want)
		}
		if (n.Herdr != nil) != (tt.want == "herdr") {
			t.Errorf("%s: herdr sender set = %v", tt.name, n.Herdr != nil)
		}
	}
}

func TestHerdrArgs(t *testing.T) {
	got := strings.Join(herdrArgs("mantis-tui: 1 new on wh", "-#9 starts with a dash"), "|")
	want := "notification|show|mantis-tui: 1 new on wh|--body|-#9 starts with a dash|--sound|request"
	if got != want {
		t.Errorf("args = %s\nwant   %s", got, want)
	}
}
