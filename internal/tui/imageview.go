package tui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/whleucka/mantis-tui/internal/graphics"
)

// ErrNoGraphics means the terminal can't show images, so the caller opens
// the file another way.
var ErrNoGraphics = errors.New("the terminal does not support kitty graphics")

// ViewerImage is one image the viewer can step to. Fetch returns its local
// path, downloading it first when it isn't cached.
type ViewerImage struct {
	Title string
	Fetch func() (string, error)
}

// ImageShower shows images[start] on a cols×rows terminal, lets the user
// step through the others, and reports through done once the terminal is
// the TUI's again.
type ImageShower func(images []ViewerImage, start, cols, rows int, done func(error) tea.Msg) tea.Cmd

// KittyImages shows images with `kitten icat`, given the kitten binary's
// path. The TUI is suspended while the viewer is up.
func KittyImages(kitten string) ImageShower {
	return func(images []ViewerImage, start, cols, rows int, done func(error) tea.Msg) tea.Cmd {
		v := &icatView{kitten: kitten, images: images, start: start, cols: cols, rows: rows}
		return tea.Exec(v, func(err error) tea.Msg { return done(err) })
	}
}

// viewerImageID is the full-size viewer's image, apart from the thumbnails'.
const viewerImageID = 4000

// icatView is a tea.ExecCommand: it checks for graphics support, then shows
// one image at a time below a title line on the alternate screen, stepping
// with n/p or the arrows, until q, enter or esc is pressed.
type icatView struct {
	kitten     string
	images     []ViewerImage
	start      int
	cols, rows int

	stdin          io.Reader
	stdout, stderr io.Writer
}

func (v *icatView) SetStdin(r io.Reader)  { v.stdin = r }
func (v *icatView) SetStdout(w io.Writer) { v.stdout = w }
func (v *icatView) SetStderr(w io.Writer) { v.stderr = w }

func (v *icatView) icat(args ...string) *exec.Cmd {
	c := exec.Command(v.kitten, append([]string{"icat"}, args...)...) //nolint:gosec // the kitten found on PATH
	c.Stdout, c.Stderr = v.stdout, v.stderr
	// Only a terminal: exec would copy any other reader in, eating the keys
	// meant for the viewer.
	if f, ok := v.stdin.(*os.File); ok {
		c.Stdin = f
	}
	return c
}

func (v *icatView) Run() error {
	if len(v.images) == 0 {
		return nil
	}
	detect := v.icat("--detect-support", "--detection-timeout", "2")
	detect.Stderr = io.Discard // it reports the transfer mode there
	if err := detect.Run(); err != nil {
		return ErrNoGraphics
	}

	fmt.Fprint(v.stdout, "\x1b[?1049h") // alternate screen
	// Delete just this image: --clear would take the thumbnails with it.
	defer fmt.Fprint(v.stdout, graphics.Delete(viewerImageID)+"\x1b[?1049l")
	i := min(max(v.start, 0), len(v.images)-1)
	for {
		v.show(i)
		key, err := v.readKey()
		if err != nil {
			return err
		}
		switch key {
		case viewerClose:
			return nil
		case viewerNext:
			i = (i + 1) % len(v.images)
		case viewerPrev:
			i = (i + len(v.images) - 1) % len(v.images)
		}
	}
}

// show draws image i under its title. An image that can't be fetched or
// drawn leaves the reason under the title, so the others can still be
// reached.
func (v *icatView) show(i int) {
	cols, rows := max(v.cols, 10), max(v.rows, 3)
	img := v.images[i]
	hint := " · q to go back"
	if len(v.images) > 1 {
		hint = fmt.Sprintf(" · %d/%d · n/p or ←/→ for more · q to go back", i+1, len(v.images))
	}
	title := ansi.Truncate(clean(img.Title)+hint, cols-2, "…")
	// Cleared, with the title on the first line.
	fmt.Fprintf(v.stdout, "%s\x1b[2J\x1b[H\x1b[7m %s \x1b[0m", graphics.Delete(viewerImageID), title)
	path, err := img.Fetch()
	if err == nil {
		err = v.icat("--image-id", strconv.Itoa(viewerImageID), "--place", fmt.Sprintf("%dx%d@0x1", cols, rows-1), path).Run()
	}
	if err != nil {
		fmt.Fprintf(v.stdout, "\x1b[3;2H%s", ansi.Truncate(clean(err.Error()), cols-2, "…"))
	}
}

// readKey waits for a key the viewer knows, in raw mode when stdin is a
// terminal.
func (v *icatView) readKey() (viewerKey, error) {
	if f, ok := v.stdin.(interface{ Fd() uintptr }); ok && term.IsTerminal(f.Fd()) {
		state, err := term.MakeRaw(f.Fd())
		if err != nil {
			return 0, err
		}
		defer term.Restore(f.Fd(), state) //nolint:errcheck // best effort; Bubble Tea takes the terminal back next
	}
	buf := make([]byte, 64)
	for {
		n, err := v.stdin.Read(buf)
		if k := keyOfInput(buf[:n]); k != viewerNone {
			return k, nil
		}
		if err != nil {
			return 0, err
		}
	}
}

type viewerKey int

const (
	viewerNone viewerKey = iota
	viewerClose
	viewerNext
	viewerPrev
)

// keyOfInput reads one chunk of terminal input as a viewer key: q, enter,
// esc or ctrl+c close it (plain, or in the kitty keyboard protocol's CSI u
// form), n, l, space or → step forward, and p, h, backspace or ← back.
func keyOfInput(b []byte) viewerKey {
	if len(b) == 0 {
		return viewerNone
	}
	if b[0] == 0x1b {
		if len(b) == 1 {
			return viewerClose // a lone esc; longer reads are sequences
		}
		switch {
		case bytes.Equal(b, []byte("\x1b[C")), bytes.Equal(b, []byte("\x1bOC")):
			return viewerNext
		case bytes.Equal(b, []byte("\x1b[D")), bytes.Equal(b, []byte("\x1bOD")):
			return viewerPrev
		}
		for _, code := range []string{"27", "13", "113", "81"} {
			if bytes.HasPrefix(b, []byte("\x1b["+code+"u")) || bytes.HasPrefix(b, []byte("\x1b["+code+";")) {
				return viewerClose
			}
		}
		return viewerNone
	}
	for _, c := range b {
		switch c {
		case 'q', 'Q', '\r', '\n', 0x03:
			return viewerClose
		case 'n', 'l', ' ':
			return viewerNext
		case 'p', 'h', 0x7f:
			return viewerPrev
		}
	}
	return viewerNone
}
