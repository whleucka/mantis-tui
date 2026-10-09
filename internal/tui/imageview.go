package tui

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/whleucka/mantis-tui/internal/graphics"
)

// ErrNoGraphics means the terminal can't show images, so the caller opens
// the file another way.
var ErrNoGraphics = errors.New("the terminal does not support kitty graphics")

// ImageShower shows the image at path, titled, on a cols×rows terminal, and
// reports through done once the terminal is the TUI's again.
type ImageShower func(path, title string, cols, rows int, done func(error) tea.Msg) tea.Cmd

// KittyImages shows images with `kitten icat`, given the kitten binary's
// path. The TUI is suspended while the image is up.
func KittyImages(kitten string) ImageShower {
	return func(path, title string, cols, rows int, done func(error) tea.Msg) tea.Cmd {
		v := &icatView{kitten: kitten, path: path, title: title, cols: cols, rows: rows}
		return tea.Exec(v, func(err error) tea.Msg { return done(err) })
	}
}

// viewerImageID is the full-size viewer's image, apart from the thumbnails'.
const viewerImageID = 4000

// icatView is a tea.ExecCommand: it checks for graphics support, then shows
// one image below a title line on the alternate screen until enter or
// esc is pressed.
type icatView struct {
	kitten      string
	path, title string
	cols, rows  int

	stdin          io.Reader
	stdout, stderr io.Writer
}

func (v *icatView) SetStdin(r io.Reader)  { v.stdin = r }
func (v *icatView) SetStdout(w io.Writer) { v.stdout = w }
func (v *icatView) SetStderr(w io.Writer) { v.stderr = w }

func (v *icatView) icat(args ...string) *exec.Cmd {
	c := exec.Command(v.kitten, append([]string{"icat"}, args...)...) //nolint:gosec // the kitten found on PATH
	c.Stdin, c.Stdout, c.Stderr = v.stdin, v.stdout, v.stderr
	return c
}

func (v *icatView) Run() error {
	detect := v.icat("--detect-support", "--detection-timeout", "2")
	detect.Stderr = io.Discard // it reports the transfer mode there
	if err := detect.Run(); err != nil {
		return ErrNoGraphics
	}

	cols, rows := max(v.cols, 10), max(v.rows, 3)
	title := ansi.Truncate(clean(v.title)+" · enter or esc to go back", cols-2, "…")
	// Alternate screen, cleared, with the title on the first line.
	fmt.Fprintf(v.stdout, "\x1b[?1049h\x1b[2J\x1b[H\x1b[7m %s \x1b[0m", title)
	err := v.icat("--hold", "--image-id", strconv.Itoa(viewerImageID), "--place", fmt.Sprintf("%dx%d@0x1", cols, rows-1), v.path).Run()
	// Delete just this image: --clear would take the thumbnails with it.
	fmt.Fprint(v.stdout, graphics.Delete(viewerImageID)+"\x1b[?1049l")
	return err
}
