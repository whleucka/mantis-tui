package tui

import (
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/config"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

// harness drives a Model synchronously: every Cmd is executed and its Msg fed
// back, except timers (anything that does not return within a few ms).
type harness struct {
	t     *testing.T
	m     *Model
	fakes map[string]*mantistest.Fake
	quit  bool

	mu    sync.Mutex
	saved []string // SaveLastHost calls
}

func testHosts() []config.Host {
	return []config.Host{
		{Name: "alpha", URL: "https://alpha.example.test", Token: "token-alpha-secret"},
		{Name: "beta", URL: "https://beta.example.test", Token: "token-beta-secret"},
	}
}

func newHarness(t *testing.T, initial *config.Host, setup func(host string, f *mantistest.Fake)) *harness {
	t.Helper()
	return newHarnessWith(t, initial, setup, nil)
}

func winSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

func newHarnessWith(t *testing.T, initial *config.Host, setup func(host string, f *mantistest.Fake), tweak func(*config.Config)) *harness {
	t.Helper()
	h := &harness{t: t, fakes: map[string]*mantistest.Fake{}}
	for _, host := range testHosts() {
		f := &mantistest.Fake{
			CurrentUser:  mantis.User{ID: 2, Name: "me", RealName: "Me"},
			ConfigValues: mantistest.StandardConfig(),
			Issues:       map[int]mantis.Issue{},
		}
		if setup != nil {
			setup(host.Name, f)
		}
		h.fakes[host.Name] = f
	}
	cfg := config.Defaults()
	cfg.List.AutoRefresh.Duration = 0
	cfg.Issue.AutoRefresh.Duration = 0
	cfg.List.Sort = "id" // the fake serves issues by id; most tests rely on that order
	if tweak != nil {
		tweak(cfg)
	}
	m := New(Options{
		Config:  cfg,
		Hosts:   testHosts(),
		Initial: initial,
		NewSession: func(host config.Host) *Session {
			return NewSession(host, h.fakes[host.Name])
		},
		SaveLastHost: func(name string) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.saved = append(h.saved, name)
			return nil
		},
	})
	h.m = m
	h.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	h.exec(m.Init())
	return h
}

// send delivers msgs and processes all resulting commands.
func (h *harness) send(msgs ...tea.Msg) {
	h.t.Helper()
	queue := append([]tea.Msg{}, msgs...)
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 500 {
			h.t.Fatal("message loop did not settle")
		}
		msg := queue[0]
		queue = queue[1:]
		switch msg := msg.(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				queue = append(queue, run(c)...)
			}
			continue
		case tea.QuitMsg:
			h.quit = true
			continue
		}
		next, cmd := h.m.Update(msg)
		h.m = next.(*Model)
		queue = append(queue, run(cmd)...)
	}
}

func (h *harness) exec(cmd tea.Cmd) { h.send(run(cmd)...) }

// keys sends key presses one at a time.
func (h *harness) keys(ks ...string) {
	h.t.Helper()
	for _, k := range ks {
		h.send(key(k))
	}
}

func (h *harness) view() string { return h.m.View().Content }

func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(30 * time.Millisecond):
		return nil // a timer; tests trigger timeouts explicitly
	}
}
