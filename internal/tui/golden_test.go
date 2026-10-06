package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Render times in UTC so goldens do not depend on the machine's zone.
func init() { time.Local = time.UTC }

// golden compares the ANSI-stripped view with testdata/<name>.golden.
func golden(t *testing.T, name, view string) {
	t.Helper()
	var lines []string
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		lines = append(lines, strings.TrimRight(l, " "))
	}
	got := strings.Join(lines, "\n") + "\n"
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run go test ./internal/tui -update): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("view differs from %s (run with -update after checking):\n--- got ---\n%s", path, got)
	}
}
