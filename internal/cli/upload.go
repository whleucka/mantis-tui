package cli

import (
	"context"
	"strconv"

	"github.com/spf13/pflag"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

// uploadFlags are the --file and --clipboard flags of note and create.
type uploadFlags struct {
	paths     []string
	clipboard bool
}

func (u *uploadFlags) register(f *pflag.FlagSet) {
	f.StringArrayVar(&u.paths, "file", nil, "attach a file (repeatable)")
	f.BoolVar(&u.clipboard, "clipboard", false, "attach the image in the clipboard")
}

func (u *uploadFlags) any() bool { return len(u.paths) > 0 || u.clipboard }

// load reads the attachments and checks them against the server's limits,
// so nothing is sent (and no editor opens) when one would be refused.
func (u *uploadFlags) load(ctx context.Context, o *globalOpts, s *session) ([]mantis.FileUpload, error) {
	if !u.any() {
		return nil, nil
	}
	limits, err := s.meta.Uploads(ctx)
	if err != nil {
		return nil, err
	}
	var files []mantis.FileUpload
	for _, p := range u.paths {
		f, err := service.ReadUpload(p, limits)
		if err != nil {
			return nil, usageErrorf("%v", err)
		}
		files = append(files, f)
	}
	if u.clipboard {
		img := o.deps.clipboard(ctx)
		if img == nil {
			return nil, usageErrorf("--clipboard: the clipboard holds no image")
		}
		files = append(files, *img)
	}
	if err := service.CheckUploads(files, limits); err != nil {
		return nil, usageErrorf("%v", err)
	}
	return files, nil
}

// withFiles is " with N files" for a status line, or nothing.
func withFiles(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return " with 1 file"
	}
	return " with " + strconv.Itoa(n) + " files"
}
