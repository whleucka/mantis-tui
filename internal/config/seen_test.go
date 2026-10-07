package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestSeenBaselineHidesOldIssues(t *testing.T) {
	s := NewSeen("")
	if s.Unread("h", 1, t0.Add(time.Hour)) {
		t.Error("a host with no baseline has nothing unread")
	}
	if !s.Begin("h", t0) || s.Begin("h", t0.Add(time.Hour)) {
		t.Error("Begin should set the baseline once")
	}
	if s.Unread("h", 1, t0.Add(-time.Hour)) {
		t.Error("an issue untouched since the baseline is read")
	}
	if !s.Unread("h", 1, t0.Add(time.Second)) {
		t.Error("an issue updated after the baseline is unread")
	}
}

func TestSeenMarkAndUnread(t *testing.T) {
	s := NewSeen("")
	s.Begin("h", t0)
	upd := t0.Add(time.Hour)
	if !s.Mark("h", 1, upd) {
		t.Fatal("Mark should record a new time")
	}
	if s.Unread("h", 1, upd) {
		t.Error("marked issue should be read")
	}
	if s.Mark("h", 1, t0.Add(time.Minute)) {
		t.Error("Mark must not move the seen time backwards")
	}
	if !s.Unread("h", 1, upd.Add(time.Second)) {
		t.Error("a later update makes it unread again")
	}
	if s.Mark("h", 2, t0.Add(-time.Hour)) {
		t.Error("marking an issue already read by the baseline should not add an entry")
	}
	if !s.MarkUnread("h", 1) || s.MarkUnread("h", 1) {
		t.Error("MarkUnread should change once")
	}
	if !s.Unread("h", 1, upd) {
		t.Error("explicitly unread issue should be unread")
	}
	if !s.Mark("h", 1, upd) || s.Unread("h", 1, upd) {
		t.Error("seeing it again clears the explicit unread mark")
	}
	if s.Mark("other", 1, upd) || s.MarkUnread("other", 1) {
		t.Error("hosts without a baseline are not tracked")
	}
}

func TestSeenSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "seen.json")
	s := NewSeen(path)
	s.Begin("h", t0)
	s.Mark("h", 7, t0.Add(time.Hour))
	s.MarkUnread("h", 8)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("seen file mode = %o, want 600", perm)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".seen-*")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}

	got := LoadSeen(path)
	if got.Unread("h", 7, t0.Add(time.Hour)) || !got.Unread("h", 8, t0) || !got.Unread("h", 9, t0.Add(time.Second)) {
		t.Error("loaded store should keep marks and baseline")
	}
}

func TestSeenCorruptOrMissingFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if s := LoadSeen(filepath.Join(dir, "missing.json")); s.Unread("h", 1, t0) {
		t.Error("missing file should load empty")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := LoadSeen(bad)
	if !s.Begin("h", t0) {
		t.Error("corrupt file should load empty")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if LoadSeen(bad).Begin("h", t0) {
		t.Error("saving should replace the corrupt file")
	}
}

func TestSeenPruneKeepsNewestAndRaisesBaseline(t *testing.T) {
	s := NewSeen(filepath.Join(t.TempDir(), "seen.json"))
	s.Begin("h", t0)
	for i := 1; i <= MaxSeenPerHost+10; i++ {
		s.Mark("h", i, t0.Add(time.Duration(i)*time.Second))
	}
	s.MarkUnread("h", 1) // explicit unread marks survive pruning
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	h := s.hosts["h"]
	if len(h.Issues) != MaxSeenPerHost {
		t.Fatalf("kept %d entries, want %d", len(h.Issues), MaxSeenPerHost)
	}
	if _, ok := h.Issues[1]; !ok {
		t.Error("explicit unread mark should be kept")
	}
	if _, ok := h.Issues[2]; ok {
		t.Error("oldest entry should be dropped")
	}
	if s.Unread("h", 2, t0.Add(2*time.Second)) {
		t.Error("a dropped issue must still count as seen")
	}
	if !s.Unread("h", MaxSeenPerHost+10, t0.Add(time.Duration(MaxSeenPerHost+11)*time.Second)) {
		t.Error("kept entries still detect newer updates")
	}
}

func TestDefaultSeenPath(t *testing.T) {
	env := map[string]string{"XDG_STATE_HOME": "/x/state"}
	if got := DefaultSeenPath(func(k string) string { return env[k] }); got != "/x/state/mantis-tui/seen.json" {
		t.Errorf("DefaultSeenPath = %q", got)
	}
}
