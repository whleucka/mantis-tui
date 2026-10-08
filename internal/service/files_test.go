package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func filesIssue() mantis.Issue {
	return mantis.Issue{
		ID:          7,
		Attachments: []mantis.Attachment{{ID: 1, Filename: "guide.pdf", Size: 5}},
		Notes: []mantis.Note{
			{ID: 40, Reporter: mantis.User{Name: "jane"}},
			{ID: 41, Reporter: mantis.User{Name: "bob"}, Attachments: []mantis.Attachment{
				{ID: 2, Filename: "image.png", Size: 4, ContentType: "image/png; charset=binary"},
				{ID: 3, Filename: "image.png", Size: 4, ContentType: "image/png"},
			}},
		},
	}
}

func TestFilesListsIssueThenNoteFiles(t *testing.T) {
	is := filesIssue()
	got := Files(&is)
	var where []string
	for _, f := range got {
		where = append(where, f.Filename+"@"+f.Where())
	}
	want := "guide.pdf@issue image.png@note 41 by bob image.png@note 41 by bob"
	if strings.Join(where, " ") != want {
		t.Errorf("files = %v", where)
	}
}

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"image.png":                       "image.png",
		"../../etc/passwd":                "_.._etc_passwd",
		`..\win.ini`:                      "_win.ini",
		".bashrc":                         "bashrc",
		"a\x1b[31mb\n.txt":                "a_[31mb_.txt",
		"":                                "file",
		"...":                             "file",
		strings.Repeat("x", 150) + ".png": strings.Repeat("x", 96) + ".png",
	} {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDownloadWritesPrivateFileAndReuses(t *testing.T) {
	is := filesIssue()
	f := &mantistest.Fake{Issues: map[int]mantis.Issue{7: is}, FileContent: map[int][]byte{2: []byte("\x89PNG")}}
	dir := filepath.Join(t.TempDir(), "cache", "alpha", "7")
	a := is.Notes[1].Attachments[0]

	path, err := Download(context.Background(), f, 7, a, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "2-image.png" {
		t.Errorf("path = %s", path)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, err %v", st.Mode(), err)
	}
	if dst, _ := os.Stat(dir); dst.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v", dst.Mode())
	}
	if b, _ := os.ReadFile(path); string(b) != "\x89PNG" {
		t.Errorf("content = %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".download-*")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}

	f.Errs = map[string]error{"GetFile": os.ErrDeadlineExceeded}
	if again, err := Download(context.Background(), f, 7, a, dir, true); err != nil || again != path {
		t.Errorf("a cached file of the right size must not be fetched again: %s, %v", again, err)
	}
	if _, err := Download(context.Background(), f, 7, a, dir, false); err == nil {
		t.Error("without reuse it must fetch again")
	}
}

func TestIsImageAndSafeToOpen(t *testing.T) {
	for _, tc := range []struct {
		a     mantis.Attachment
		image bool
	}{
		{mantis.Attachment{Filename: "image.png", ContentType: "image/png; charset=binary"}, true},
		{mantis.Attachment{Filename: "file-6.txt", ContentType: "image/png; charset=binary"}, true},
		{mantis.Attachment{Filename: "Shot.JPG"}, true},
		{mantis.Attachment{Filename: "logo.svg", ContentType: "image/svg+xml"}, false},
		{mantis.Attachment{Filename: "guide.pdf", ContentType: "application/pdf"}, false},
	} {
		if got := IsImage(tc.a); got != tc.image {
			t.Errorf("IsImage(%s) = %v", tc.a.Filename, got)
		}
	}
	for path, want := range map[string]bool{
		"/c/1-guide.pdf": true, "/c/2-api.postman_collection.json": true, "/c/3-Shot.PNG": true,
		"/c/4-run.sh": false, "/c/5-app.desktop": false, "/c/6-page.html": false, "/c/7-noext": false,
	} {
		if got := SafeToOpen(path); got != want {
			t.Errorf("SafeToOpen(%s) = %v", path, got)
		}
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{512: "512 B", 4718: "4.6 KB", 651323: "636.1 KB", 3 << 20: "3.0 MB"} {
		if got := HumanSize(n); got != want {
			t.Errorf("HumanSize(%d) = %s, want %s", n, got, want)
		}
	}
}
