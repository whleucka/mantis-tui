// Package editor round-trips text through the user's $VISUAL/$EDITOR via a
// private temp file.
package editor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// ErrEmpty means the user saved nothing new; the caller should send nothing.
var ErrEmpty = errors.New("empty text, nothing sent")

// Request describes one editing session.
type Request struct {
	Host    string   // used in the temp file name
	IssueID int      // used in the temp file name
	Kind    string   // "note", "description", ...
	Initial string   // pre-filled text
	Hints   []string // shown as "# ..." lines and stripped afterwards
}

// Session is a prepared temp file plus the editor command to run on it.
type Session struct {
	Path    string
	initial string
	hints   map[string]bool
}

// Command returns the editor argv from $VISUAL, then $EDITOR, then vi.
func Command(getenv func(string) string) []string {
	for _, k := range []string{"VISUAL", "EDITOR"} {
		if f := strings.Fields(getenv(k)); len(f) > 0 {
			return f
		}
	}
	return []string{"vi"}
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9]+`)

// Prepare writes the initial text and hints to a new 0600 temp file.
func Prepare(req Request) (*Session, error) {
	pattern := fmt.Sprintf("mantis-%s-%d-%s-*.md", unsafeName.ReplaceAllString(req.Host, "-"), req.IssueID, req.Kind)
	f, err := os.CreateTemp("", pattern) // CreateTemp uses mode 0600
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}

	s := &Session{Path: f.Name(), initial: strings.TrimSpace(req.Initial), hints: map[string]bool{}}
	var b strings.Builder
	if req.Initial != "" {
		b.WriteString(strings.TrimRight(req.Initial, "\n"))
	}
	b.WriteString("\n")
	for _, h := range req.Hints {
		line := "# " + h
		s.hints[line] = true
		b.WriteString(line + "\n")
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	return s, nil
}

// Cmd returns the editor command without stdio attached, for
// tea.ExecProcess, which wires the terminal itself.
func (s *Session) Cmd() *exec.Cmd {
	argv := append(Command(os.Getenv), s.Path)
	return exec.Command(argv[0], argv[1:]...) //nolint:gosec // the user's own $EDITOR
}

// Run runs the editor attached to this process's terminal.
func (s *Session) Run() error {
	c := s.Cmd()
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("editor %s: %w", c.Args[0], err)
	}
	return nil
}

// Text reads the edited file and strips the hint lines. It returns ErrEmpty
// when the result is blank or identical to the initial text.
func (s *Session) Text() (string, error) {
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return "", fmt.Errorf("read edited text: %w", err)
	}
	var kept []string
	for _, line := range strings.Split(string(b), "\n") {
		if !s.hints[strings.TrimRight(line, " \t\r")] {
			kept = append(kept, line)
		}
	}
	text := strings.TrimSpace(strings.Join(kept, "\n"))
	if text == "" || text == s.initial {
		return "", ErrEmpty
	}
	return text, nil
}

// Cleanup removes the temp file. Call it only after the text was submitted;
// on failure leave the file so the user's text is not lost.
func (s *Session) Cleanup() { _ = os.Remove(s.Path) }
