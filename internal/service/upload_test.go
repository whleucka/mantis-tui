package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
)

var wh = meta.UploadLimits{MaxFileSize: 5 << 20, Disallowed: []string{"svg"}}

func TestReadUpload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("boom\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := ReadUpload(path, wh)
	if err != nil || f.Name != "app.log" || string(f.Content) != "boom\n" {
		t.Fatalf("ReadUpload = %+v, %v", f, err)
	}
	if _, err := ReadUpload(dir, wh); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("a directory: %v", err)
	}
	if _, err := ReadUpload(filepath.Join(dir, "missing"), wh); err == nil {
		t.Error("a missing file should fail")
	}
	if _, err := ReadUpload(path, meta.UploadLimits{MaxFileSize: 3}); err == nil || !strings.Contains(err.Error(), "app.log is 5 B; the server takes files up to 3 B") {
		t.Errorf("too big: %v", err)
	}
}

func TestCheckUploads(t *testing.T) {
	file := func(name string, n int) mantis.FileUpload {
		return mantis.FileUpload{Name: name, Content: bytes.Repeat([]byte("x"), n)}
	}
	for _, tc := range []struct {
		name  string
		files []mantis.FileUpload
		l     meta.UploadLimits
		want  string // "" for no error
	}{
		{"fine", []mantis.FileUpload{file("a.png", 10), file("b.log", 10)}, wh, ""},
		{"too big", []mantis.FileUpload{file("big.png", 6<<20)}, wh, "big.png is 6.0 MB; the server takes files up to 5.0 MB"},
		{"disallowed", []mantis.FileUpload{file("logo.SVG", 10)}, wh, "logo.SVG: the server doesn't accept .svg files"},
		{"not allowed", []mantis.FileUpload{file("a.zip", 10)}, meta.UploadLimits{Allowed: []string{"png", "log"}}, "a.zip: the server doesn't accept .zip files"},
		{"allowed", []mantis.FileUpload{file("a.png", 10)}, meta.UploadLimits{Allowed: []string{"png"}}, ""},
		{"request too big", []mantis.FileUpload{file("a.png", 4<<20), file("b.png", 4<<20)}, wh, "one request must stay under 8.0 MB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckUploads(tc.files, tc.l)
			if (tc.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.want)) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// fakeClipboard answers wl-paste or xclip with types and contents.
type fakeClipboard struct {
	types string
	data  map[string][]byte
	fail  bool
	ran   []string
}

func (c *fakeClipboard) run(_ context.Context, name string, args ...string) ([]byte, error) {
	c.ran = append(c.ran, name+" "+strings.Join(args, " "))
	if c.fail {
		return nil, errors.New("exit status 1")
	}
	for _, a := range args {
		if b, ok := c.data[a]; ok {
			return b, nil
		}
	}
	return []byte(c.types), nil
}

func env(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }

func has(names ...string) func(string) (string, error) {
	return func(n string) (string, error) {
		for _, x := range names {
			if x == n {
				return "/usr/bin/" + n, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestClipboardImage(t *testing.T) {
	now := time.Date(2026, 10, 9, 7, 45, 12, 0, time.Local)
	wayland := env(map[string]string{"WAYLAND_DISPLAY": "wayland-1", "DISPLAY": ":0"})

	c := &fakeClipboard{types: "image/jpeg\nimage/png\ntext/plain\n", data: map[string][]byte{"image/png": []byte("\x89PNG"), "image/jpeg": []byte("JPEG")}}
	f := ClipboardImage(context.Background(), c.run, has("wl-paste", "xclip"), wayland, now)
	if f == nil || f.Name != "screenshot-20261009-074512.png" || string(f.Content) != "\x89PNG" {
		t.Fatalf("got %+v, want the PNG", f)
	}
	if c.ran[0] != "wl-paste --list-types" || c.ran[1] != "wl-paste --no-newline --type image/png" {
		t.Errorf("ran %q", c.ran)
	}

	c = &fakeClipboard{types: "image/jpeg", data: map[string][]byte{"image/jpeg": []byte("JPEG")}}
	if f := ClipboardImage(context.Background(), c.run, has("xclip"), wayland, now); f == nil || f.Name != "screenshot-20261009-074512.jpg" {
		t.Errorf("xclip JPEG = %+v", f)
	}
	if c.ran[0] != "xclip -selection clipboard -t TARGETS -o" {
		t.Errorf("without wl-paste use xclip: %q", c.ran)
	}

	for name, tc := range map[string]struct {
		c      *fakeClipboard
		look   func(string) (string, error)
		getenv func(string) string
	}{
		"text only":   {&fakeClipboard{types: "text/plain"}, has("wl-paste"), wayland},
		"tool fails":  {&fakeClipboard{fail: true}, has("wl-paste"), wayland},
		"no tool":     {&fakeClipboard{types: "image/png"}, has(), wayland},
		"no display":  {&fakeClipboard{types: "image/png"}, has("wl-paste", "xclip"), env(nil)},
		"empty image": {&fakeClipboard{types: "image/png", data: map[string][]byte{"image/png": {}}}, has("wl-paste"), wayland},
	} {
		if f := ClipboardImage(context.Background(), tc.c.run, tc.look, tc.getenv, now); f != nil {
			t.Errorf("%s: got %+v, want no image", name, f)
		}
	}
}
