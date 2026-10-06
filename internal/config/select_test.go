package config

import (
	"errors"
	"strings"
	"testing"
)

func hosts(names ...string) []Host {
	out := make([]Host, len(names))
	for i, n := range names {
		out[i] = Host{Name: n, URL: "https://" + n, Token: "t"}
	}
	return out
}

func TestSelectOrder(t *testing.T) {
	withDefault := hosts("a", "b", "c")
	withDefault[2].Default = true

	tests := []struct {
		name  string
		hosts []Host
		in    SelectInput
		want  string
	}{
		{"flag wins over everything", withDefault, SelectInput{Flag: "a", EnvHost: "b", LastUsed: "b"}, "a"},
		{"env beats default", withDefault, SelectInput{EnvHost: "b", LastUsed: "a"}, "b"},
		{"default beats last used", withDefault, SelectInput{LastUsed: "a"}, "c"},
		{"only host", hosts("solo"), SelectInput{LastUsed: "gone"}, "solo"},
		{"last used when no default", hosts("a", "b"), SelectInput{LastUsed: "b"}, "b"},
		{"flag is case-insensitive", hosts("Work", "b"), SelectInput{Flag: "work"}, "Work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Select(tt.hosts, tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tt.want {
				t.Errorf("selected %q, want %q", got.Name, tt.want)
			}
		})
	}
}

func TestSelectNeedsPicker(t *testing.T) {
	_, err := Select(hosts("a", "b"), SelectInput{LastUsed: "stale"})
	if !errors.Is(err, ErrNeedPicker) {
		t.Fatalf("err = %v, want ErrNeedPicker", err)
	}
	if !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Errorf("error should list host names for the CLI: %v", err)
	}
}

func TestSelectUnknownFlagOrEnvHost(t *testing.T) {
	for _, in := range []SelectInput{{Flag: "zzz"}, {EnvHost: "zzz"}} {
		_, err := Select(hosts("a", "b"), in)
		if err == nil || errors.Is(err, ErrNeedPicker) {
			t.Fatalf("input %+v: err = %v, want unknown-host error", in, err)
		}
		if !strings.Contains(err.Error(), "zzz") || !strings.Contains(err.Error(), "a, b") {
			t.Errorf("error should name the bad host and list valid ones: %v", err)
		}
	}
}

func TestSelectNoHosts(t *testing.T) {
	if _, err := Select(nil, SelectInput{}); !errors.Is(err, ErrNoHosts) {
		t.Fatalf("err = %v, want ErrNoHosts", err)
	}
}
