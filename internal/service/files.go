package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// FileRef is an attachment of an issue or of one of its notes.
type FileRef struct {
	mantis.Attachment
	NoteID int    // 0 for a file on the issue itself
	Author string // the note's author; empty for a file on the issue
}

// Where says what a file is attached to: "issue" or "note 45 by jane".
func (f FileRef) Where() string {
	if f.NoteID == 0 {
		return "issue"
	}
	return fmt.Sprintf("note %d by %s", f.NoteID, f.Author)
}

// Files lists the issue's own attachments, then its notes' in note order.
func Files(is *mantis.Issue) []FileRef {
	var out []FileRef
	for _, a := range is.Attachments {
		out = append(out, FileRef{Attachment: a})
	}
	for _, n := range is.Notes {
		for _, a := range n.Attachments {
			out = append(out, FileRef{Attachment: a, NoteID: n.ID, Author: n.Reporter.Display()})
		}
	}
	return out
}

// HumanSize formats a byte count as "512 B", "4.6 KB" or "1.2 MB".
func HumanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}

// SafeName reduces an untrusted file name to one harmless path component:
// no directories, no control characters, no leading dot, at most 100 bytes.
func SafeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimLeft(strings.TrimSpace(name), ".")
	if len(name) > 100 {
		ext := filepath.Ext(name)
		if len(ext) > 10 {
			ext = ""
		}
		name = strings.ToValidUTF8(name[:100-len(ext)], "") + ext
	}
	if name == "" {
		return "file"
	}
	return name
}

// DiskName is a file's name on disk, "<id>-<safe name>", which is unique
// within an issue even when two notes attach "image.png".
func DiskName(a mantis.Attachment) string {
	return fmt.Sprintf("%d-%s", a.ID, SafeName(a.Filename))
}

// Download saves file a of issueID into dir (created 0700 if missing) as
// DiskName(a), mode 0600, and returns its path. With reuse, a file already
// there with the expected size is returned without a request.
func Download(ctx context.Context, api mantis.API, issueID int, a mantis.Attachment, dir string, reuse bool) (string, error) {
	path := filepath.Join(dir, DiskName(a))
	if reuse {
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() && st.Size() == a.Size {
			return path, nil
		}
	}
	f, err := api.GetFile(ctx, issueID, a.ID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("save %s: %w", a.Filename, err)
	}
	tmp, err := os.CreateTemp(dir, ".download-*") // CreateTemp uses mode 0600
	if err != nil {
		return "", fmt.Errorf("save %s: %w", a.Filename, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // a no-op after the rename
	if _, err := tmp.Write(f.Content); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("save %s: %w", a.Filename, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("save %s: %w", a.Filename, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", fmt.Errorf("save %s: %w", a.Filename, err)
	}
	return path, nil
}

var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/bmp": true}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true}

// IsImage reports whether a is a raster image a terminal can show, by its
// content type or its extension.
func IsImage(a mantis.Attachment) bool {
	ct, _, _ := strings.Cut(a.ContentType, ";")
	return imageTypes[strings.ToLower(strings.TrimSpace(ct))] || imageExts[strings.ToLower(filepath.Ext(a.Filename))]
}

// safeExts are the extensions desktop openers hand to a viewer rather than
// run. Attachments come from other people, so nothing else is opened.
var safeExts = map[string]bool{
	".pdf": true, ".txt": true, ".log": true, ".md": true, ".json": true, ".csv": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
}

// SafeToOpen reports whether the file at path may be handed to xdg-open.
func SafeToOpen(path string) bool { return safeExts[strings.ToLower(filepath.Ext(path))] }

// OpenFile opens a local file with the desktop's default application
// without waiting for it.
func OpenFile(path string) error {
	if err := OpenBrowser(path); err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	return nil
}
