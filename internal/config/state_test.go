package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.toml")

	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("missing state file should not be an error: %v", err)
	}
	if st.LastHost != "" {
		t.Errorf("empty state expected, got %+v", st)
	}

	if err := SaveState(path, State{LastHost: "williamhleucka"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("state file mode = %o, want 600", perm)
	}

	st, err = LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastHost != "williamhleucka" {
		t.Errorf("LastHost = %q", st.LastHost)
	}
}

func TestLoadStateCorruptIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	if err := os.WriteFile(path, []byte("not = [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("corrupt state should be ignored, got %v", err)
	}
	if st.LastHost != "" {
		t.Errorf("got %+v", st)
	}
}
