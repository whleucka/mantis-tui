package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

func TestListsRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lists")
	l := NewLists(dir)
	host := Host{Name: "alpha", URL: "https://alpha.example.test"}
	now := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	issues := []mantis.Issue{{ID: 2, Summary: "two", UpdatedAt: now}, {ID: 1, Summary: "one"}}
	if err := l.Save(host, "assigned", issues, now); err != nil {
		t.Fatal(err)
	}
	got, ok := l.Load(host, "assigned")
	if !ok || len(got.Issues) != 2 || got.Issues[0].Summary != "two" || !got.SavedAt.Equal(now) {
		t.Fatalf("Load = %+v, %v", got, ok)
	}
	if _, ok := l.Load(host, "all"); ok {
		t.Error("another filter has no list")
	}
	if _, ok := l.Load(Host{Name: "alpha", URL: "https://moved.example.test"}, "assigned"); ok {
		t.Error("a list saved for another URL must be ignored")
	}

	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, %v", info.Mode().Perm(), err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("want one file and no temp files, got %v", entries)
	}
	info, _ = entries[0].Info()
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v", info.Mode().Perm())
	}
}

func TestListsIgnoreBadFiles(t *testing.T) {
	dir := t.TempDir()
	l := NewLists(dir)
	host := Host{Name: "alpha", URL: "https://alpha.example.test"}
	for _, body := range []string{"not json", `{"version":99,"url":"https://alpha.example.test","filter":"all","issues":[]}`} {
		if err := os.WriteFile(filepath.Join(dir, "alpha.all.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := l.Load(host, "all"); ok {
			t.Errorf("%q should read as no list", body)
		}
	}
}

func TestListsHostNameStaysInTheDir(t *testing.T) {
	dir := t.TempDir()
	l := NewLists(dir)
	host := Host{Name: "../../evil", URL: "https://x.example.test"}
	if err := l.Save(host, "all", nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "______evil.all.json")); err != nil {
		t.Errorf("want the file inside dir: %v", err)
	}
}

func TestNilListsKeepNothing(t *testing.T) {
	var l *Lists
	if err := l.Save(Host{Name: "a"}, "all", []mantis.Issue{{ID: 1}}, time.Time{}); err != nil {
		t.Error(err)
	}
	if _, ok := l.Load(Host{Name: "a"}, "all"); ok {
		t.Error("nil Lists has no lists")
	}
}
