package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func helpText(h *harness) string { return ansi.Strip(h.m.modal.view(200, 200)) }

func TestHelpListsEveryListBinding(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys("?")
	if _, ok := h.m.modal.(*helpModal); !ok {
		t.Fatal("? should open help")
	}
	out := helpText(h)
	for _, b := range h.m.keys.list {
		if !strings.Contains(out, b.help) || !strings.Contains(out, keyLabel(b.keys)) {
			t.Errorf("help missing %q (%s)", b.help, keyLabel(b.keys))
		}
	}
	h.keys("?")
	if h.m.modal != nil {
		t.Error("? again closes help")
	}
}

func TestHelpListsEveryIssueBinding(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.keys("enter", "?")
	out := helpText(h)
	for _, b := range h.m.keys.issue {
		if !strings.Contains(out, b.help) || !strings.Contains(out, keyLabel(b.keys)) {
			t.Errorf("issue help missing %q (%s)", b.help, keyLabel(b.keys))
		}
	}
	h.keys("esc")
	if h.m.modal != nil {
		t.Error("esc closes help")
	}
}

func TestKeyLabel(t *testing.T) {
	for in, want := range map[string]string{"g g": "gg", "b s": "bs", "ctrl+h": "ctrl+h"} {
		if got := keyLabel([]string{in}); got != want {
			t.Errorf("keyLabel(%q) = %q, want %q", in, got, want)
		}
	}
	if got := keyLabel([]string{"k", "up"}); got != "k/up" {
		t.Errorf("alternatives: %q", got)
	}
}

func TestHelpFitsSmallTerminalsByScrolling(t *testing.T) {
	h := listHarness(t, 3, nil)
	h.send(winSize(60, 15))
	h.keys("?")
	first := ansi.Strip(h.m.modal.view(60, 15))
	if lines := strings.Count(first, "\n") + 1; lines > 15 {
		t.Errorf("help is %d lines in a 15-line terminal", lines)
	}
	h.keys("j", "j")
	if ansi.Strip(h.m.modal.view(60, 15)) == first {
		t.Error("j should scroll the help")
	}
}

// The README's key tables must cover every binding.
func TestReadmeDocumentsEveryKey(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	km := defaultKeymap()
	for _, b := range append(km.list, km.issue...) {
		for _, k := range b.keys {
			label := "`" + keyLabel([]string{k}) + "`"
			if !strings.Contains(string(readme), label) {
				t.Errorf("README does not document %s (%s)", label, b.help)
			}
		}
	}
}
