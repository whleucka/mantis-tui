package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/whleucka/mantis-tui/internal/graphics"
	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/meta"
	"github.com/whleucka/mantis-tui/internal/service"
)

// The clipboard image's thumbnail in a form is smaller than a note's.
const (
	clipThumbRows = 8
	clipThumbCols = 40
)

// Rows of the attachments section.
const (
	rowClip  = "clip"
	rowFiles = "files"
)

// attachSection picks a new note's or issue's attachments: the image in
// the clipboard and files from disk.
type attachSection struct {
	clip     *mantis.FileUpload // nil when the clipboard holds no image
	clipInfo string             // "1920×1080, 412.0 KB"
	clipOn   bool
	thumb    *thumb // the clipboard image, drawn once ready
	paths    []string
	input    textinput.Model
	hint     string // completions, or why a path was refused
	hintErr  bool
}

func newAttachSection(width int) *attachSection {
	in := textinput.New()
	in.Placeholder = "path to a file (tab completes)"
	in.SetWidth(max(min(width/2, 60), 20))
	return &attachSection{input: in}
}

// setClip offers the clipboard image, checked.
func (a *attachSection) setClip(clip *mantis.FileUpload) {
	if clip == nil {
		return
	}
	a.clip, a.clipOn = clip, true
	a.clipInfo = service.HumanSize(int64(len(clip.Content)))
	if w, h, err := graphics.Size(clip.Content); err == nil {
		a.clipInfo = fmt.Sprintf("%d×%d, %s", w, h, a.clipInfo)
	}
}

// rows are the section's focusable rows, in order.
func (a *attachSection) rows() []string {
	if a.clip != nil {
		return []string{rowClip, rowFiles}
	}
	return []string{rowFiles}
}

// focus moves the cursor to row, focusing the path input when it's Files.
func (a *attachSection) focus(row string) tea.Cmd {
	if row == rowFiles {
		return a.input.Focus()
	}
	a.input.Blur()
	return nil
}

// key handles a key on row. Unhandled keys (moving between rows, enter on
// an empty path, esc) are the form's.
func (a *attachSection) key(row string, msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k := msg.String()
	if row == rowClip {
		if k == "space" || k == "x" {
			a.clipOn = !a.clipOn
			return nil, true
		}
		return nil, false
	}
	switch k {
	case "tab":
		a.complete()
		return nil, true
	case "enter":
		if strings.TrimSpace(a.input.Value()) == "" {
			return nil, false
		}
		a.add()
		return nil, true
	case "backspace":
		if a.input.Value() == "" && len(a.paths) > 0 {
			a.paths = a.paths[:len(a.paths)-1]
			return nil, true
		}
	case "up", "down", "shift+tab", "esc", "alt+enter", "ctrl+s":
		return nil, false
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	a.hint, a.hintErr = "", false
	return cmd, true
}

// add checks the typed path and adds it to the list.
func (a *attachSection) add() {
	path := expandHome(strings.TrimSpace(a.input.Value()))
	st, err := os.Stat(path)
	switch {
	case err != nil:
		a.hint, a.hintErr = "no such file: "+path, true
		return
	case !st.Mode().IsRegular():
		a.hint, a.hintErr = "not a regular file: "+path, true
		return
	}
	a.paths = append(a.paths, path)
	a.input.SetValue("")
	a.hint, a.hintErr = "", false
}

// complete extends the typed path to the longest prefix its matches share,
// adding a slash after a directory, and lists them when there are several.
func (a *attachSection) complete() {
	typed := a.input.Value()
	dir, prefix := filepath.Split(typed)
	entries, err := os.ReadDir(expandHome(dir + "."))
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, prefix) && (strings.HasPrefix(prefix, ".") || !strings.HasPrefix(n, ".")) {
			if e.IsDir() {
				n += "/"
			}
			names = append(names, n)
		}
	}
	switch len(names) {
	case 0:
		a.hint, a.hintErr = "no match", true
		return
	case 1:
		a.hint = ""
	default:
		a.hint = strings.Join(names[:min(len(names), 8)], "  ")
		if len(names) > 8 {
			a.hint += fmt.Sprintf("  … %d more", len(names)-8)
		}
	}
	a.hintErr = false
	common := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, common) {
			common = common[:len(common)-1]
		}
	}
	a.input.SetValue(dir + common)
	a.input.CursorEnd()
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// selection is what to send: the clipboard image if checked, and the paths.
func (a *attachSection) selection() (*mantis.FileUpload, []string) {
	clip := a.clip
	if !a.clipOn {
		clip = nil
	}
	return clip, append([]string(nil), a.paths...)
}

// count is how many files will be sent.
func (a *attachSection) count() int {
	n := len(a.paths)
	if a.clip != nil && a.clipOn {
		n++
	}
	return n
}

// names lists the files that will be sent.
func (a *attachSection) names() []string {
	var out []string
	if a.clip != nil && a.clipOn {
		out = append(out, a.clip.Name)
	}
	for _, p := range a.paths {
		out = append(out, filepath.Base(p))
	}
	return out
}

// gatherUploads reads the chosen files and checks everything against the
// server's limits. It reads files, so it runs inside a tea.Cmd or on a
// form's enter, never in View.
func gatherUploads(clip *mantis.FileUpload, paths []string, l meta.UploadLimits) ([]mantis.FileUpload, error) {
	var files []mantis.FileUpload
	if clip != nil {
		files = append(files, *clip)
	}
	for _, p := range paths {
		f, err := service.ReadUpload(p, l)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if err := service.CheckUploads(files, l); err != nil {
		return nil, err
	}
	return files, nil
}

// view draws the section with the focused row highlighted ("" for none).
func (a *attachSection) view(focused string) string {
	var b strings.Builder
	if a.clip != nil {
		box := "[ ]"
		if a.clipOn {
			box = "[x]"
		}
		line := box + " clipboard image"
		if focused == rowClip {
			line = styleSelected.Render(line)
		}
		b.WriteString(line + styleMuted.Render(" "+a.clipInfo) + "\n")
		if a.thumb != nil && a.thumb.ready {
			b.WriteString(strings.Join(a.thumb.lines, "\n") + "\n")
		}
	}
	label := "Files "
	if focused == rowFiles {
		label = styleSelected.Render(label)
	}
	b.WriteString(label + a.input.View())
	for _, p := range a.paths {
		size := ""
		if st, err := os.Stat(p); err == nil {
			size = " · " + service.HumanSize(st.Size())
		}
		b.WriteString("\n  " + filepath.Base(p) + styleMuted.Render(size))
	}
	if a.hint != "" {
		style := styleMuted
		if a.hintErr {
			style = styleError
		}
		b.WriteString("\n" + style.Render(a.hint))
	}
	return b.String()
}

// clipThumb sends the clipboard image to the terminal as a small
// thumbnail for the form, when inline images are on.
func (m *Model) clipThumb(a *attachSection) tea.Cmd {
	if !m.img.on || a.clip == nil || a.thumb != nil {
		return nil
	}
	m.img.clipSeq++
	key := "clipboard/" + strconv.Itoa(m.img.clipSeq)
	id := m.img.allocate(key)
	a.thumb = &thumb{id: id}
	m.img.thumbs[key] = a.thumb
	content, cell := a.clip.Content, m.img.cell
	return func() tea.Msg {
		msg := thumbReadyMsg{key: key, id: id}
		img, err := graphics.Decode(content)
		if err == nil {
			msg.seq, msg.cols, msg.rows, err = thumbOf(img, id, cell, clipThumbCols, clipThumbRows)
		}
		msg.err = err
		return msg
	}
}

// attachModal is the create form's attachments section as a modal.
type attachModal struct {
	sec   *attachSection
	row   int
	title string
}

func newAttachModal(sec *attachSection, title string) *attachModal {
	am := &attachModal{sec: sec, title: title, row: len(sec.rows()) - 1}
	sec.focus(sec.rows()[am.row])
	return am
}

// opened starts the clipboard thumbnail when the modal appears.
func (am *attachModal) opened(m *Model) tea.Cmd {
	return tea.Batch(m.clipThumb(am.sec), am.sec.input.Focus())
}

func (am *attachModal) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	rows := am.sec.rows()
	am.row = min(am.row, len(rows)-1)
	if cmd, ok := am.sec.key(rows[am.row], msg); ok {
		return cmd, false
	}
	switch msg.String() {
	case "esc", "enter":
		am.sec.input.Blur()
		return nil, true
	case "up", "shift+tab":
		am.row = (am.row + len(rows) - 1) % len(rows)
	case "down", "tab":
		am.row = (am.row + 1) % len(rows)
	}
	return am.sec.focus(rows[am.row]), false
}

func (am *attachModal) view(width, _ int) string {
	rows := am.sec.rows()
	body := styleTitle.Render(am.title) + "\n\n" + am.sec.view(rows[min(am.row, len(rows)-1)]) +
		"\n\n" + styleMuted.Render("↑↓ move · space toggle · tab complete · enter add · backspace remove last · esc done")
	return styleModal.Width(min(max(width*2/3, 50), width-2)).Render(body)
}
