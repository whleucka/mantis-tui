package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// listsVersion changes when the file layout does; other versions are ignored.
const listsVersion = 1

// DefaultListsDir is where the TUI keeps the last list of each host and
// filter: $XDG_CACHE_HOME/mantis-tui/lists, falling back to ~/.cache.
func DefaultListsDir(getenv func(string) string) string {
	return filepath.Join(filepath.Dir(DefaultFilesDir(getenv)), "lists")
}

// Lists keeps the last complete issue list of each host and filter, so the
// TUI can show it at once on the next start while the real load runs.
// A nil *Lists keeps nothing.
type Lists struct {
	dir string
}

// NewLists stores lists under dir.
func NewLists(dir string) *Lists { return &Lists{dir: dir} }

// SavedList is a list as it was last loaded.
type SavedList struct {
	SavedAt time.Time
	Issues  []mantis.Issue
}

type listFile struct {
	Version int            `json:"version"`
	URL     string         `json:"url"`
	Filter  string         `json:"filter"`
	SavedAt time.Time      `json:"saved_at"`
	Issues  []mantis.Issue `json:"issues"`
}

// Load returns host's saved list for filter. A missing, unreadable or
// foreign file (another version, or the host's URL changed) is no list.
func (l *Lists) Load(host Host, filter string) (SavedList, bool) {
	if l == nil {
		return SavedList{}, false
	}
	data, err := os.ReadFile(l.path(host, filter))
	if err != nil {
		return SavedList{}, false
	}
	var f listFile
	if json.Unmarshal(data, &f) != nil || f.Version != listsVersion || f.URL != host.URL || f.Filter != filter {
		return SavedList{}, false
	}
	return SavedList{SavedAt: f.SavedAt, Issues: f.Issues}, true
}

// Save writes host's list for filter atomically, owner-only.
func (l *Lists) Save(host Host, filter string, issues []mantis.Issue, now time.Time) error {
	if l == nil {
		return nil
	}
	data, err := json.Marshal(listFile{Version: listsVersion, URL: host.URL, Filter: filter, SavedAt: now, Issues: issues})
	if err != nil {
		return fmt.Errorf("encode list: %w", err)
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return fmt.Errorf("create list cache: %w", err)
	}
	tmp, err := os.CreateTemp(l.dir, ".list-*.json") // CreateTemp uses mode 0600
	if err != nil {
		return fmt.Errorf("write list cache: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write list cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write list cache: %w", err)
	}
	if err := os.Rename(tmp.Name(), l.path(host, filter)); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write list cache: %w", err)
	}
	return nil
}

// path is <dir>/<host>.<filter>.json, with anything but letters, digits,
// '-' and '_' in the names replaced, so a host name can't leave dir.
func (l *Lists) path(host Host, filter string) string {
	safe := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				return r
			}
			return '_'
		}, s)
	}
	return filepath.Join(l.dir, safe(host.Name)+"."+safe(filter)+".json")
}
