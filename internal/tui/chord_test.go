package tui

import "testing"

func testBindings() []binding {
	return []binding{
		{action: actTop, keys: []string{"g g"}},
		{action: actBottom, keys: []string{"G"}},
		{action: actDeleteNote, keys: []string{"d n"}},
		{action: actBatchStatus, keys: []string{"b s"}},
		{action: actStatus, keys: []string{"s"}},
		{action: actDown, keys: []string{"j", "down"}},
	}
}

func TestChordSingleKey(t *testing.T) {
	var c chord
	if a, wait := c.feed("s", testBindings()); a != actStatus || wait {
		t.Errorf("got %q wait=%v", a, wait)
	}
	if a, _ := c.feed("down", testBindings()); a != actDown {
		t.Errorf("alternate key: got %q", a)
	}
}

func TestChordSequences(t *testing.T) {
	for _, tt := range []struct {
		keys []string
		want action
	}{
		{[]string{"g", "g"}, actTop},
		{[]string{"d", "n"}, actDeleteNote},
		{[]string{"b", "s"}, actBatchStatus},
	} {
		var c chord
		a, wait := c.feed(tt.keys[0], testBindings())
		if a != "" || !wait {
			t.Fatalf("%v: first key should wait, got %q wait=%v", tt.keys, a, wait)
		}
		a, wait = c.feed(tt.keys[1], testBindings())
		if a != tt.want || wait {
			t.Errorf("%v: got %q wait=%v, want %q", tt.keys, a, wait, tt.want)
		}
	}
}

func TestChordUnknownSecondKeyFallsThrough(t *testing.T) {
	var c chord
	c.feed("b", testBindings())
	// "b j" is not a sequence: the pending "b" is dropped and "j" acts alone.
	if a, wait := c.feed("j", testBindings()); a != actDown || wait {
		t.Errorf("got %q wait=%v, want %q", a, wait, actDown)
	}
	if len(c.pending) != 0 {
		t.Errorf("pending = %v", c.pending)
	}
}

func TestChordTimeoutDiscardsPending(t *testing.T) {
	var c chord
	c.feed("g", testBindings())
	gen := c.gen
	if !c.timeout(gen) {
		t.Error("timeout for the current generation should reset")
	}
	if a, _ := c.feed("g", testBindings()); a != "" {
		t.Errorf("after timeout a single g should wait again, got %q", a)
	}
}

func TestChordStaleTimeoutIgnored(t *testing.T) {
	var c chord
	c.feed("g", testBindings())
	stale := c.gen
	c.feed("g", testBindings()) // completes gg
	c.feed("d", testBindings()) // new pending sequence
	if c.timeout(stale) {
		t.Error("a stale timeout must not reset a newer pending sequence")
	}
	if a, _ := c.feed("n", testBindings()); a != actDeleteNote {
		t.Errorf("got %q", a)
	}
}

func TestChordUnboundKey(t *testing.T) {
	var c chord
	if a, wait := c.feed("z", testBindings()); a != "" || wait {
		t.Errorf("got %q wait=%v", a, wait)
	}
}

// No context may contain a binding that is a strict prefix of another, or the
// shorter one could never fire.
func TestKeymapHasNoPrefixConflicts(t *testing.T) {
	km := defaultKeymap()
	for ctx, bs := range map[string][]binding{"list": km.list, "issue": km.issue} {
		var seqs []string
		for _, b := range bs {
			seqs = append(seqs, b.keys...)
		}
		for _, a := range seqs {
			for _, b := range seqs {
				if a != b && len(b) > len(a) && b[:len(a)+1] == a+" " {
					t.Errorf("%s: %q is a prefix of %q", ctx, a, b)
				}
				if a == b {
					continue
				}
			}
		}
		seen := map[string]bool{}
		for _, s := range seqs {
			if seen[s] {
				t.Errorf("%s: %q bound twice", ctx, s)
			}
			seen[s] = true
		}
	}
}
