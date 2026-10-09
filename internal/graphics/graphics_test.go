package graphics

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi/kitty"
)

func TestFit(t *testing.T) {
	cell := Cell{W: 10, H: 20}
	for _, tc := range []struct {
		name               string
		w0, h0, maxC, maxR int
		w, h, cols, rows   int
	}{
		{"screenshot shrinks to the row limit", 1920, 1080, 80, 12, 426, 240, 43, 12},
		{"wide banner shrinks to the column limit", 4000, 100, 80, 12, 800, 20, 80, 1},
		{"small icon is not scaled up", 64, 32, 80, 12, 64, 32, 7, 2},
		{"exact fit", 800, 240, 80, 12, 800, 240, 80, 12},
		{"empty", 0, 10, 80, 12, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, h, c, r := Fit(tc.w0, tc.h0, cell, tc.maxC, tc.maxR)
			if w != tc.w || h != tc.h || c != tc.cols || r != tc.rows {
				t.Errorf("Fit = %dx%d in %dx%d cells, want %dx%d in %dx%d", w, h, c, r, tc.w, tc.h, tc.cols, tc.rows)
			}
			if c > tc.maxC || r > tc.maxR {
				t.Errorf("%dx%d cells is outside the %dx%d box", c, r, tc.maxC, tc.maxR)
			}
		})
	}
}

func TestThumbnailAveragesAndPads(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := range 2 {
		for x := range 4 {
			v := uint8(0)
			if x%2 == 1 {
				v = 200
			}
			src.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	out := Thumbnail(src, 2, 1, 1, 1, Cell{W: 3, H: 2})
	if out.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Fatalf("bounds = %v, want one 3x2 cell", out.Bounds())
	}
	if got := out.RGBAAt(0, 0); got != (color.RGBA{100, 100, 100, 255}) {
		t.Errorf("pixel = %v, want the average of a 2x2 block", got)
	}
	if got := out.RGBAAt(2, 1); got.A != 0 {
		t.Errorf("padding = %v, want transparent", got)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	img, err := Load(path)
	if err != nil || img.Bounds().Dx() != 3 {
		t.Fatalf("Load = %v, %v", img, err)
	}

	bad := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(bad, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Error("garbage should not decode")
	}
}

// A PNG that claims 50000×50000 pixels is refused from its header alone.
func TestLoadRefusesHugeImages(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 50000)
	binary.BigEndian.PutUint32(ihdr[4:], 50000)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	writeChunk(&b, "IHDR", ihdr)
	path := filepath.Join(t.TempDir(), "bomb.png")
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func writeChunk(b *bytes.Buffer, typ string, data []byte) {
	_ = binary.Write(b, binary.BigEndian, uint32(len(data))) //nolint:gosec // tiny
	b.WriteString(typ)
	b.Write(data)
	_ = binary.Write(b, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), data...)))
}

func TestPlaceholderIsExactlyTheBox(t *testing.T) {
	lines := Placeholder(42, 7, 3)
	if len(lines) != 3 {
		t.Fatalf("%d lines, want 3", len(lines))
	}
	for r, l := range lines {
		if w := lipgloss.Width(l); w != 7 {
			t.Errorf("line %d is %d cells wide, want 7", r, w)
		}
		if !strings.HasPrefix(l, "\x1b[38;5;42m") || !strings.HasSuffix(l, "\x1b[39m") {
			t.Errorf("line %d = %q, want the id as an indexed colour", r, l)
		}
		want := string([]rune{kitty.Placeholder, kitty.Diacritic(r), kitty.Diacritic(6)})
		if !strings.Contains(l, want) {
			t.Errorf("line %d lacks its last cell %q", r, want)
		}
	}
}

func TestTransmitChunks(t *testing.T) {
	// Noise doesn't compress, so the PNG needs several chunks.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // test noise
	for i := range img.Pix {
		img.Pix[i] = uint8(rng.Uint32()) //nolint:gosec // test noise
	}
	seq, err := Transmit(20, img, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	chunks := regexp.MustCompile(`\x1b_G([^;]*);([^\x1b]*)\x1b\\`).FindAllStringSubmatch(seq, -1)
	if len(chunks) < 2 {
		t.Fatalf("%d chunks, want several", len(chunks))
	}
	first := chunks[0][1]
	for _, opt := range []string{"a=T", "f=100", "i=20", "p=1", "U=1", "c=4", "r=2", "q=2", "m=1"} {
		if !strings.Contains(","+first+",", ","+opt+",") {
			t.Errorf("first chunk options %q lack %s", first, opt)
		}
	}
	for i, c := range chunks {
		if len(c[2]) > kitty.MaxChunkSize {
			t.Errorf("chunk %d carries %d bytes", i, len(c[2]))
		}
	}
	if last := chunks[len(chunks)-1][1]; !strings.Contains(last, "m=0") {
		t.Errorf("last chunk options %q lack m=0", last)
	}
}

func TestCommands(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{Place(20, 4, 2), "\x1b_Gi=20,p=1,U=1,c=4,r=2,q=2,a=p\x1b\\"},
		{Delete(20), "\x1b_Gi=20,d=I,q=2,a=d\x1b\\"},
		{Query(), "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[16t\x1b[14t"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}
