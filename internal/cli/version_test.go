package cli

import (
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	out, _, err := runCLI(t, "--version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "mantis-tui version ") {
		t.Fatalf("--version printed %q", out)
	}
}
