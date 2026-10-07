package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// MaxSeenPerHost bounds how many issues the seen file remembers per host.
const MaxSeenPerHost = 5000

// Seen remembers, per host, the updated_at of each issue when the user last
// saw it, so the TUI can mark issues that changed since. It is safe for
// concurrent use.
type Seen struct {
	mu     sync.Mutex
	saveMu sync.Mutex // serializes saves, so the last write holds the newest state
	path   string     // "" keeps it in memory only
	hosts  map[string]*seenHost
}

type seenHost struct {
	// Baseline is when the host was first used. Issues never seen count as
	// seen at this time, so a first run does not mark everything unread.
	Baseline int64 `json:"baseline"`
	// Issues maps an issue id to its updated_at (unix seconds) when last
	// seen. 0 means the user marked it unread.
	Issues map[int]int64 `json:"issues"`
}

// DefaultSeenPath is seen.json next to the state file.
func DefaultSeenPath(getenv func(string) string) string {
	return filepath.Join(filepath.Dir(DefaultStatePath(getenv)), "seen.json")
}

// NewSeen returns an empty store that saves to path ("" never saves).
func NewSeen(path string) *Seen {
	return &Seen{path: path, hosts: map[string]*seenHost{}}
}

// LoadSeen reads the seen file. A missing or corrupt file yields an empty
// store: losing read state is harmless.
func LoadSeen(path string) *Seen {
	s := NewSeen(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var file struct {
		Hosts map[string]*seenHost `json:"hosts"`
	}
	if json.Unmarshal(data, &file) != nil {
		return s
	}
	for name, h := range file.Hosts {
		if h == nil {
			continue
		}
		if h.Issues == nil {
			h.Issues = map[int]int64{}
		}
		s.hosts[name] = h
	}
	return s
}

// Begin sets the host's baseline to now if it has none. It reports whether
// anything changed.
func (s *Seen) Begin(host string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.hosts[host]; ok {
		return false
	}
	s.hosts[host] = &seenHost{Baseline: now.Unix(), Issues: map[int]int64{}}
	return true
}

// Unread reports whether an issue last updated at updated changed since the
// user saw it. Hosts without a baseline have nothing unread.
func (s *Seen) Unread(host string, id int, updated time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.hosts[host]
	if h == nil {
		return false
	}
	last, ok := h.Issues[id]
	if !ok {
		last = h.Baseline
	}
	return updated.Unix() > last
}

// Mark records that the user saw the issue as of updated. It never moves
// the seen time backwards, and reports whether anything changed.
func (s *Seen) Mark(host string, id int, updated time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.hosts[host]
	if h == nil {
		return false
	}
	u := updated.Unix()
	last, ok := h.Issues[id]
	if !ok && u <= h.Baseline {
		return false // already read by the baseline; no entry needed
	}
	if ok && u <= last {
		return false
	}
	h.Issues[id] = u
	return true
}

// MarkUnread flags the issue as unread until it is seen again.
func (s *Seen) MarkUnread(host string, id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.hosts[host]
	if h == nil {
		return false
	}
	if last, ok := h.Issues[id]; ok && last == 0 {
		return false
	}
	h.Issues[id] = 0
	return true
}

// Save writes the store atomically with owner-only permissions, keeping
// the most recently updated MaxSeenPerHost issues per host.
func (s *Seen) Save() error {
	if s.path == "" {
		return nil
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	for _, h := range s.hosts {
		prune(h)
	}
	data, err := json.Marshal(struct {
		Hosts map[string]*seenHost `json:"hosts"`
	}{s.hosts})
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("encode seen state: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".seen-*.json") // CreateTemp uses mode 0600
	if err != nil {
		return fmt.Errorf("write seen state: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write seen state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write seen state: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write seen state: %w", err)
	}
	return nil
}

// prune drops the entries with the oldest seen times beyond the cap and
// raises the baseline to the newest dropped time, so a dropped issue still
// counts as seen instead of turning unread. Explicit unread marks are kept.
func prune(h *seenHost) {
	extra := len(h.Issues) - MaxSeenPerHost
	if extra <= 0 {
		return
	}
	ids := make([]int, 0, len(h.Issues))
	for id, at := range h.Issues {
		if at != 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := h.Issues[ids[i]], h.Issues[ids[j]]
		if a != b {
			return a < b
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids[:min(extra, len(ids))] {
		h.Baseline = max(h.Baseline, h.Issues[id])
		delete(h.Issues, id)
	}
}
