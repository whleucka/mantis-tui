// Package graphics draws images in the terminal with the kitty graphics
// protocol's Unicode placeholders: the image is sent once, and the terminal
// draws it wherever placeholder characters carrying its id appear, so a TUI
// can treat it as ordinary text.
//
// See https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders
package graphics

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"  // decoders for Load
	_ "image/jpeg" // decoders for Load
	_ "image/png"  // decoders for Load
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// QueryID is the image id of the support query; the terminal's reply
// carries it.
const QueryID = 31

// MaxPixels bounds the images Load decodes.
const MaxPixels = 40_000_000

// ErrTooLarge is returned for an image over MaxPixels.
var ErrTooLarge = errors.New("image too large")

// Cell is a terminal cell's size in pixels.
type Cell struct{ W, H int }

// DefaultCell is assumed until the terminal reports its cell size.
var DefaultCell = Cell{W: 10, H: 20}

// Query asks the terminal whether it supports kitty graphics (a 1×1 RGB
// image that is checked, not stored) and for its cell and window sizes in
// pixels.
func Query() string {
	return ansi.KittyGraphics([]byte("AAAA"), fmt.Sprintf("i=%d", QueryID), "s=1", "v=1", "a=q", "t=d", "f=24") +
		"\x1b[16t\x1b[14t"
}

// Load decodes a PNG, JPEG or GIF file, refusing one over MaxPixels before
// decoding its pixels.
func Load(path string) (image.Image, error) {
	b, err := os.ReadFile(path) //nolint:gosec // a file this program downloaded into its cache
	if err != nil {
		return nil, err //nolint:wrapcheck // already names the path
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return nil, fmt.Errorf("%dx%d: %w", cfg.Width, cfg.Height, ErrTooLarge)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

// Fit returns the w×h pixels a w0×h0 image is scaled to so that it fits
// in maxCols×maxRows cells, never scaling it up, and the cells it covers.
func Fit(w0, h0 int, cell Cell, maxCols, maxRows int) (w, h, cols, rows int) {
	if w0 <= 0 || h0 <= 0 || maxCols <= 0 || maxRows <= 0 {
		return 0, 0, 0, 0
	}
	boxW, boxH := maxCols*cell.W, maxRows*cell.H
	w, h = w0, h0
	if w > boxW {
		w, h = boxW, max(1, h*boxW/w)
	}
	if h > boxH {
		w, h = max(1, w*boxH/h), boxH
	}
	return w, h, ceilDiv(w, cell.W), ceilDiv(h, cell.H)
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }

// Thumbnail scales img down to w×h with a box filter and pads it with
// transparency to cols×rows whole cells, image in the top-left corner, so
// the terminal fitting or stretching it into the cells gives the same
// result.
func Thumbnail(img image.Image, w, h, cols, rows int, cell Cell) *image.RGBA {
	b := img.Bounds()
	src := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	out := image.NewRGBA(image.Rect(0, 0, cols*cell.W, rows*cell.H))
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, bl, a, n uint32
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride:]
				for sx := x0; sx < x1; sx++ {
					p := row[sx*4 : sx*4+4]
					r, g, bl, a = r+uint32(p[0]), g+uint32(p[1]), bl+uint32(p[2]), a+uint32(p[3])
					n++
				}
			}
			o := out.Pix[y*out.Stride+x*4:]
			o[0], o[1], o[2], o[3] = uint8(r/n), uint8(g/n), uint8(bl/n), uint8(a/n) //nolint:gosec // averages of bytes
		}
	}
	return out
}

// Transmit sends img as a PNG with image id and creates its virtual
// placement of cols×rows cells, quietly, in 4 KB chunks.
func Transmit(id int, img image.Image, cols, rows int) (string, error) {
	var b strings.Builder
	err := kitty.EncodeGraphics(&b, img, &kitty.Options{
		Action: kitty.TransmitAndPut, Transmission: kitty.Direct, Format: kitty.PNG,
		ID: id, PlacementID: 1, VirtualPlacement: true, Columns: cols, Rows: rows,
		Quiet: 2, Chunk: true,
	})
	if err != nil {
		return "", fmt.Errorf("encode image: %w", err)
	}
	return b.String(), nil
}

// Place puts image id's virtual placement again, as cols×rows cells.
func Place(id, cols, rows int) string {
	return ansi.KittyGraphics(nil, fmt.Sprintf("i=%d", id), "p=1", "U=1", fmt.Sprintf("c=%d", cols), fmt.Sprintf("r=%d", rows), "q=2", "a=p")
}

// Delete frees image id and its placements in the terminal.
func Delete(id int) string {
	return ansi.KittyGraphics(nil, fmt.Sprintf("i=%d", id), "d=I", "q=2", "a=d")
}

// Placeholder is the text that shows image id as rows lines of cols cells.
// The id rides in an indexed foreground colour, so it must be 16–255 to
// survive renderers that rewrite low indexes as basic colours.
func Placeholder(id, cols, rows int) []string {
	lines := make([]string, rows)
	var b strings.Builder
	for r := range rows {
		b.Reset()
		fmt.Fprintf(&b, "\x1b[38;5;%dm", id)
		for c := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
		}
		b.WriteString("\x1b[39m")
		lines[r] = b.String()
	}
	return lines
}
