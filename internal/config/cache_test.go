package config

import "testing"

func TestDefaultFilesDir(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := DefaultFilesDir(env(map[string]string{"XDG_CACHE_HOME": "/x/cache", "HOME": "/home/u"})); got != "/x/cache/mantis-tui/files" {
		t.Errorf("with XDG_CACHE_HOME: %s", got)
	}
	if got := DefaultFilesDir(env(map[string]string{"HOME": "/home/u"})); got != "/home/u/.cache/mantis-tui/files" {
		t.Errorf("fallback: %s", got)
	}
}
