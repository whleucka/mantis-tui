package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const chordTimeout = time.Second

// chord resolves multi-key sequences such as "g g" or "b s".
type chord struct {
	pending []string
	gen     int // bumped whenever a new sequence starts, to ignore stale timeouts
}

type chordTimeoutMsg struct{ gen int }

// feed adds a key. It returns the matched action, or waiting=true when the
// keys so far are the prefix of a longer sequence. A key that cannot extend
// the pending sequence drops it and is tried on its own.
func (c *chord) feed(key string, bindings []binding) (a action, waiting bool) {
	seq := strings.Join(append(append([]string{}, c.pending...), key), " ")
	match, prefix := lookup(seq, bindings)
	if prefix {
		if len(c.pending) == 0 {
			c.gen++
		}
		c.pending = append(c.pending, key)
		return "", true
	}
	hadPending := len(c.pending) > 0
	c.pending = nil
	if match != "" {
		return match, false
	}
	if hadPending {
		return c.feed(key, bindings)
	}
	return "", false
}

// timeout drops a pending sequence if gen is still current.
func (c *chord) timeout(gen int) bool {
	if gen != c.gen || len(c.pending) == 0 {
		return false
	}
	c.pending = nil
	return true
}

// waitCmd schedules the timeout for the current pending sequence.
func (c *chord) waitCmd() tea.Cmd {
	gen := c.gen
	return tea.Tick(chordTimeout, func(time.Time) tea.Msg { return chordTimeoutMsg{gen: gen} })
}

func lookup(seq string, bindings []binding) (match action, prefix bool) {
	for _, b := range bindings {
		for _, k := range b.keys {
			switch {
			case k == seq:
				match = b.action
			case strings.HasPrefix(k, seq+" "):
				prefix = true
			}
		}
	}
	return match, prefix
}
