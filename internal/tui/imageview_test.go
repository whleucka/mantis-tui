package tui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyOfInput(t *testing.T) {
	for in, want := range map[string]viewerKey{
		"q": viewerClose, "Q": viewerClose, "\r": viewerClose, "\n": viewerClose, "\x1b": viewerClose, "\x03": viewerClose,
		"\x1b[27u": viewerClose, "\x1b[13u": viewerClose, "\x1b[113u": viewerClose, "\x1b[113;1u": viewerClose,
		"n": viewerNext, "l": viewerNext, " ": viewerNext, "\x1b[C": viewerNext, "\x1bOC": viewerNext,
		"p": viewerPrev, "h": viewerPrev, "\x7f": viewerPrev, "\x1b[D": viewerPrev, "\x1bOD": viewerPrev,
		"j": viewerNone, "\x1b[A": viewerNone, "\x1b[106u": viewerNone, "": viewerNone,
	} {
		if got := keyOfInput([]byte(in)); got != want {
			t.Errorf("keyOfInput(%q) = %v, want %v", in, got, want)
		}
	}
}

// keystrokes hands out one chunk per Read, as a terminal does per key.
type keystrokes []string

func (k *keystrokes) Read(p []byte) (int, error) {
	if len(*k) == 0 {
		return 0, io.EOF
	}
	n := copy(p, (*k)[0])
	*k = (*k)[1:]
	return n, nil
}

// fakeKitten is a kitten that logs the file each icat call draws, one per
// line, to the returned log.
func fakeKitten(t *testing.T) (kitten, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "log")
	kitten = filepath.Join(dir, "kitten")
	script := "#!/bin/sh\nfor a; do last=$a; done\ncase \"$*\" in *--detect-support*) exit 0;; esac\necho \"$last\" >> " + log + "\n"
	if err := os.WriteFile(kitten, []byte(script), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	return kitten, log
}

func TestViewerStepsThroughImages(t *testing.T) {
	kitten, log := fakeKitten(t)
	img := func(path string) ViewerImage {
		return ViewerImage{Title: filepath.Base(path), Fetch: func() (string, error) { return path, nil }}
	}
	broken := ViewerImage{Title: "gone.png", Fetch: func() (string, error) { return "", errors.New("404 not found") }}
	keys := keystrokes{"n", "j", "n", "\x1b[C", "p", "\x1b[D", "q", "n"}
	var out bytes.Buffer
	v := &icatView{kitten: kitten, images: []ViewerImage{img("/a.png"), img("/b.png"), broken}, start: 1, cols: 80, rows: 24}
	v.SetStdin(&keys)
	v.SetStdout(&out)
	v.SetStderr(io.Discard)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log) //nolint:gosec // the test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	// Start on b; n → gone (not drawn); j ignored; n wraps to a; → b; p a;
	// ← wraps to gone; q closes before the last n.
	if got, want := strings.Fields(string(b)), []string{"/b.png", "/a.png", "/b.png", "/a.png"}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("drawn %v, want %v", got, want)
	}
	if len(keys) != 1 {
		t.Errorf("q should close the viewer, %d keys left", len(keys))
	}
	for _, want := range []string{"b.png · 2/3", "gone.png · 3/3", "404 not found", "\x1b[?1049l"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

func TestViewerWithoutGraphics(t *testing.T) {
	v := &icatView{kitten: "false", images: []ViewerImage{{Title: "a.png"}}}
	v.SetStdin(&keystrokes{})
	v.SetStdout(io.Discard)
	v.SetStderr(io.Discard)
	if err := v.Run(); !errors.Is(err, ErrNoGraphics) {
		t.Errorf("err = %v, want ErrNoGraphics", err)
	}
}
