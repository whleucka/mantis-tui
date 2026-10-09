package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
)

// MaxRequestBytes is PHP's default post_max_size. Mantis can't report the
// real one, and a bigger request is answered with success and nothing
// attached, so uploads stay under it.
const MaxRequestBytes = 8 << 20

// ReadUpload reads a regular file to attach, refusing one over the
// server's size limit before reading it.
func ReadUpload(path string, l meta.UploadLimits) (mantis.FileUpload, error) {
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return mantis.FileUpload{}, fmt.Errorf("attach %s: no such file", path)
	}
	if err != nil {
		return mantis.FileUpload{}, fmt.Errorf("attach %s: %w", path, err)
	}
	if !st.Mode().IsRegular() {
		return mantis.FileUpload{}, fmt.Errorf("attach %s: not a regular file", path)
	}
	if err := checkSize(filepath.Base(path), st.Size(), l); err != nil {
		return mantis.FileUpload{}, err
	}
	b, err := os.ReadFile(path) //nolint:gosec // a file the user chose to attach
	if err != nil {
		return mantis.FileUpload{}, fmt.Errorf("attach %s: %w", path, err)
	}
	return mantis.FileUpload{Name: filepath.Base(path), Content: b}, nil
}

func checkSize(name string, size int64, l meta.UploadLimits) error {
	if l.MaxFileSize > 0 && size > l.MaxFileSize {
		return fmt.Errorf("%s is %s; the server takes files up to %s", name, HumanSize(size), HumanSize(l.MaxFileSize))
	}
	return nil
}

// CheckUploads applies the server's limits to files about to be sent, so
// nothing goes out that Mantis would refuse or silently drop.
func CheckUploads(files []mantis.FileUpload, l meta.UploadLimits) error {
	var body int64
	for _, f := range files {
		if err := checkSize(f.Name, int64(len(f.Content)), l); err != nil {
			return err
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Name), "."))
		if slices.Contains(l.Disallowed, ext) || (len(l.Allowed) > 0 && !slices.Contains(l.Allowed, ext)) {
			return fmt.Errorf("%s: the server doesn't accept .%s files", f.Name, ext)
		}
		body += int64(len(f.Content)+2) / 3 * 4 // base64
	}
	if body > MaxRequestBytes-64<<10 { // room for the rest of the request
		return fmt.Errorf("attachments come to %s encoded; one request must stay under %s", HumanSize(body), HumanSize(MaxRequestBytes))
	}
	return nil
}

// Runner runs a command and returns what it printed.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// clipboardTypes are the image types taken from the clipboard, best first.
var clipboardTypes = []struct{ mime, ext string }{{"image/png", ".png"}, {"image/jpeg", ".jpg"}}

// ClipboardImage returns the image in the clipboard, named
// screenshot-<time>, read with wl-paste on Wayland or xclip on X11. No
// image, no tool or a failing tool all mean nil: the clipboard is only
// ever offered.
func ClipboardImage(ctx context.Context, run Runner, lookPath func(string) (string, error), getenv func(string) string, now time.Time) *mantis.FileUpload {
	var list, get func(mime string) []string
	var tool string
	switch {
	case getenv("WAYLAND_DISPLAY") != "" && found(lookPath, "wl-paste"):
		tool = "wl-paste"
		list = func(string) []string { return []string{"--list-types"} }
		get = func(mime string) []string { return []string{"--no-newline", "--type", mime} }
	case getenv("DISPLAY") != "" && found(lookPath, "xclip"):
		tool = "xclip"
		list = func(string) []string { return []string{"-selection", "clipboard", "-t", "TARGETS", "-o"} }
		get = func(mime string) []string { return []string{"-selection", "clipboard", "-t", mime, "-o"} }
	default:
		return nil
	}
	out, err := run(ctx, tool, list("")...)
	if err != nil {
		return nil
	}
	types := strings.Fields(string(out))
	for _, t := range clipboardTypes {
		if !slices.Contains(types, t.mime) {
			continue
		}
		b, err := run(ctx, tool, get(t.mime)...)
		if err != nil || len(b) == 0 {
			return nil
		}
		return &mantis.FileUpload{Name: ScreenshotName(now, t.ext), Content: b}
	}
	return nil
}

func found(lookPath func(string) (string, error), name string) bool {
	_, err := lookPath(name)
	return err == nil
}

// ScreenshotName names a pasted image like the web UI's pastes, in local
// time: screenshot-20261009-074512.png.
func ScreenshotName(now time.Time, ext string) string {
	return "screenshot-" + now.Format("20060102-150405") + ext
}

// RunOutput runs a command and returns its standard output.
func RunOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // the fixed clipboard tools
}

// Clipboard reads the clipboard image now, giving up after a few seconds.
func Clipboard(ctx context.Context) *mantis.FileUpload {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return ClipboardImage(ctx, RunOutput, exec.LookPath, os.Getenv, time.Now())
}
