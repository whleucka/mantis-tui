package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func TestStartsWithPickerWhenNoInitialHost(t *testing.T) {
	h := newHarness(t, nil, nil)
	out := h.view()
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") || !strings.Contains(out, "Select a host") {
		t.Fatalf("picker not shown:\n%s", out)
	}
	h.keys("down", "enter")
	if h.m.cur == nil || h.m.cur.sess.Host.Name != "beta" {
		t.Fatalf("current host = %+v", h.m.cur)
	}
	if len(h.saved) != 1 || h.saved[0] != "beta" {
		t.Errorf("SaveLastHost calls = %v, want [beta]", h.saved)
	}
	if !strings.Contains(h.view(), "beta") {
		t.Error("status bar should show the host")
	}
}

func TestEscOnStartupPickerQuits(t *testing.T) {
	h := newHarness(t, nil, nil)
	h.keys("esc")
	if !h.quit {
		t.Error("esc with no host chosen should quit")
	}
}

func TestInitialHostSkipsPicker(t *testing.T) {
	hosts := testHosts()
	h := newHarness(t, &hosts[0], nil)
	if h.m.modal != nil {
		t.Fatal("no picker expected")
	}
	if h.m.cur.sess.Host.Name != "alpha" || len(h.saved) != 1 || h.saved[0] != "alpha" {
		t.Errorf("cur = %v, saved = %v", h.m.cur.sess.Host.Name, h.saved)
	}
}

func TestSwitchHostKeepsPerHostState(t *testing.T) {
	hosts := testHosts()
	h := newHarness(t, &hosts[0], nil)
	alpha := h.m.cur

	h.keys("ctrl+h")
	if h.m.modal == nil {
		t.Fatal("ctrl+h should open the host picker")
	}
	h.keys("down", "enter")
	if h.m.cur.sess.Host.Name != "beta" {
		t.Fatalf("switched to %q", h.m.cur.sess.Host.Name)
	}

	h.keys("ctrl+h", "up", "enter")
	if h.m.cur != alpha {
		t.Error("switching back should restore alpha's existing state")
	}
	if strings.Join(h.saved, ",") != "alpha,beta,alpha" {
		t.Errorf("saved = %v", h.saved)
	}
}

func TestErrorsShowInStatusBarWithoutTokens(t *testing.T) {
	hosts := testHosts()
	h := newHarness(t, &hosts[0], nil)
	h.send(errMsg{host: "alpha", err: errors.New("request failed for token-alpha-secret")})
	out := h.view()
	if !strings.Contains(out, "request failed") {
		t.Errorf("error not shown:\n%s", out)
	}
	if strings.Contains(out, "token-alpha-secret") {
		t.Error("status bar leaks a token")
	}
	h.keys("j") // next action clears it
	if strings.Contains(h.view(), "request failed") {
		t.Error("error should clear on the next key")
	}
}

func TestStaleHostMessagesAreDropped(t *testing.T) {
	hosts := testHosts()
	h := newHarness(t, &hosts[0], nil)
	h.send(errMsg{host: "beta", err: errors.New("from another host")})
	if strings.Contains(h.view(), "from another host") {
		t.Error("message for a non-current host should be ignored")
	}
}

func TestQuitKey(t *testing.T) {
	hosts := testHosts()
	h := newHarness(t, &hosts[0], nil)
	h.keys("q")
	if !h.quit {
		t.Error("q should quit from the list")
	}
}

func TestCtrlCAlwaysQuits(t *testing.T) {
	h := newHarness(t, nil, nil) // even with the picker open
	h.keys("ctrl+c")
	if !h.quit {
		t.Error("ctrl+c should quit")
	}
}

func TestChordTimeoutViaMessage(t *testing.T) {
	hosts := testHosts()
	h := newHarness(t, &hosts[0], nil)
	h.keys("b")
	if len(h.m.chord.pending) != 1 {
		t.Fatal("b should start a chord")
	}
	if !strings.Contains(h.view(), "b-") {
		t.Error("pending chord should be shown in the status bar")
	}
	h.send(chordTimeoutMsg{gen: h.m.chord.gen})
	if len(h.m.chord.pending) != 0 {
		t.Error("timeout should clear the chord")
	}
}

func TestSaveLastHostFailureIsAWarningNotFatal(t *testing.T) {
	hosts := testHosts()
	h := newHarness(t, nil, nil)
	h.m.opts.SaveLastHost = func(string) error { return errors.New("read-only fs") }
	h.keys("enter")
	if h.m.cur == nil || h.m.cur.sess.Host.Name != hosts[0].Name {
		t.Fatal("host should still be selected")
	}
	if !strings.Contains(h.view(), "read-only fs") {
		t.Error("save failure should be reported")
	}
}

var _ = mantistest.Fake{}
