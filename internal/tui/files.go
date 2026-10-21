package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/service"
)

// fileTimeout bounds one attachment download, which can be a large PDF.
const fileTimeout = 2 * time.Minute

type (
	// fileReadyMsg reports a downloaded (or cached) attachment.
	fileReadyMsg struct {
		host    string
		issueID int
		file    service.FileRef
		path    string
		err     error
	}
	// imageShownMsg arrives when the image viewer gives the terminal back.
	imageShownMsg struct {
		host string
		file service.FileRef
		path string
		err  error
	}
)

// pickFile offers the issue's attachments, its own and its notes', and
// opens the chosen one. A single attachment opens at once.
func (m *Model) pickFile(iv *issueModel) tea.Cmd {
	if iv.issue == nil {
		return nil
	}
	files := service.Files(iv.issue)
	switch len(files) {
	case 0:
		return infoCmd(iv.sess.Host.Name, fmt.Sprintf("#%d has no attachments", iv.id))
	case 1:
		return m.fetchFile(iv.id, files[0])
	}
	opts := make([]pickerOption, len(files))
	for i, f := range files {
		opts[i] = pickerOption{label: f.Filename, detail: service.HumanSize(f.Size) + " · " + f.Where(), value: f}
	}
	id := iv.id
	m.modal = newPicker(fmt.Sprintf("Open which attachment of #%d?", id), opts, func(o pickerOption) tea.Cmd {
		return m.fetchFile(id, o.value.(service.FileRef))
	})
	return nil
}

// fetchFile downloads f into the per-host, per-issue cache, reusing a copy
// already there.
func (m *Model) fetchFile(issueID int, f service.FileRef) tea.Cmd {
	host, api := m.cur.sess.Host.Name, m.cur.sess.API
	if m.opts.FilesDir == "" {
		return errCmd(host, errors.New("no directory to save attachments in"))
	}
	dir := m.filesDir(host, issueID)
	return m.call(func(ctx context.Context) tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, fileTimeout)
		defer cancel()
		path, err := service.Download(ctx, api, issueID, f.Attachment, dir, true)
		return fileReadyMsg{host: host, issueID: issueID, file: f, path: path, err: err}
	})
}

// downloadAll saves every attachment of the issue that isn't cached yet
// into its folder.
func (m *Model) downloadAll(iv *issueModel) tea.Cmd {
	if iv.issue == nil {
		return nil
	}
	host, api := iv.sess.Host.Name, iv.sess.API
	files := service.Files(iv.issue)
	switch {
	case len(files) == 0:
		return infoCmd(host, fmt.Sprintf("#%d has no attachments", iv.id))
	case m.opts.FilesDir == "":
		return errCmd(host, errors.New("no directory to save attachments in"))
	}
	id, dir := iv.id, m.filesDir(host, iv.id)
	return m.call(func(ctx context.Context) tea.Msg {
		var errs []error
		for _, f := range files {
			ctx, cancel := context.WithTimeout(ctx, fileTimeout)
			if _, err := service.Download(ctx, api, id, f.Attachment, dir, true); err != nil {
				errs = append(errs, err)
			}
			cancel()
		}
		if err := errors.Join(errs...); err != nil {
			return errMsg{host: host, err: err}
		}
		return infoMsg{host: host, text: fmt.Sprintf("saved %d attachments to %s", len(files), dir)}
	})
}

// openFolder opens the issue's attachments folder in the file manager.
func (m *Model) openFolder(iv *issueModel) tea.Cmd {
	host, dir := iv.sess.Host.Name, m.filesDir(iv.sess.Host.Name, iv.id)
	switch st, err := os.Stat(dir); {
	case m.opts.FilesDir == "" || err != nil || !st.IsDir():
		return infoCmd(host, fmt.Sprintf("nothing saved for #%d yet; %s downloads it all", iv.id, keyOf(actDownloadAll)))
	case m.opts.OpenFolder == nil:
		return infoCmd(host, dir)
	}
	if err := m.opts.OpenFolder(dir); err != nil {
		return errCmd(host, err)
	}
	return infoCmd(host, "opened "+dir)
}

// filesDir is where an issue's attachments are cached.
func (m *Model) filesDir(host string, issueID int) string {
	return filepath.Join(m.opts.FilesDir, service.SafeName(host), strconv.Itoa(issueID))
}

// onFileReady shows an image in the terminal when it can, and otherwise
// hands safe file types to the desktop opener.
func (m *Model) onFileReady(msg fileReadyMsg) tea.Cmd {
	if msg.err != nil {
		return errCmd(msg.host, msg.err)
	}
	if service.IsImage(msg.file.Attachment) && m.opts.ShowImage != nil {
		m.editing = true // pauses refreshes while the viewer owns the terminal
		images, start := m.viewerImages(msg)
		return m.opts.ShowImage(images, start, m.width, m.height, func(err error) tea.Msg {
			return imageShownMsg{host: msg.host, file: msg.file, path: msg.path, err: err}
		})
	}
	return m.openFile(msg.host, msg.file, msg.path)
}

// viewerImages is every image on the issue msg's file belongs to, for the
// viewer to step through, and the index of msg's. The others download when
// the viewer reaches them.
func (m *Model) viewerImages(msg fileReadyMsg) ([]ViewerImage, int) {
	title := func(f service.FileRef) string { return fmt.Sprintf("%s · %s", f.Filename, f.Where()) }
	shown := ViewerImage{Title: title(msg.file), Fetch: func() (string, error) { return msg.path, nil }}
	if m.cur == nil || m.cur.sess.Host.Name != msg.host {
		return []ViewerImage{shown}, 0
	}
	iv := m.cur.issue
	if iv == nil || iv.id != msg.issueID || iv.issue == nil {
		return []ViewerImage{shown}, 0
	}
	api, dir := iv.sess.API, m.filesDir(msg.host, msg.issueID)
	var images []ViewerImage
	start := 0
	for _, f := range service.Files(iv.issue) {
		switch {
		case !service.IsImage(f.Attachment):
			continue
		case f.ID == msg.file.ID:
			start = len(images)
			images = append(images, shown)
			continue
		}
		images = append(images, ViewerImage{Title: title(f), Fetch: func() (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
			defer cancel()
			return service.Download(ctx, api, msg.issueID, f.Attachment, dir, true)
		}})
	}
	if len(images) == 0 { // the file left the issue meanwhile
		return []ViewerImage{shown}, 0
	}
	return images, start
}

func (m *Model) onImageShown(msg imageShownMsg) tea.Cmd {
	m.editing = false
	switch {
	case errors.Is(msg.err, ErrNoGraphics):
		return m.openFile(msg.host, msg.file, msg.path)
	case msg.err != nil:
		return errCmd(msg.host, fmt.Errorf("show %s: %w", msg.file.Filename, msg.err))
	}
	return nil
}

// openFile opens path with the desktop's default application, but only for
// file types that are viewed rather than run; anything else is just saved.
func (m *Model) openFile(host string, f service.FileRef, path string) tea.Cmd {
	if !service.SafeToOpen(path) || m.opts.OpenFile == nil {
		return infoCmd(host, "saved "+path)
	}
	if err := m.opts.OpenFile(path); err != nil {
		return errCmd(host, err)
	}
	return infoCmd(host, "opened "+f.Filename)
}
