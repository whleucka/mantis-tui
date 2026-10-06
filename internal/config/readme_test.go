package config

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The README's sample config must stay valid.
func TestReadmeSampleConfigParses(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("(?s)```toml\n(.*?)```").FindSubmatch(readme)
	if m == nil {
		t.Fatal("README has no ```toml block")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, m[1], 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("README sample config does not load: %v", err)
	}
	if len(cfg.Hosts) == 0 {
		t.Error("sample config should define hosts")
	}
}
